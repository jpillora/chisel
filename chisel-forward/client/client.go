package chiselclient

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jpillora/backoff"
	"github.com/jpillora/chisel"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type Client struct {
	*chisel.Logger
	config      *chisel.Config
	sshConfig   *ssh.ClientConfig
	proxies     []*Proxy
	sshConn     ssh.Conn
	fingerprint string
	running     bool
	runningc    chan error
}

func NewClient(auth, server string, remotes ...string) (*Client, error) {

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

	config := &chisel.Config{
		Version: chisel.ProtocolVersion,
		Auth:    auth,
		Server:  u.String(),
	}

	for _, s := range remotes {
		r, err := chisel.DecodeRemote(s)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode remote '%s': %s", s, err)
		}
		config.Remotes = append(config.Remotes, r)
	}

	c := &Client{
		Logger:   chisel.NewLogger("client"),
		config:   config,
		running:  true,
		runningc: make(chan error, 1),
	}

	c.sshConfig = &ssh.ClientConfig{
		ClientVersion: "chisel-client-" + chisel.ProtocolVersion,
		User:          "jpillora",
		Auth:          []ssh.AuthMethod{ssh.Password("t0ps3cr3t")},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			c.fingerprint = chisel.FingerprintKey(key)
			return nil
		},
	}

	return c, nil
}

//Start then Wait
func (c *Client) Run() error {
	go c.start()
	return c.Wait()
}

//Starts the client
func (c *Client) Start() {
	go c.start()
}

func (c *Client) start() {
	c.Infof("Connecting to %s\n", c.config.Server)

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

		ws, err := websocket.Dial(c.config.Server, chisel.ConfigPrefix, "localhost:80")
		if err != nil {
			connerr = err
			continue
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(ws, "", c.sshConfig)
		if err != nil {
			connerr = err
			c.Infof("Handshake failed: %s", err)
			continue
		}
		c.Infof("Connected (%s)", c.fingerprint)
		//connected
		b.Reset()
		c.sshConn = sshConn
		go ssh.DiscardRequests(reqs)
		go chisel.RejectStreams(chans)
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
