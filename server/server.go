package chserver

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"

	socks5 "github.com/armon/go-socks5"
	"github.com/gorilla/websocket"
	"github.com/jpillora/requestlog"
	"golang.org/x/crypto/ssh"

	"github.com/jpillora/chisel/share"
)

// Config is the configuration for the chisel service
type Config struct {
	KeySeed       string
	AuthFile      string
	Auth          string
	Proxy         string
	UpstreamProxy string
	Socks5        bool
	Reverse       bool
}

// Server respresent a chisel service
type Server struct {
	*chshare.Logger
	connStats        chshare.ConnStats
	fingerprint      string
	httpServer       *chshare.HTTPServer
	reverseProxy     *httputil.ReverseProxy
	upstreamProxyUrl *url.URL
	upstreamDial     chshare.FnDial
	sessCount        int32
	sessions         *chshare.Users
	socksServer      *socks5.Server
	sshConfig        *ssh.ServerConfig
	users            *chshare.UserIndex
	reverseOk        bool
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// NewServer creates and returns a new chisel server
func NewServer(config *Config) (*Server, error) {
	s := &Server{
		httpServer: chshare.NewHTTPServer(),
		Logger:     chshare.NewLogger("server"),
		sessions:   chshare.NewUsers(),
		reverseOk:  config.Reverse,
	}
	s.Info = true
	s.users = chshare.NewUserIndex(s.Logger)
	if config.AuthFile != "" {
		if err := s.users.LoadUsers(config.AuthFile); err != nil {
			return nil, err
		}
	}
	if config.Auth != "" {
		u := &chshare.User{Addrs: []*regexp.Regexp{chshare.UserAllowAll}}
		u.Name, u.Pass = chshare.ParseAuth(config.Auth)
		if u.Name != "" {
			s.users.AddUser(u)
		}
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
		ServerVersion:    "SSH-" + chshare.ProtocolVersion + "-server",
		PasswordCallback: s.authUser,
	}
	s.sshConfig.AddHostKey(private)
	//setup reverse proxy
	if config.Proxy != "" {
		u, err := url.Parse(config.Proxy)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, s.Errorf("Missing protocol (%s)", u)
		}
		s.reverseProxy = httputil.NewSingleHostReverseProxy(u)
		//always use proxy host
		s.reverseProxy.Director = func(r *http.Request) {
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		}
	}
	//setup upstream dial
	if config.UpstreamProxy != "" {
		if !strings.Contains(config.UpstreamProxy, "://") {
			config.UpstreamProxy = "socks5://" + config.UpstreamProxy
		}
		proxyURL, err := url.Parse(config.UpstreamProxy)
		if err != nil {
			return nil, err
		}
		if proxyURL.Host == "" {
			return nil, s.Errorf("Missing upstream proxy host (%s)", proxyURL)
		}
		if proxyURL.Scheme == "" || strings.HasPrefix(proxyURL.Scheme, "socks") {
			// SOCKS5 proxy
			dial, err := chshare.NewSocks5Dial(proxyURL)
			if err != nil {
				return nil, err
			}
			s.upstreamProxyUrl = proxyURL
			s.upstreamDial = dial
		} else {
			return nil, s.Errorf("Only SOCKS5 upstream proxy is supported (%s)", proxyURL)
		}
	}
	//setup socks server (not listening on any port!)
	if config.Socks5 {
		socksConfig := &socks5.Config{}
		if s.Debug {
			socksConfig.Logger = log.New(os.Stdout, "[socks]", log.Ldate|log.Ltime)
		} else {
			socksConfig.Logger = log.New(ioutil.Discard, "", 0)
		}
		if s.upstreamDial != nil {
			socksConfig.Dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return s.upstreamDial(network, addr)
			}
		}
		s.socksServer, err = socks5.New(socksConfig)
		if err != nil {
			return nil, err
		}
		s.Infof("SOCKS5 server enabled")
	}
	//print when reverse tunnelling is enabled
	if config.Reverse {
		s.Infof("Reverse tunnelling enabled")
	}
	return s, nil
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
	if s.upstreamProxyUrl != nil {
		s.Infof("Upstream proxy: " + s.upstreamProxyUrl.String())
	}
	s.Infof("Listening on %s:%s...", host, port)
	h := http.Handler(http.HandlerFunc(s.handleClientHandler))
	if s.Debug {
		h = requestlog.Wrap(h)
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
	// @note: this should probably have a lock on it given the map isn't thread-safe??
	s.sessions.Set(string(c.SessionID()), user)
	return nil, nil
}

// AddUser adds a new user into the server user index
func (s *Server) AddUser(user, pass string, addrs ...string) error {
	authorizedAddrs := make([]*regexp.Regexp, 0)

	for _, addr := range addrs {
		authorizedAddr, err := regexp.Compile(addr)
		if err != nil {
			return err
		}

		authorizedAddrs = append(authorizedAddrs, authorizedAddr)
	}

	u := &chshare.User{Name: user, Pass: pass, Addrs: authorizedAddrs}
	s.users.AddUser(u)
	return nil
}

// DeleteUser removes a user from the server user index
func (s *Server) DeleteUser(user string) {
	s.users.Del(user)
}
