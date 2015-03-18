package chclient

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jpillora/backoff"
	"github.com/jpillora/chisel/share"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type Client struct {
	*chshare.Logger
	config      *chshare.Config
	sshConfig   *ssh.ClientConfig
	proxies     []*Proxy
	sshConn     ssh.Conn
	fingerprint string
	server      string
	keyPrefix   string
	running     bool
	runningc    chan error
}

func NewClient(keyPrefix, auth, server string, remotes ...string) (*Client, error) {

	//apply default scheme
	if !strings.HasPrefix(server, "http") {
		server = "http://" + server
	}

	u, err := url.Parse(server)
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

	config := &chshare.Config{}
	for _, s := range remotes {
		r, err := chshare.DecodeRemote(s)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode remote '%s': %s", s, err)
		}
		config.Remotes = append(config.Remotes, r)
	}

	c := &Client{
		Logger:    chshare.NewLogger("client"),
		config:    config,
		server:    u.String(),
		keyPrefix: keyPrefix,
		running:   true,
		runningc:  make(chan error, 1),
	}

	c.sshConfig = &ssh.ClientConfig{
		ClientVersion:   chshare.ProtocolVersion + "-client",
		HostKeyCallback: c.verifyServer,
	}

	user, pass := chshare.ParseAuth(auth)
	if user != "" {
		c.sshConfig.User = user
		c.sshConfig.Auth = []ssh.AuthMethod{ssh.Password(pass)}
	}

	return c, nil
}

//Start then Wait
func (c *Client) Run() error {
	go c.start()
	return c.Wait()
}

func (c *Client) verifyServer(hostname string, remote net.Addr, key ssh.PublicKey) error {
	f := chshare.FingerprintKey(key)
	if c.keyPrefix != "" && !strings.HasPrefix(f, c.keyPrefix) {
		return fmt.Errorf("Invalid fingerprint (Got %s)", f)
	}
	c.fingerprint = f
	return nil
}

//Starts the client
func (c *Client) Start() {
	go c.start()
}

func (c *Client) start() {
	c.Infof("Connecting to %s\n", c.server)

	//prepare proxies
	for id, r := range c.config.Remotes {
		proxy := NewProxy(c, id, r)
		go proxy.start()
		c.proxies = append(c.proxies, proxy)
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

		ws, err := websocket.Dial(c.server, chshare.ProtocolVersion, "http://localhost/")
		if err != nil {
			connerr = err
			continue
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(ws, "", c.sshConfig)
		//NOTE break -> dont retry on handshake failures
		if err != nil {
			if strings.Contains(err.Error(), "unable to authenticate") {
				c.Infof("Authentication failed")
			} else {
				c.Infof(err.Error())
			}
			break
		}
		conf, _ := chshare.EncodeConfig(c.config)
		_, conerr, err := sshConn.SendRequest("config", true, conf)
		if err != nil {
			c.Infof("Config verification failed", c.fingerprint)
			break
		}
		if len(conerr) > 0 {
			c.Infof(string(conerr))
			break
		}

		c.Infof("Connected (%s)", c.fingerprint)
		//connected
		b.Reset()

		c.sshConn = sshConn
		go ssh.DiscardRequests(reqs)
		go chshare.RejectStreams(chans)
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
