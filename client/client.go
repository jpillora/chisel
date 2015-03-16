package chclient

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/backoff"
	"github.com/jpillora/chisel/share"
	"github.com/jpillora/conncrypt"
	"golang.org/x/net/websocket"
)

type Client struct {
	*chshare.Logger
	config      *chshare.Config
	encconfig   []byte
	key, server string
	proxies     []*Proxy
	session     *yamux.Session
	running     bool
	runningc    chan error
}

func NewClient(key, server string, remotes ...string) (*Client, error) {

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

	encconfig, err := chshare.EncodeConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to encode config: %s", err)
	}

	return &Client{
		Logger:    chshare.NewLogger("client"),
		config:    config,
		encconfig: encconfig,
		key:       key,
		server:    u.String(),
		running:   true,
		runningc:  make(chan error, 1),
	}, nil
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
	c.Infof("Connecting to %s\n", c.server)

	//proxies all use this function
	openStream := func() (net.Conn, error) {
		if c.session == nil || c.session.IsClosed() {
			return nil, c.Errorf("no session available")
		}
		stream, err := c.session.Open()
		if err != nil {
			return nil, err
		}
		return stream, nil
	}

	//prepare proxies
	for id, r := range c.config.Remotes {
		proxy := NewProxy(c, id, r, openStream)
		go proxy.start()
		c.proxies = append(c.proxies, proxy)
	}

	var connerr error
	b := &backoff.Backoff{Max: 15 * time.Second}

	//connection loop!
	for {
		if !c.running {
			break
		}
		if connerr != nil {
			d := b.Duration()
			c.Infof("Connerr: %v", connerr)
			c.Infof("Retrying in %s...", d)
			connerr = nil
			time.Sleep(d)
		}

		ws, err := websocket.Dial(c.server, chshare.ProtocolVersion, "http://localhost/")
		if err != nil {
			connerr = err
			continue
		}

		conn := net.Conn(ws)

		if c.key != "" {
			conn = conncrypt.New(conn, &conncrypt.Config{Password: c.key})
		}

		//write config, read result
		chshare.SizeWrite(conn, c.encconfig)

		resp := chshare.SizeRead(conn)
		if string(resp) != "Handshake Success" {
			//no point in retrying
			c.runningc <- errors.New("Handshake failed")
			conn.Close()
			break
		}

		// Setup client side of yamux
		c.session, err = yamux.Client(conn, nil)
		if err != nil {
			connerr = err
			continue
		}
		b.Reset()

		//check latency
		go func() {
			d, err := c.session.Ping()
			if err == nil {
				c.Infof("Connected (Latency: %s)\n", d)
			} else {
				c.Infof("Connected\n")
			}
		}()

		//signal is connected
		connected := make(chan bool)

		//poll websocket state
		go func() {
			for {
				if c.session.IsClosed() {
					connerr = c.Errorf("disconnected")
					c.Infof("Disconnected\n")
					close(connected)
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}()
		//block!
		<-connected
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
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}
