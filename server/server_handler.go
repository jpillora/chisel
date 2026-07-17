package chserver

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	chshare "github.com/jpillora/chisel/share"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/settings"
	"github.com/jpillora/chisel/share/tunnel"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// handleClientHandler is the main http websocket handler for the chisel server
func (s *Server) handleClientHandler(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	if upgrade == "websocket" {
		if protocol == chshare.ProtocolVersion {
			s.handleWebsocket(w, r)
			return
		}
		//print into server logs and silently fall-through
		s.Infof("ignored client connection using protocol '%s', expected '%s'",
			protocol, chshare.ProtocolVersion)
	}
	//proxy target was provided
	if s.reverseProxy != nil {
		s.reverseProxy.ServeHTTP(w, r)
		return
	}
	//no proxy defined, provide access to health/version checks
	switch r.URL.Path {
	case "/health":
		w.Write([]byte("OK\n"))
		return
	case "/version":
		w.Write([]byte(chshare.BuildVersion))
		return
	}
	//missing :O
	w.WriteHeader(404)
	w.Write([]byte("Not found"))
}

// handleWebsocket is responsible for handling the websocket connection
func (s *Server) handleWebsocket(w http.ResponseWriter, req *http.Request) {
	id := atomic.AddInt32(&s.sessCount, 1)
	l := s.Fork("session#%d", id)
	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		l.Debugf("Failed to upgrade (%s)", err)
		return
	}
	conn := cnet.NewWebSocketConn(wsConn)
	// perform SSH handshake on net.Conn
	l.Debugf("Handshaking with %s...", req.RemoteAddr)
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.Debugf("Failed to handshake (%s)", err)
		return
	}
	// resolve the user from the authenticated username set by
	// authUser (nil permissions means auth is disabled: allow-all)
	var user *settings.User
	if sshConn.Permissions != nil {
		n := sshConn.Permissions.Extensions["user"]
		u, found := s.users.Get(n)
		if !found {
			//user was removed by an authfile reload mid-handshake
			l.Infof("User %s no longer exists", n)
			sshConn.Close()
			return
		}
		user = u
	}
	// chisel server handshake (reverse of client handshake)
	// verify configuration
	l.Debugf("Verifying configuration")
	// wait for request, with timeout
	var r *ssh.Request
	select {
	case r = <-reqs:
	case <-time.After(settings.EnvDuration("CONFIG_TIMEOUT", 10*time.Second)):
		l.Debugf("Timeout waiting for configuration")
		sshConn.Close()
		return
	}
	if r == nil {
		//connection closed before the config request arrived; the
		//closed request channel yields nil
		l.Debugf("Connection closed before configuration")
		sshConn.Close()
		return
	}
	failed := func(err error) {
		l.Debugf("Failed: %s", err)
		r.Reply(false, []byte(err.Error()))
	}
	if r.Type != "config" {
		failed(s.Errorf("expecting config request"))
		return
	}
	c, err := settings.DecodeConfig(r.Payload)
	if err != nil {
		failed(s.Errorf("invalid config"))
		return
	}
	//print if client and server versions dont match,
	//skipping dev/unknown builds where the mismatch is meaningless
	cv := strings.TrimPrefix(c.Version, "v")
	sv := strings.TrimPrefix(chshare.BuildVersion, "v")
	devVersion := func(v string) bool {
		return v == "" || strings.HasPrefix(v, "0.0.0")
	}
	if cv != sv && !devVersion(cv) && !devVersion(sv) {
		l.Infof("Client version (%s) differs from server version (%s)", cv, sv)
	}
	//validate remotes
	for _, r := range c.Remotes {
		//if user is provided, ensure they have
		//access to the desired remotes
		if user != nil {
			addr := r.UserAddr()
			if !user.HasAccess(addr) {
				failed(s.Errorf("access to '%s' denied", addr))
				return
			}
		}
		//confirm reverse tunnels are allowed
		if r.Reverse && !s.config.Reverse {
			l.Debugf("Denied reverse port forwarding request, please enable --reverse")
			failed(s.Errorf("Reverse port forwarding not enabled on server"))
			return
		}
		//confirm reverse tunnel is available
		if r.Reverse && !r.CanListen() {
			failed(s.Errorf("Server cannot listen on %s", r.String()))
			return
		}
	}
	//successfully validated config!
	r.Reply(true, nil)
	//log session opens at info level so operators can see connected
	//clients, their IPs and declared remotes without debug mode
	username := "-"
	if user != nil {
		username = user.Name
	}
	opened := time.Now()
	l.Infof("Open (user=%s addr=%s remotes=%s)",
		username, req.RemoteAddr, strings.Join(c.Remotes.Encode(), ","))
	//tunnel per ssh connection
	tunnelConfig := tunnel.Config{
		Logger:    l,
		Inbound:   s.config.Reverse,
		Outbound:  true, //server always accepts outbound
		Socks:     s.config.Socks5,
		KeepAlive: s.config.KeepAlive,
	}
	//enforce ACL on every channel, not just the initial config.
	//the user is re-resolved from the live index per channel, so
	//authfile reloads apply to connected clients' new tunnels
	if user != nil {
		name := user.Name
		tunnelConfig.ACL = func(addr string) bool {
			u, found := s.users.Get(name)
			return found && u.HasAccess(addr)
		}
	}
	tunnel := tunnel.New(tunnelConfig)
	//bind
	eg, ctx := errgroup.WithContext(req.Context())
	eg.Go(func() error {
		//connected, handover ssh connection for tunnel to use, and block
		return tunnel.BindSSH(ctx, sshConn, reqs, chans)
	})
	eg.Go(func() error {
		//connected, setup reversed-remotes?
		serverInbound := c.Remotes.Reversed(true)
		if len(serverInbound) == 0 {
			return nil
		}
		//block
		return tunnel.BindRemotes(ctx, serverInbound)
	})
	err = eg.Wait()
	errmsg := ""
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		errmsg = fmt.Sprintf(" (error %s)", err)
	}
	l.Infof("Close (user=%s addr=%s duration=%s)%s",
		username, req.RemoteAddr, time.Since(opened), errmsg)
}
