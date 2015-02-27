package client

import (
	"log"
	"net"
	"strings"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Client struct {
	config  *chisel.Config
	proxies []*Proxy
}

func NewClient(auth, server string, remotes []string) *Client {

	c := &chisel.Config{
		Version: chisel.Version,
		Auth:    auth,
		Server:  server,
	}

	for _, s := range remotes {
		r, err := chisel.DecodeRemote(s)
		if err != nil {
			log.Fatalf("Failed to decode remote '%s': %s", s, err)
		}
		c.Remotes = append(c.Remotes, r)
	}

	return &Client{config: c}
}

func (c *Client) Start() {
	encconfig, err := chisel.EncodeConfig(c.config)
	if err != nil {
		log.Fatal(err)
	}

	url := strings.Replace(c.config.Server, "http:", "ws:", 1)
	ws, err := websocket.Dial(url, encconfig, "http://localhost/")
	if err != nil {
		log.Fatal(err)
	}

	b := make([]byte, 0xff)
	n, _ := ws.Read(b)
	if msg := string(b[:n]); msg != "handshake-success" {
		log.Fatal(msg)
	}

	// Setup client side of yamux
	session, err := yamux.Client(ws, nil)
	if err != nil {
		log.Fatal(err)
	}

	markClosed := make(chan bool)
	isClosed := false

	//proxies all use this function
	openStream := func() (net.Conn, error) {
		stream, err := session.Open()
		if err != nil {
			if !isClosed {
				close(markClosed)
			}
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

	log.Printf("Connected to %s\n", c.config.Server)
	<-markClosed
	isClosed = true
	log.Printf("Disconnected\n")
}
