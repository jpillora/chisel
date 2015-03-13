package chiselserver

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/jpillora/chisel"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type Server struct {
	*chisel.Logger
	auth        string
	fingerprint string
	wsCount     int
	wsServer    websocket.Server
	httpServer  *chisel.HTTPServer
	proxy       *httputil.ReverseProxy
	sshConfig   *ssh.ServerConfig
}

func NewServer(auth, proxy string) (*Server, error) {

	s := &Server{
		Logger:     chisel.NewLogger("server"),
		auth:       auth,
		wsServer:   websocket.Server{},
		httpServer: chisel.NewHTTPServer(),
	}
	s.wsServer.Handler = websocket.Handler(s.handleWS)

	key, _ := chisel.GenerateKey()

	config := &ssh.ServerConfig{
		ServerVersion: "chisel-server-" + chisel.ProtocolVersion,
		NoClientAuth:  true,
	}
	private, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatal("Failed to parse key")
	}

	s.fingerprint = chisel.FingerprintKey(private.PublicKey())

	config.AddHostKey(private)

	s.sshConfig = config

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
	s.Infof("Fingerprint %s", s.fingerprint)

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

func (s *Server) handleWS(ws *websocket.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sshConn, chans, reqs, err := ssh.NewServerConn(ws, s.sshConfig)
	if err != nil {
		log.Printf("Failed to handshake (%s)", err)
		return
	}

	s.wsCount++
	id := s.wsCount
	l := s.Fork("conn#%d", id)

	l.Infof("Open")

	go ssh.DiscardRequests(reqs)
	go chisel.ConnectStreams(l, chans)

	sshConn.Wait()
	l.Infof("Close")
}
