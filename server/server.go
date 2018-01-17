package chserver

import (
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"

	socks5 "github.com/armon/go-socks5"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jpillora/requestlog"
	"golang.org/x/crypto/ssh"

	"github.com/jpillora/chisel/share"
)

// Config is the configuration for the chisel service
type Config struct {
	KeySeed  string
	AuthFile string
	Auth     string
	Proxy    string
	Socks5   bool
}

// Server respresent a chisel service
type Server struct {
	*chshare.Logger
	//Users is an empty map of usernames to Users
	//It can be optionally initialized using the
	//file found at AuthFile
	Users    chshare.Users
	sessions chshare.Users

	fingerprint  string
	sessCount    int32
	connCount    int32
	connOpen     int32
	httpServer   *chshare.HTTPServer
	reverseProxy *httputil.ReverseProxy
	sshConfig    *ssh.ServerConfig
	socksServer  *socks5.Server
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
		sessions:   chshare.Users{},
	}
	s.Info = true

	if config.AuthFile != "" {
		users, err := chshare.ParseUsers(config.AuthFile)
		if err != nil {
			return nil, err
		}
		s.Users = users
	}

	if config.Auth != "" {
		u := &chshare.User{Addrs: []*regexp.Regexp{chshare.UserAllowAll}}
		u.Name, u.Pass = chshare.ParseAuth(config.Auth)
		if u.Name != "" {
			if s.Users == nil {
				s.Users = chshare.Users{}
			}
			s.Users[u.Name] = u
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
		ServerVersion:    chshare.ProtocolVersion + "-server",
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

	//setup socks server (not listening on any port!)
	if config.Socks5 {
		socksConfig := &socks5.Config{}
		if s.Debug {
			socksConfig.Logger = log.New(os.Stdout, "[socks]", log.Ldate|log.Ltime)
		} else {
			socksConfig.Logger = log.New(ioutil.Discard, "", 0)
		}
		s.socksServer, err = socks5.New(socksConfig)
		if err != nil {
			return nil, err
		}
		s.Infof("SOCKS5 Enabled")
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
	s.Infof("Chisel service ssh fingerprint %s", s.fingerprint)

	if len(s.Users) > 0 {
		s.Infof("User authenication has been enabled")
	}
	if s.reverseProxy != nil {
		s.Infof("Reverse proxy enabled")
	}

	s.Infof("Chisel service is listening on %s:%s...", host, port)

	e := mux.NewRouter()
	e.HandleFunc("/", s.handleClientHandler)
	e.HandleFunc("/health", s.healthHandler).Methods(http.MethodGet)
	e.HandleFunc("/version", s.versionHandler).Methods(http.MethodGet)

	if s.Debug {
		e.Use(requestlog.Wrap)
	}

	return s.httpServer.GoListenAndServe(host+":"+port, e)
}

// Wait waits for the http server to close
func (s *Server) Wait() error {
	return s.httpServer.Wait()
}

// Close forciable closes the http server
func (s *Server) Close() error {
	return s.httpServer.Close()
}

// authUser is responsible for validating the ssh user / password combination
func (s *Server) authUser(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	// @check if user authenication is enable and it not allow all
	if len(s.Users) == 0 {
		return nil, nil
	}

	// @check the user is authenicated
	n := c.User()
	user, found := s.Users[n]
	if !found || user.Pass != string(password) {
		s.Debugf("Login failed for user: %s", n)

		return nil, errors.New("Invalid authentication for username: %s")
	}

	// insert the user session map
	// @note: this should probably have a lock on it given the map isn't thread-safe??
	s.sessions[string(c.SessionID())] = user

	return nil, nil
}
