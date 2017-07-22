package chserver

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	socks5 "github.com/armon/go-socks5"
	chshare "github.com/jpillora/chisel/share"
	"github.com/jpillora/requestlog"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	KeySeed  string
	AuthFile string
	Auth     string
	Proxy    string
	Socks5   bool
}

type Server struct {
	*chshare.Logger
	//Users is an empty map of usernames to Users
	//It can be optionally initialized using the
	//file found at AuthFile
	Users    chshare.Users
	sessions chshare.Users

	fingerprint  string
	sessCount    int32
	httpServer   *chshare.HTTPServer
	reverseProxy *httputil.ReverseProxy
	sshConfig    *ssh.ServerConfig
	socksServer  *socks5.Server
}

func NewServer(config *Config) (*Server, error) {
	s := &Server{
		Logger:     chshare.NewLogger("server"),
		httpServer: chshare.NewHTTPServer(),
		sessions:   chshare.Users{},
	}
	s.Info = true

	//parse users, if provided
	if config.AuthFile != "" {
		users, err := chshare.ParseUsers(config.AuthFile)
		if err != nil {
			return nil, err
		}
		s.Users = users
	}
	//parse single user, if provided
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
		ServerVersion: chshare.ProtocolVersion + "-server",
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
		socksConfig := &socks5.Config{
			Rules: &socksRule{s},
		}
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
	//ready!
	return s, nil
}

func (s *Server) Run(host, port string) error {
	if err := s.Start(host, port); err != nil {
		return err
	}
	return s.Wait()
}

func (s *Server) Start(host, port string) error {
	s.Infof("Fingerprint %s", s.fingerprint)
	if len(s.Users) > 0 {
		s.Infof("User authenication enabled")
	}
	if s.reverseProxy != nil {
		s.Infof("Reverse proxy enabled")
	}
	s.Infof("Listening on %s...", port)

	h := http.Handler(http.HandlerFunc(s.handleHTTP))
	if s.Debug {
		h = requestlog.Wrap(h)
	}
	return s.httpServer.GoListenAndServe(host+":"+port, h)
}

func (s *Server) Wait() error {
	return s.httpServer.Wait()
}

func (s *Server) Close() error {
	//this should cause an error in the open websockets
	return s.httpServer.Close()
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	//websockets upgrade AND has chisel prefix
	if upgrade == "websocket" && protocol == chshare.ProtocolVersion {
		s.handleWS(w, r)
		return
	}
	//proxy target was provided
	if s.reverseProxy != nil {
		s.reverseProxy.ServeHTTP(w, r)
		return
	}
	//missing :O
	w.WriteHeader(404)
	w.Write([]byte("Not found"))
}

func (s *Server) handleWS(w http.ResponseWriter, req *http.Request) {
	id := atomic.AddInt32(&s.sessCount, 1)
	session := newSession(s, int(id))
	//inc session count
	if err := session.handle(w, req); err != nil {
		if err != io.EOF {
			session.Debugf("%s", err)
		}
	}
	//dec session count
}

type socksRule struct {
	*Server
}

func (s *socksRule) Allow(ctx context.Context, req *socks5.Request) (context.Context, bool) {
	//only connect is allowed
	if req.Command != socks5.ConnectCommand {
		return ctx, false
	}
	//TODO check req.DestAddr
	//need to add user object into ctx value, need to customise go-socks5
	return ctx, true
}
