package chclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jpillora/backoff"
	"github.com/jpillora/chisel/share"
	"golang.org/x/crypto/ssh"
)

//Config represents a client configuration
type Config struct {
	shared              *chshare.Config
	Fingerprint         string
	Auth                string
	KeepAlive           time.Duration
	MaxRetryCount       int
	MaxRetryInterval    time.Duration
	Server              string
	SkipTlsVerification bool
	Proxy               string
	Remotes             []string
	HostHeader          string
}

//Client represents a client instance
type Client struct {
	*chshare.Logger
	config    *Config
	sshConfig *ssh.ClientConfig
	sshConn   ssh.Conn
	proxyURL  *url.URL
	server    string
	running   bool
	runningc  chan error
	connStats chshare.ConnStats
}

//NewClient creates a new client instance
func NewClient(config *Config) (*Client, error) {
	//apply default scheme
	if !strings.HasPrefix(config.Server, "http") {
		config.Server = "http://" + config.Server
	}
	if config.MaxRetryInterval < time.Second {
		config.MaxRetryInterval = 5 * time.Minute
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
	client.Info = true

	if p := config.Proxy; p != "" {
		client.proxyURL, err = url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("Invalid proxy URL (%s)", err)
		}
	}

	user, pass := chshare.ParseAuth(config.Auth)

	client.sshConfig = &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		ClientVersion:   "SSH-" + chshare.ProtocolVersion + "-client",
		HostKeyCallback: client.verifyServer,
		Timeout:         30 * time.Second,
	}

	return client, nil
}

//Run starts client and blocks while connected
func (c *Client) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		return err
	}
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

//Start client and does not block
func (c *Client) Start(ctx context.Context) error {
	via := ""
	if c.proxyURL != nil {
		via = " via " + c.proxyURL.String()
	}
	//prepare non-reverse proxies
	for i, r := range c.config.shared.Remotes {
		if !r.Reverse {
			proxy := chshare.NewTCPProxy(c.Logger, func() ssh.Conn { return c.sshConn }, i, r)
			if err := proxy.Start(ctx); err != nil {
				return err
			}
		}
	}
	c.Infof("Connecting to %s%s\n", c.server, via)
	//optional keepalive loop
	if c.config.KeepAlive > 0 {
		go c.keepAliveLoop()
	}
	//connection loop
	go c.connectionLoop()
	return nil
}

func (c *Client) keepAliveLoop() {
	for c.running {
		time.Sleep(c.config.KeepAlive)
		if c.sshConn != nil {
			c.sshConn.SendRequest("ping", true, nil)
		}
	}
}

func (c *Client) connectionLoop() {
	//connection loop!
	var connerr error
	b := &backoff.Backoff{Max: c.config.MaxRetryInterval}
	for c.running {
		if connerr != nil {
			attempt := int(b.Attempt())
			maxAttempt := c.config.MaxRetryCount
			d := b.Duration()
			//show error and attempt counts
			msg := fmt.Sprintf("Connection error: %s", connerr)
			if attempt > 0 {
				msg += fmt.Sprintf(" (Attempt: %d", attempt)
				if maxAttempt > 0 {
					msg += fmt.Sprintf("/%d", maxAttempt)
				}
				msg += ")"
			}
			c.Debugf(msg)
			//give up?
			if maxAttempt >= 0 && attempt >= maxAttempt {
				break
			}
			c.Infof("Retrying in %s...", d)
			connerr = nil
			chshare.SleepSignal(d)
		}
		d := websocket.Dialer{
			ReadBufferSize:   1024,
			WriteBufferSize:  1024,
			HandshakeTimeout: 45 * time.Second,
			Subprotocols:     []string{chshare.ProtocolVersion},
		}
		//optionally proxy
		if c.proxyURL != nil {
			if strings.HasPrefix(c.proxyURL.Scheme, "socks") {
				// SOCKS5 proxy
				if c.proxyURL.Scheme != "socks" && c.proxyURL.Scheme != "socks5h" {
					c.Infof(
						"unsupported socks proxy type: %s:// (only socks5h:// or socks:// is supported)",
						c.proxyURL.Scheme)
					break
				}
				dial, err := chshare.NewSocks5Dial(c.proxyURL)
				if err != nil {
					connerr = err
					continue
				}
				d.NetDial = dial
			} else {
				// CONNECT proxy
				d.Proxy = func(*http.Request) (*url.URL, error) {
					return c.proxyURL, nil
				}
			}
		}
		if c.config.SkipTlsVerification {
			d.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		wsHeaders := http.Header{}
		if c.config.HostHeader != "" {
			wsHeaders = http.Header{
				"Host": {c.config.HostHeader},
			}
		}
		wsConn, _, err := d.Dial(c.server, wsHeaders)
		if err != nil {
			connerr = err
			continue
		}
		conn := chshare.NewWebSocketConn(wsConn)
		// perform SSH handshake on net.Conn
		c.Debugf("Handshaking...")
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", c.sshConfig)
		if err != nil {
			if strings.Contains(err.Error(), "unable to authenticate") {
				c.Infof("Authentication failed")
				c.Debugf(err.Error())
			} else {
				c.Infof(err.Error())
			}
			break
		}
		c.config.shared.Version = chshare.BuildVersion
		conf, _ := chshare.EncodeConfig(c.config.shared)
		c.Debugf("Sending config")
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
		c.Infof("Connected (Latency %s)", time.Since(t0))
		//connected
		b.Reset()
		c.sshConn = sshConn
		go ssh.DiscardRequests(reqs)
		go c.connectStreams(chans)
		err = sshConn.Wait()
		//disconnected
		c.sshConn = nil
		if err != nil && err != io.EOF {
			connerr = err
			continue
		}
		c.Infof("Disconnected\n")
	}
	close(c.runningc)
}

//Wait blocks while the client is running.
//Can only be called once.
func (c *Client) Wait() error {
	return <-c.runningc
}

//Close manually stops the client
func (c *Client) Close() error {
	c.running = false
	if c.sshConn == nil {
		return nil
	}
	return c.sshConn.Close()
}

func (c *Client) connectStreams(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		remote := string(ch.ExtraData())
		stream, reqs, err := ch.Accept()
		if err != nil {
			c.Debugf("Failed to accept stream: %s", err)
			continue
		}
		go ssh.DiscardRequests(reqs)
		l := c.Logger.Fork("conn#%d", c.connStats.New())
		go chshare.HandleTCPStream(l, &c.connStats, stream, remote, net.Dial)
	}
}
