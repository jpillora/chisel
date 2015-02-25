package client

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Client struct {
	config  *chisel.Config
	proxies []*ProxyServer
	ws      *websocket.Conn
	session *yamux.Session
}

func NewClient(auth, server string, args []string) *Client {

	c := &chisel.Config{
		Version: chisel.Version,
		Auth:    auth,
		Server:  server,
	}

	for _, a := range args {
		r, err := chisel.DecodeRemote(a)
		if err != nil {
			log.Fatalf("Remote decode failed: %s", err)
		}
		c.Remotes = append(c.Remotes, r)
	}

	return &Client{config: c}
}

func (c *Client) Start() {
	s, err := chisel.EncodeConfig(c.config)
	if err != nil {
		log.Fatal(err)
	}

	url := strings.Replace(c.config.Server, "http:", "ws:", 1)
	c.ws, err = websocket.Dial(url, s, "http://localhost/")
	if err != nil {
		log.Fatal(err)
	}

	// Setup client side of yamux
	c.session, err = yamux.Client(c.ws, nil)
	if err != nil {
		log.Fatal(err)
	}

	//prepare proxies
	for id, r := range c.config.Remotes {
		if err := c.setupProxy(id, r, c.session); err != nil {
			log.Fatal(err)
		}
	}

	// ws.Write([]byte("hello!"))

	fmt.Printf("Connected to %s\n", c.config.Server)
	c.handleData()
	fmt.Printf("Disconnected\n")
}

func (c *Client) setupProxy(id int, r *chisel.Remote, session *yamux.Session) error {

	addr := r.LocalHost + ":" + r.LocalPort

	proxy := NewProxyServer(id, addr, session)
	//watch conn for writes
	// go func() {
	// 	for b := range conn.out {
	// 		//encode
	// 		c.ws.Write(b)
	// 	}
	// }()

	go proxy.start()
	c.proxies = append(c.proxies, proxy)
	return nil
}

func (c *Client) handleData() {

	buff := make([]byte, 0xffff)
	for {
		n, err := c.ws.Read(buff)

		if err != nil {
			break
		}

		b := buff[:n]

		fmt.Printf("%s\n", b)

		//decode
		//place on proxy's read queue
	}
}
