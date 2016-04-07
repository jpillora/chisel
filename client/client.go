package chclient

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jpillora/backoff"
	"github.com/jpillora/chisel/share"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type Config struct {
	shared      *chshare.Config
	Fingerprint string
	Auth        string
	KeepAlive   time.Duration
	Server      string
	Proxy       string
	Remotes     []string
}

type Client struct {
	*chshare.Logger
	config    *Config
	sshConfig *ssh.ClientConfig
	proxies   []*Proxy
	sshConn   ssh.Conn
	server    string
	running   bool
	runningc  chan error
}

func NewClient(config *Config) (*Client, error) {

	//apply default scheme
	if !strings.HasPrefix(config.Server, "http") {
		config.Server = "http://" + config.Server
	}

	u, err := url.Parse(config.Server)
	if err != nil {
		return nil, err
	}

	//apply default port
	if !regexp.MustCompile(`:\d+$`).MatchString(u.Host) {
		if u.Scheme == "https" || u.Scheme == "wss" {
			u.Host = u.Host + ":443"
		} else {
			u.Host = u.Host + ":80"
		}
	}

	//swap to websockets scheme
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)

	shared := &chshare.Config{}
	for _, s := range config.Remotes {
		r, err := chshare.DecodeRemote(s)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode remote '%s': %s", s, err)
		}
		shared.Remotes = append(shared.Remotes, r)
	}
	config.shared = shared

	client := &Client{
		Logger:   chshare.NewLogger("client"),
		config:   config,
		server:   u.String(),
		running:  true,
		runningc: make(chan error, 1),
	}

	user, pass := chshare.ParseAuth(config.Auth)

	client.sshConfig = &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		ClientVersion:   chshare.ProtocolVersion + "-client",
		HostKeyCallback: client.verifyServer,
	}

	return client, nil
}

//Start then Wait
func (c *Client) Run() error {
	go c.start()
	return c.Wait()
}

func (c *Client) verifyServer(hostname string, remote net.Addr, key ssh.PublicKey) error {
	expect := c.config.Fingerprint
	got := chshare.FingerprintKey(key)
	if expect != "" && !strings.HasPrefix(got, expect) {
		return fmt.Errorf("Invalid fingerprint (%s)", got)
	}
	//overwrite with complete fingerprint
	c.Infof("Fingerprint %s", got)
	return nil
}

func HttpConnect(proxy, url_ string) (net.Conn, error) {
	p, err := net.Dial("tcp", proxy)
	if err != nil {
		return nil, err
	}

	turl, err := url.Parse(url_)
	if err != nil {
		return nil, err
	}

	req := http.Request{
		Method: "CONNECT",
		URL:    &url.URL{},
		Host:   turl.Host,
	}

	cc := httputil.NewProxyClientConn(p, nil)
	cc.Do(&req)
	if err != nil && err != httputil.ErrPersistEOF {
		return nil, err
	}

	rwc, _ := cc.Hijack()

	return rwc, nil
}

func ProxyDial(server, proxy, protocol, origin string) (ws *websocket.Conn, err error) {
	var wsConn net.Conn

	if proxy == "" {
		return websocket.Dial(server, protocol, origin)
	}

	purl, err := url.Parse(proxy)
	if err != nil {
		return nil, err
	}

	config, err := websocket.NewConfig(server, origin)
	if err != nil {
		return nil, err
	}

	if protocol != "" {
		config.Protocol = []string{protocol}
	}

	client, err := HttpConnect(purl.Host, server)
	if err != nil {
		return nil, err
	}

	// Parse the endpoint of the URL to determine how to proxy
	endptUrl, _ := url.Parse(server)
	host, _, err := net.SplitHostPort(endptUrl.Host)

	switch endptUrl.Scheme {
	case "ws":
		wsConn = client
	case "wss":
		wsConn = tls.Client(client, &tls.Config{ServerName: host})
	default:
		return nil, websocket.ErrBadScheme
	}

	return websocket.NewClient(config, wsConn)
}

//Starts the client
func (c *Client) Start() {
	go c.start()
}

func (c *Client) start() {
	c.Infof("Connecting to %s\n", c.server)

	//prepare proxies
	for id, r := range c.config.shared.Remotes {
		proxy := NewProxy(c, id, r)
		go proxy.start()
		c.proxies = append(c.proxies, proxy)
	}

	//optional keepalive loop
	if c.config.KeepAlive > 0 {
		go func() {
			for range time.Tick(c.config.KeepAlive) {
				if c.sshConn != nil {
					c.sshConn.SendRequest("ping", true, nil)
				}
			}
		}()
	}

	//connection loop!
	var connerr error
	b := &backoff.Backoff{Max: 5 * time.Minute}

	for {
		if !c.running {
			break
		}
		if connerr != nil {
			d := b.Duration()
			c.Infof("Retrying in %s...\n", d)
			connerr = nil
			time.Sleep(d)
		}

		ws, err := ProxyDial(c.server, c.config.Proxy, chshare.ProtocolVersion, "http://localhost/")
		if err != nil {
			connerr = err
			continue
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(ws, "", c.sshConfig)

		//NOTE: break == dont retry on handshake failures
		if err != nil {
			if strings.Contains(err.Error(), "unable to authenticate") {
				c.Infof("Authentication failed")
				c.Debugf(err.Error())
			} else {
				c.Infof(err.Error())
			}
			break
		}
		conf, _ := chshare.EncodeConfig(c.config.shared)
		c.Debugf("Sending configurating")
		t0 := time.Now()
		_, configerr, err := sshConn.SendRequest("config", true, conf)
		if err != nil {
			c.Infof("Config verification failed")
			break
		}
		if len(configerr) > 0 {
			c.Infof(string(configerr))
			break
		}
		c.Infof("Connected (Latency %s)", time.Now().Sub(t0))
		//connected
		b.Reset()

		c.sshConn = sshConn
		go ssh.DiscardRequests(reqs)
		go chshare.RejectStreams(chans) //TODO allow client to ConnectStreams
		err = sshConn.Wait()
		//disconnected
		c.sshConn = nil
		if err != nil && err != io.EOF {
			connerr = err
			c.Infof("Disconnection error: %s", err)
			continue
		}
		c.Infof("Disconnected\n")
	}
	close(c.runningc)
}

//Wait blocks while the client is running
func (c *Client) Wait() error {
	return <-c.runningc
}

//Close manual stops the client
func (c *Client) Close() error {
	c.running = false
	if c.sshConn == nil {
		return nil
	}
	return c.sshConn.Close()
}
