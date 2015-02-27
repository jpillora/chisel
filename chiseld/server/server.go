package server

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Server struct {
	auth      string
	wsServer  websocket.Server
	endpoints []*Endpoint
}

func NewServer(auth string) *Server {
	s := &Server{
		auth:     auth,
		wsServer: websocket.Server{},
	}
	s.wsServer.Handler = websocket.Handler(s.handleWS)
	return s
}

func (s *Server) Start(host, port string) {
	if s.auth != "" {
		log.Println("Authenication activated")
	}
	log.Println("Listening on " + port)
	log.Fatal(http.ListenAndServe(":"+port, http.HandlerFunc(s.handleHTTP)))
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	//route by header
	if r.Header.Get("Upgrade") == "websocket" {
		s.wsServer.ServeHTTP(w, r)
	} else {
		w.WriteHeader(200)
		w.Write([]byte("hello world\n"))
	}
}

func (s *Server) handshake(h string) (*chisel.Config, error) {
	if h == "" {
		return nil, fmt.Errorf("Handshake missing")
	}
	c, err := chisel.DecodeConfig(h)
	if err != nil {
		return nil, err
	}
	if chisel.Version != c.Version {
		return nil, fmt.Errorf("Version mismatch")
	}
	if s.auth != "" {
		if s.auth != c.Auth {
			return nil, fmt.Errorf("Authentication failed")
		}
	}
	return c, nil
}

func (s *Server) handleWS(ws *websocket.Conn) {

	ps := ws.Config().Protocol
	p := ""
	if len(ps) == 1 {
		p = ps[0]
	}

	config, err := s.handshake(p)
	if err != nil {
		msg := "Handshake denied: " + err.Error()
		ws.Write([]byte(msg))
		ws.Close()
		return
	}

	log.Printf("success %+v\n", config)
	ws.Write([]byte("handshake-success"))

	// Setup server side of yamux
	session, err := yamux.Server(ws, nil)
	if err != nil {
		log.Fatalf("Yamux server: %s", err)
	}

	// Create an endpoint for each required
	for id, r := range config.Remotes {
		addr := r.RemoteHost + ":" + r.RemotePort
		e := NewEndpoint(id, addr)
		go e.start()
		s.endpoints = append(s.endpoints, e)
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			if session.IsClosed() {
				log.Printf("Websocket closed")
				break
			}
			log.Printf("Session accept: %s", err)
			continue
		}
		go s.handleStream(stream)
	}
}

func (s *Server) handleStream(stream net.Conn) {
	// extract endpoint id
	b := make([]byte, 2)
	n, err := stream.Read(b)
	if err != nil {
		log.Fatalf("Stream initial read: %s", err)
		return
	}
	if n != 2 {
		log.Println("Should read 2 bytes...")
		return
	}
	id := binary.BigEndian.Uint16(b)

	//then pipe
	e := s.endpoints[id]
	e.session <- stream
}
