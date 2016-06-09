package chserver

import (
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/jpillora/chisel/share"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type Config struct {
	KeySeed  string
	AuthFile string
	Proxy    string
}

type Server struct {
	*chshare.Logger
	//Users is an empty map of usernames to Users
	//It can be optionally initialized using the
	//file found at AuthFile
	Users chshare.Users

	fingerprint string
	wsCount     int
	wsServer    websocket.Server
	httpServer  *chshare.HTTPServer
	proxy       *httputil.ReverseProxy
	sshConfig   *ssh.ServerConfig
	sessions    map[string]*chshare.User
}

func NewServer(config *Config) (*Server, error) {
	s := &Server{
		Logger:     chshare.NewLogger("server"),
		wsServer:   websocket.Server{},
		httpServer: chshare.NewHTTPServer(),
		sessions:   map[string]*chshare.User{},
	}
	s.wsServer.Handler = websocket.Handler(s.handleWS)

	//parse users, if provided
	if config.AuthFile != "" {
		users, err := chshare.ParseUsers(config.AuthFile)
		if err != nil {
			return nil, err
		}
		s.Users = users
	}

	//generate private key (optionally using seed)
	key, _ := chshare.GenerateKey(config.KeySeed)
	//convert into ssh.PrivateKey
	private, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatal("Failed to parse key")
	}
	//fingerprint this key
	s.fingerprint = chshare.FingerprintKey(private.PublicKey())
	//create ssh config
	s.sshConfig = &ssh.ServerConfig{
		ServerVersion:    chshare.ProtocolVersion + "-server",
		PasswordCallback: s.authUser,
	}
	s.sshConfig.AddHostKey(private)

	if config.Proxy != "" {
		u, err := url.Parse(config.Proxy)
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

func (s *Server) Run(host string, port string) error {
	if err := s.Start(host, port); err != nil {
		return err
	}
	return s.Wait()
}

func (s *Server) Start(host string, port string) error {
	s.Infof("Fingerprint %s", s.fingerprint)
	if len(s.Users) > 0 {
		s.Infof("User authenication enabled")
	}
	if s.proxy != nil {
		s.Infof("Default proxy enabled")
	}
	s.Infof("Listening on %s...", port)

	return s.httpServer.GoListenAndServe(host+":"+port, http.HandlerFunc(s.handleHTTP))
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
		r.Header.Get("Sec-WebSocket-Protocol") == chshare.ProtocolVersion {
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

//
func (s *Server) authUser(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	// no auth - allow all
	if len(s.Users) == 0 {
		return nil, nil
	}
	// authenticate user
	n := c.User()
	u, ok := s.Users[n]
	if !ok || u.Pass != string(pass) {
		s.Debugf("Login failed: %s", n)
		return nil, errors.New("Invalid auth")
	}
	//insert session
	s.sessions[string(c.SessionID())] = u
	return nil, nil
}

func (s *Server) handleWS(ws *websocket.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sshConn, chans, reqs, err := ssh.NewServerConn(ws, s.sshConfig)
	if err != nil {
		s.Debugf("Failed to handshake (%s)", err)
		return
	}

	//load user
	var user *chshare.User
	if len(s.Users) > 0 {
		sid := string(sshConn.SessionID())
		user = s.sessions[sid]
		defer delete(s.sessions, sid)
	}

	//verify configuration
	s.Debugf("Verifying configuration")

	//wait for request, with timeout
	var r *ssh.Request
	select {
	case r = <-reqs:
	case <-time.After(10 * time.Second):
		sshConn.Close()
		return
	}

	failed := func(err error) {
		r.Reply(false, []byte(err.Error()))
	}
	if r.Type != "config" {
		failed(s.Errorf("expecting config request"))
		return
	}
	c, err := chshare.DecodeConfig(r.Payload)
	if err != nil {
		failed(s.Errorf("invalid config"))
		return
	}
	//if user is provided, ensure they have
	//access to the desired remotes
	if user != nil {
		for _, r := range c.Remotes {
			addr := r.RemoteHost + ":" + r.RemotePort
			if !user.HasAccess(addr) {
				failed(s.Errorf("access to '%s' denied", addr))
				return
			}
		}
	}
	//success!
	r.Reply(true, nil)

	//prepare connection logger
	s.wsCount++
	id := s.wsCount
	l := s.Fork("session#%d", id)

	l.Debugf("Open")

	go func() {
		for r := range reqs {
			switch r.Type {
			case "ping":
				r.Reply(true, nil)
			default:
				l.Debugf("Unknown request: %s", r.Type)
			}
		}
	}()

	go chshare.ConnectStreams(l, chans)
	sshConn.Wait()
	l.Debugf("Close")
}
