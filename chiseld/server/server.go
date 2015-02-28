package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Server struct {
	auth     string
	wsCount  int
	wsServer websocket.Server
	proxy    *httputil.ReverseProxy
}

func NewServer(auth, proxy string) (*Server, error) {
	s := &Server{
		auth:     auth,
		wsServer: websocket.Server{},
	}
	s.wsServer.Handler = websocket.Handler(s.handleWS)

	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		s.proxy = httputil.NewSingleHostReverseProxy(u)
	}

	return s, nil
}

func (s *Server) Start(host, port string) error {
	if s.auth != "" {
		chisel.Printf("Authenication enabled\n")
	}
	if s.proxy != nil {
		chisel.Printf("Default proxy enabled\n")
	}
	chisel.Printf("Listening on %s...\n", port)
	return http.ListenAndServe(":"+port, http.HandlerFunc(s.handleHTTP))
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	if r.Header.Get("Upgrade") == "websocket" &&
		strings.HasPrefix(r.Header.Get("Sec-WebSocket-Protocol"), chisel.ConfigPrefix) {
		s.wsServer.ServeHTTP(w, r)
		return
	}
	if s.proxy != nil {
		s.proxy.ServeHTTP(w, r)
		return
	}
	//missing :O
	w.WriteHeader(404)
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
		msg := err.Error()
		ws.Write([]byte(msg))
		ws.Close()
		return
	}

	// chisel.Printf("success %+v\n", config)
	ws.Write([]byte("handshake-success"))
	s.wsCount++

	NewWebSocket(s.wsCount, config, ws).handle()
}
