package chserver

import (
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"

	"github.com/gorilla/websocket"
	chshare "github.com/jpillora/chisel/share"
	"github.com/jpillora/requestlog"
	"golang.org/x/crypto/ssh"
)

// Config is the configuration for the chisel service
type Config struct {
	KeySeed  string
	AuthFile string
	Auth     string
	Proxy    string
	Socks5   bool
	Reverse  bool
}

// Server respresent a chisel service
type Server struct {
	*chshare.Logger
	config       *Config
	fingerprint  string
	httpServer   *chshare.HTTPServer
	reverseProxy *httputil.ReverseProxy
	sessCount    int32
	sessions     *chshare.Users
	sshConfig    *ssh.ServerConfig
	users        *chshare.UserIndex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewServer creates and returns a new chisel server
func NewServer(config *Config) (*Server, error) {
	server := &Server{
		config:     config,
		httpServer: chshare.NewHTTPServer(),
		Logger:     chshare.NewLogger("server"),
		sessions:   chshare.NewUsers(),
	}
	server.Info = true
	server.users = chshare.NewUserIndex(server.Logger)
	if config.AuthFile != "" {
		if err := server.users.LoadUsers(config.AuthFile); err != nil {
			return nil, err
		}
	}
	if config.Auth != "" {
		u := &chshare.User{Addrs: []*regexp.Regexp{chshare.UserAllowAll}}
		u.Name, u.Pass = chshare.ParseAuth(config.Auth)
		if u.Name != "" {
			server.users.AddUser(u)
		}
	}
	//generate private key (optionally using seed)
	key, err := chshare.GenerateKey(config.KeySeed)
	if err != nil {
		log.Fatal("Failed to generate key")
	}
	//convert into ssh.PrivateKey
	private, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatal("Failed to parse key")
	}
	//fingerprint this key
	server.fingerprint = chshare.FingerprintKey(private.PublicKey())
	//create ssh config
	server.sshConfig = &ssh.ServerConfig{
		ServerVersion:    "SSH-" + chshare.ProtocolVersion + "-server",
		PasswordCallback: server.authUser,
	}
	server.sshConfig.AddHostKey(private)
	//setup reverse proxy
	if config.Proxy != "" {
		u, err := url.Parse(config.Proxy)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, server.Errorf("Missing protocol (%s)", u)
		}
		server.reverseProxy = httputil.NewSingleHostReverseProxy(u)
		//always use proxy host
		server.reverseProxy.Director = func(r *http.Request) {
			//enforce origin, keep path
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		}
	}
	//print when reverse tunnelling is enabled
	if config.Reverse {
		server.Infof("Reverse tunnelling enabled")
	}
	return server, nil
}

// Run is responsible for starting the chisel service
func (s *Server) Run(host, port string) error {
	if err := s.Start(host, port); err != nil {
		return err
	}
	return s.Wait()
}

// Start is responsible for kicking off the http server
func (s *Server) Start(host, port string) error {
	s.Infof("Fingerprint %s", s.fingerprint)
	if s.users.Len() > 0 {
		s.Infof("User authenication enabled")
	}
	if s.reverseProxy != nil {
		s.Infof("Reverse proxy enabled")
	}
	s.Infof("Listening on %s:%s...", host, port)
	h := http.Handler(http.HandlerFunc(s.handleClientHandler))
	if s.Debug {
		o := requestlog.DefaultOptions
		o.TrustProxy = true
		h = requestlog.WrapWith(h, o)
	}
	return s.httpServer.GoListenAndServe(host+":"+port, h)
}

// Wait waits for the http server to close
func (s *Server) Wait() error {
	return s.httpServer.Wait()
}

// Close forcibly closes the http server
func (s *Server) Close() error {
	return s.httpServer.Close()
}

// GetFingerprint is used to access the server fingerprint
func (s *Server) GetFingerprint() string {
	return s.fingerprint
}

// authUser is responsible for validating the ssh user / password combination
func (s *Server) authUser(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	// check if user authenication is enable and it not allow all
	if s.users.Len() == 0 {
		return nil, nil
	}
	// check the user exists and has matching password
	n := c.User()
	user, found := s.users.Get(n)
	if !found || user.Pass != string(password) {
		s.Debugf("Login failed for user: %s", n)
		return nil, errors.New("Invalid authentication for username: %s")
	}
	// insert the user session map
	// TODO this should probably have a lock on it given the map isn't thread-safe
	s.sessions.Set(string(c.SessionID()), user)
	return nil, nil
}

// AddUser adds a new user into the server user index
func (s *Server) AddUser(user, pass string, addrs ...string) error {
	authorizedAddrs := []*regexp.Regexp{}
	for _, addr := range addrs {
		authorizedAddr, err := regexp.Compile(addr)
		if err != nil {
			return err
		}
		authorizedAddrs = append(authorizedAddrs, authorizedAddr)
	}
	s.users.AddUser(&chshare.User{
		Name:  user,
		Pass:  pass,
		Addrs: authorizedAddrs,
	})
	return nil
}

// DeleteUser removes a user from the server user index
func (s *Server) DeleteUser(user string) {
	s.users.Del(user)
}
