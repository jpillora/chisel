package socks5

import (
	"bufio"
	"context"
	"fmt"
	"github.com/jpillora/chisel/share/socks5/scope"
	"net"
)

const (
	socks5Version = uint8(5)
)

// ErrorLogger error handler, compatible with std logger
type ErrorLogger interface {
	Printf(format string, v ...interface{})
}

type UDPSendBackTo func(remoteAddr *AddrSpec, data []byte, client *net.UDPAddr) error

// Config is used to setup and configure a Server
type Config struct {
	// can be provided to implement custom authentication
	// By default, "auth-less" mode is enabled.
	// For password-based auth use UserPassAuthenticator.
	AuthMethods []Authenticator

	// If provided, username/password authentication is enabled,
	// by appending a UserPassAuthenticator to AuthMethods. If not provided,
	// and AUthMethods is nil, then "auth-less" mode is enabled.
	Credentials CredentialStore

	// can be provided to do custom name resolution.
	// Defaults to NoOpResolver if not provided.
	Resolver NameResolver

	// Rules is provided to enable custom logic around permitting
	// various commands. If not provided, PermitAll is used.
	Rules RuleSet

	// can be used to transparently rewrite addresses.
	// This is invoked before the RuleSet is invoked.
	// Defaults to NoRewrite.
	Rewriter AddressRewriter

	// server queries handler
	// Defaults to SinglePortUDPHandler
	Handler Handler
}

// Server is responsible for accepting connections and handling
// the details of the SOCKS5 protocol
type Server struct {
	config      *Config
	authMethods map[uint8]Authenticator
}

// New creates a new Server and potentially returns an error
func New(conf *Config) (*Server, error) {
	// Ensure we have at least one authentication method enabled
	if len(conf.AuthMethods) == 0 {
		if conf.Credentials != nil {
			conf.AuthMethods = []Authenticator{&UserPassAuthenticator{conf.Credentials}}
		} else {
			conf.AuthMethods = []Authenticator{&NoAuthAuthenticator{}}
		}
	}

	// Ensure we have a DNS resolver
	if conf.Resolver == nil {
		conf.Resolver = NoOpResolver{}
	}

	// Ensure we have a rule set
	if conf.Rules == nil {
		conf.Rules = PermitAll()
	}

	server := &Server{config: conf}

	if conf.Handler == nil {
		conf.Handler = &SinglePortUDPHandler{}
	}

	server.authMethods = make(map[uint8]Authenticator)

	for _, a := range conf.AuthMethods {
		server.authMethods[a.GetCode()] = a
	}

	return server, nil
}

// ListenAndServe is used to create a listener and serve on it
func (s *Server) ListenAndServe(ctx context.Context, network, addr string) error {
	l, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, l)
}

// Serve is used to start serve socks client connections from a listener. Serve blocks
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	g, ctx := scope.Group(ctx)
	defer g.AddCloser(l).WaitQuietly()

	if err := s.config.Handler.OnStartServe(g, l); err != nil {
		return err
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		g.GoNoError(func() { s.ServeTCPConn(ctx, conn) })
	}
}

// ServeTCPConn is used to serve a single TCP connection.
// If you use this function directly without Serve, then don't forget to manually call Handler.OnStartServe before it!
func (s *Server) ServeTCPConn(ctx context.Context, conn net.Conn) {
	defer scope.Closer(ctx, conn).Close()
	bufConn := bufio.NewReader(conn)

	// Read the version byte
	version := []byte{0}
	if _, err := bufConn.Read(version); err != nil {
		s.config.Handler.ErrLog().Printf("socks: Failed to get version byte: %v", err)
		return
	}

	// Ensure we are compatible
	if version[0] != socks5Version {
		err := fmt.Errorf("unsupported SOCKS version: %v", version)
		s.config.Handler.ErrLog().Printf("socks: %v", err)
		return
	}

	// Authenticate the connection
	authContext, err := s.authenticate(conn, bufConn)
	if err != nil {
		err = fmt.Errorf("failed to authenticate: %v", err)
		s.config.Handler.ErrLog().Printf("socks: %v", err)
		return
	}

	request, err := NewRequest(bufConn)
	if err != nil {
		if err == errUnrecognizedAddrType {
			if err := sendReply(conn, ReplyAddrTypeNotSupported, nil); err != nil {
				s.config.Handler.ErrLog().Printf("failed to send reply: %v", err)
				return
			}
		}
		s.config.Handler.ErrLog().Printf("failed to read destination address: %v", err)
		return
	}
	request.AuthContext = authContext
	if client, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		request.RemoteAddr = &AddrSpec{IP: client.IP, Port: client.Port}
	}

	// Process the client request
	if err := s.handleRequest(ctx, request, conn); err != nil {
		err = fmt.Errorf("failed to handle request: %v", err)
		s.config.Handler.ErrLog().Printf("socks: %v", err)
		return
	}
}
