package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

type Server struct {
	*chisel.Logger
	auth       string
	wsCount    int
	wsServer   websocket.Server
	httpServer *chisel.HTTPServer
	proxy      *httputil.ReverseProxy
}

func NewServer(auth, proxy string) (*Server, error) {
	s := &Server{
		Logger:     chisel.NewLogger("server"),
		auth:       auth,
		wsServer:   websocket.Server{},
		httpServer: chisel.NewHTTPServer(),
	}
	s.wsServer.Handler = websocket.Handler(s.handleWS)

	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, s.Errorf("Missing protocol (%s)", u)
		}
		s.proxy = httputil.NewSingleHostReverseProxy(u)
		//always use proxy host
		s.proxy.Director = func(r *http.Request) {
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		}
	}

	return s, nil
}

func (s *Server) Run(host, port string) error {
	if err := s.Start(host, port); err != nil {
		return err
	}
	return s.Wait()
}

func (s *Server) Start(host, port string) error {
	if s.auth != "" {
		s.Infof("Authenication enabled")
	}
	if s.proxy != nil {
		s.Infof("Default proxy enabled")
	}
	s.Infof("Listening on %s...", port)

	return s.httpServer.GoListenAndServe(":"+port, http.HandlerFunc(s.handleHTTP))
}

func (s *Server) Wait() error {
	return s.httpServer.Wait()
}

func (s *Server) Close() error {
	//this should cause an error in the open websockets
	return s.httpServer.Close()
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	if r.Header.Get("Upgrade") == "websocket" &&
		strings.HasPrefix(r.Header.Get("Sec-webSocket-Protocol"), chisel.ConfigPrefix) {
		s.wsServer.ServeHTTP(w, r)
		return
	}
	//proxy target was provided
	if s.proxy != nil {
		s.proxy.ServeHTTP(w, r)
		return
	}
	//missing :O
	w.WriteHeader(404)
}

func (s *Server) handshake(h string) (*chisel.Config, error) {
	if h == "" {
		return nil, s.Errorf("Handshake missing")
	}
	c, err := chisel.DecodeConfig(h)
	if err != nil {
		return nil, err
	}
	if chisel.ProtocolVersion != c.Version {
		return nil, s.Errorf("Version mismatch")
	}
	if s.auth != "" {
		if s.auth != c.Auth {
			return nil, s.Errorf("Authentication failed")
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

	// s.Infof("success %+v\n", config)
	ws.Write([]byte("handshake-success"))
	s.wsCount++

	newWebSocket(s, config, ws).handle()
}
