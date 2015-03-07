package client

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/backoff"
	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Client struct {
	*chisel.Logger
	config  *chisel.Config
	proxies []*Proxy
	session *yamux.Session
}

func NewClient(auth, server string, remotes []string) (*Client, error) {

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

	c := &chisel.Config{
		Version: chisel.ProtocolVersion,
		Auth:    auth,
		Server:  u.String(),
	}

	for _, s := range remotes {
		r, err := chisel.DecodeRemote(s)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode remote '%s': %s", s, err)
		}
		c.Remotes = append(c.Remotes, r)
	}

	return &Client{
		Logger: chisel.NewLogger("client"),
		config: c,
	}, nil
}

func (c *Client) Start() error {
	encconfig, err := chisel.EncodeConfig(c.config)
	if err != nil {
		return c.Errorf("%s", err)
	}

	c.Infof("Connecting to %s\n", c.config.Server)

	var session *yamux.Session

	//proxies all use this function
	openStream := func() (net.Conn, error) {
		if session == nil || session.IsClosed() {
			return nil, c.Errorf("no session")
		}
		stream, err := session.Open()
		if err != nil {
			return nil, err
		}
		return stream, nil
	}

	//prepare proxies
	for id, r := range c.config.Remotes {
		proxy := NewProxy(c, id+1, r, openStream)
		go proxy.start()
		c.proxies = append(c.proxies, proxy)
	}

	var connerr error
	b := &backoff.Backoff{Max: 5 * time.Minute}

	//connection loop!
	for {

		if connerr != nil {
			connerr = nil
			d := b.Duration()
			c.Debugf("Retrying in %s...\n", d)
			time.Sleep(d)
		}

		ws, err := websocket.Dial(c.config.Server, encconfig, "http://localhost/")
		if err != nil {
			connerr = err
			continue
		}

		buff := make([]byte, 0xff)
		n, _ := ws.Read(buff)
		if msg := string(buff[:n]); msg != "handshake-success" {
			return c.Errorf("%s", msg) //no point in retrying
		}

		// Setup client side of yamux
		session, err = yamux.Client(ws, nil)
		if err != nil {
			connerr = err
			continue
		}
		b.Reset()

		//closed state
		markClosed := make(chan bool)
		var o sync.Once
		closed := func() {
			c.Infof("Disconnected\n")
			close(markClosed)
		}

		c.Infof("Connected\n")

		//poll state
		go func() {
			for {
				if session.IsClosed() {
					connerr = c.Errorf("disconnected")
					o.Do(closed)
					break
				}
				time.Sleep(time.Second)
			}
		}()

		//block!
		<-markClosed
	}

	return nil
}
