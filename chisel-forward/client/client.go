package client

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Client struct {
	config  *chisel.Config
	proxies []*Proxy
}

func NewClient(auth, server string, remotes []string) (*Client, error) {

	u, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	//apply default port
	if !regexp.MustCompile(`:\d+$`).MatchString(u.Host) {
		if u.Scheme == "https" {
			u.Host = u.Host + ":443"
		} else {
			u.Host = u.Host + ":80"
		}
	}

	//use websockets scheme
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)

	c := &chisel.Config{
		Version: chisel.Version,
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

	return &Client{config: c}, nil
}

func (c *Client) Start() error {
	encconfig, err := chisel.EncodeConfig(c.config)
	if err != nil {
		return err
	}

	url := strings.Replace(c.config.Server, "http", "ws", 1)
	ws, err := websocket.Dial(url, encconfig, "http://localhost/")
	if err != nil {
		return err
	}

	b := make([]byte, 0xff)
	n, _ := ws.Read(b)
	if msg := string(b[:n]); msg != "handshake-success" {
		return errors.New(msg)
	}

	// Setup client side of yamux
	session, err := yamux.Client(ws, nil)
	if err != nil {
		return err
	}

	//closed state
	markClosed := make(chan bool)
	var o sync.Once
	closed := func() {
		close(markClosed)
	}
	go func() {
		for {
			if session.IsClosed() {
				o.Do(closed)
				break
			}
			time.Sleep(time.Second)
		}
	}()

	//proxies all use this function
	openStream := func() (net.Conn, error) {
		stream, err := session.Open()
		if err != nil {
			o.Do(closed)
			return nil, err
		}
		return stream, nil
	}

	//prepare proxies
	for id, r := range c.config.Remotes {
		proxy := NewProxy(id, r, openStream)
		go proxy.start()
		c.proxies = append(c.proxies, proxy)
	}

	fmt.Printf("Connected to %s\n", c.config.Server)
	<-markClosed
	fmt.Printf("Disconnected\n")
	return nil
}
