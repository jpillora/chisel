package server

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/valkyrie-io/connector-tunnel/common/logging"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/valkyrie-io/connector-tunnel/common"
	"github.com/valkyrie-io/connector-tunnel/common/netext"
	"github.com/valkyrie-io/connector-tunnel/common/settings"
	"github.com/valkyrie-io/connector-tunnel/common/sshconnection"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// handleClientHandler is the main http websocket handler for the chisel server
func (s *Server) handleClientHandler(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	if upgraded := s.upgradeToWS(w, r); upgraded {
		return
	}
	if served := s.serveHealthCheckAPIs(w, r); served {
		return
	}
	w.WriteHeader(404)
	w.Write([]byte("Not found"))
}

func (s *Server) serveHealthCheckAPIs(w http.ResponseWriter, r *http.Request) (served bool) {
	switch r.URL.Path {
	case "/health":
		w.Write([]byte("OK\n"))
		return true
	case "/version":
		w.Write([]byte(common.BuildVersion))
		return true
	}
	return false
}

func (s *Server) upgradeToWS(w http.ResponseWriter, r *http.Request) (upgraded bool) {
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	if upgrade == "websocket" {
		if protocol == common.ProtocolVersion {
			s.handleWebsocket(w, r)
			return true
		}
		//print into server logs and silently fall-through
		s.Infof("ignored client connection using protocol '%s', expected '%s'",
			protocol, common.ProtocolVersion)
	}
	return false
}

// handleWebsocket is responsible for handling the websocket connection
func (s *Server) handleWebsocket(w http.ResponseWriter, req *http.Request) {
	l, wsConn, err := s.upgrade2WS(w, req)
	if err != nil {
		return
	}
	sshConn, chans, reqs, err := s.handshakeConn(wsConn, l, req, err)
	if err != nil {
		return
	}
	user := s.pullUserMap(sshConn)
	r, failedConn, c, configValid := s.verifyConfig(l, reqs, sshConn, err)
	if !configValid {
		return
	}
	s.verifyVersionMatch(c, l)
	if valid := s.validateRemotes(c, user, failedConn, l); !valid {
		return
	}
	r.Reply(true, nil)
	//sshconnection per ssh connection
	connection := sshconnection.New(sshconnection.Config{
		Logger:    l,
		Inbound:   s.config.Reverse,
		Outbound:  true, //server always accepts outbound
		KeepAlive: s.config.KeepAlive,
	})
	//bind
	eg, ctx := errgroup.WithContext(req.Context())
	eg.Go(func() error {
		//connected, handover ssh connection for sshconnection to use, and block
		return connection.Bind(ctx, sshConn, reqs, chans)
	})
	eg.Go(func() error {
		//connected, setup reversed-remotes?
		return s.setupReverseRemotesIfNeeded(c, connection, ctx)
	})
	err = eg.Wait()
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		l.Debugf("Closed connection (%s)", err)
	} else {
		l.Debugf("Closed connection")
	}
}

func (s *Server) setupReverseRemotesIfNeeded(c *settings.Config, connection *sshconnection.SSHConnection, ctx context.Context) error {
	serverInbound := c.Remotes.Reversed(true)
	if len(serverInbound) == 0 {
		return nil
	}
	//block
	return connection.ConnectRemotes(ctx, serverInbound)
}

func (s *Server) validateRemotes(c *settings.Config, user *settings.User, failedConn func(err error), l *logging.Logger) bool {
	for _, r := range c.Remotes {
		//if user is provided, ensure they have
		//access to the desired remotes
		if allowed := s.verifyAccessAllow(user, r, failedConn); !allowed {
			return false
		}
		//confirm reverse tunnels are allowed
		if allowed := s.verifyReverseConnectionAllow(r, l, failedConn); !allowed {
			return false
		}
		//confirm reverse sshconnection is available
		if r.Reverse && !r.CanListen() {
			failedConn(s.Errorf("Server cannot listen on %s", r.String()))
			return false
		}
	}
	return true
}

func (s *Server) verifyReverseConnectionAllow(r *settings.Remote, l *logging.Logger, failedConn func(err error)) bool {
	if r.Reverse && !s.config.Reverse {
		l.Debugf("Denied reverse port forwarding request, please enable --reverse")
		failedConn(s.Errorf("Reverse port forwaring not enabled on server"))
		return false
	}
	return true
}

func (s *Server) verifyAccessAllow(user *settings.User, r *settings.Remote, failedConn func(err error)) bool {
	if user != nil {
		addr := r.UserAddr()
		if !user.HasAccess(addr) {
			failedConn(s.Errorf("access to '%s' denied", addr))
			return false
		}
	}
	return true
}

func (s *Server) verifyVersionMatch(c *settings.Config, l *logging.Logger) {
	cv := strings.TrimPrefix(c.Version, "v")
	if cv == "" {
		cv = "<unknown>"
	}
	sv := strings.TrimPrefix(common.BuildVersion, "v")
	if cv != sv {
		l.Infof("Client version (%s) differs from server version (%s)", cv, sv)
	}
}

func (s *Server) verifyConfig(l *logging.Logger, reqs <-chan *ssh.Request, sshConn *ssh.ServerConn, err error) (*ssh.Request, func(err error), *settings.Config, bool) {
	l.Debugf("Verifying configuration")
	// wait for request, with timeout
	var r *ssh.Request
	select {
	case r = <-reqs:
	case <-time.After(settings.EnvDuration("CONFIG_TIMEOUT", 10*time.Second)):
		l.Debugf("Timeout waiting for configuration")
		sshConn.Close()
		return nil, nil, nil, false
	}
	failed := func(err error) {
		l.Debugf("Failed: %s", err)
		r.Reply(false, []byte(err.Error()))
	}
	if r.Type != "config" {
		failed(s.Errorf("expecting config request"))
		return nil, nil, nil, false
	}
	c, err := settings.DecodeConfig(r.Payload)
	if err != nil {
		failed(s.Errorf("invalid config"))
		return nil, nil, nil, false
	}
	return r, failed, c, true
}

func (s *Server) pullUserMap(sshConn *ssh.ServerConn) *settings.User {
	var user *settings.User
	if s.users.Len() > 0 {
		sid := string(sshConn.SessionID())
		u, ok := s.sessions.Get(sid)
		if !ok {
			panic("bug in ssh auth handler")
		}
		user = u
		s.sessions.Del(sid)
	}
	return user
}

func (s *Server) handshakeConn(wsConn *websocket.Conn, l *logging.Logger, req *http.Request, err error) (*ssh.ServerConn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	conn := netext.NewWebSocketConn(wsConn)
	// perform SSH handshake on netext.Conn
	l.Debugf("Handshaking with %s...", req.RemoteAddr)
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.Debugf("Failed to handshake (%s)", err)
		return nil, nil, nil, err
	}
	return sshConn, chans, reqs, nil
}

func (s *Server) upgrade2WS(w http.ResponseWriter, req *http.Request) (*logging.Logger, *websocket.Conn, error) {
	id := atomic.AddInt32(&s.sessCount, 1)
	l := s.Fork("session#%d", id)
	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		l.Debugf("Failed to upgrade (%s)", err)
		return nil, nil, err
	}
	return l, wsConn, nil
}
