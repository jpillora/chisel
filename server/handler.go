package chserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jpillora/sizestr"
	"golang.org/x/crypto/ssh"

	"github.com/jpillora/chisel/share"
)

// healthHandler is responsible for a generic /heath check
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK\n"))
}

// versionHandler endpoint provides the service version
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(chshare.BuildVersion))
}

// handleClientHandler is the main http sebsocket handler for the chisel server
func (s *Server) handleClientHandler(w http.ResponseWriter, r *http.Request) {
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	protocol := r.Header.Get("Sec-WebSocket-Protocol")

	//websockets upgrade AND has chisel prefix
	if upgrade == "websocket" && protocol == chshare.ProtocolVersion {
		s.handleWebsocket(w, r)
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

// handleWebsocket is responsible for hanlding the websocket connection
func (s *Server) handleWebsocket(w http.ResponseWriter, req *http.Request) {
	id := atomic.AddInt32(&s.sessCount, 1)
	clog := s.Fork("session#%d", id)

	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		clog.Debugf("Failed to upgrade (%s)", err)
		return
	}
	conn := chshare.NewWebSocketConn(wsConn)
	// perform SSH handshake on net.Conn
	clog.Debugf("Handshaking...")
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.Debugf("Failed to handshake (%s)", err)
		return
	}

	// pull the users from the session map
	var user *chshare.User
	if len(s.Users) > 0 {
		sid := string(sshConn.SessionID())
		user = s.sessions[sid]
		defer delete(s.sessions, sid)
	}

	//verify configuration
	clog.Debugf("Verifying configuration")

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
	if c.Version != chshare.BuildVersion {
		v := c.Version
		if v == "" {
			v = "<unknown>"
		}
		clog.Infof("Client version (%s) differs from server version (%s)",
			v, chshare.BuildVersion)
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
	clog.Debugf("Open")
	go s.handleSSHRequests(clog, reqs)
	go s.handleSSHChannels(clog, chans)
	sshConn.Wait()
	clog.Debugf("Close")
}

func (s *Server) handleSSHRequests(clientLog *chshare.Logger, reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "ping":
			r.Reply(true, nil)
		default:
			clientLog.Debugf("Unknown request: %s", r.Type)
		}
	}
}

func (s *Server) handleSSHChannels(clientLog *chshare.Logger, chans <-chan ssh.NewChannel) {
	for ch := range chans {
		remote := string(ch.ExtraData())
		socks := remote == "socks"
		//dont accept socks when --socks5 isn't enabled
		if socks && s.socksServer == nil {
			clientLog.Debugf("Denied socks request, please enable --socks5")
			ch.Reject(ssh.Prohibited, "SOCKS5 is not enabled on the server")
			continue
		}
		//accept rest
		stream, reqs, err := ch.Accept()
		if err != nil {
			clientLog.Debugf("Failed to accept stream: %s", err)
			continue
		}
		go ssh.DiscardRequests(reqs)
		//handle stream type
		connID := atomic.AddInt32(&s.connCount, 1)
		if socks {
			go s.handleSocksStream(clientLog.Fork("socks#%05d", connID), stream)
		} else {
			go s.handleTCPStream(clientLog.Fork(" tcp#%05d", connID), stream, remote)
		}
	}
}

func (s *Server) handleSocksStream(l *chshare.Logger, src io.ReadWriteCloser) {
	conn := chshare.NewRWCConn(src)
	// conn.SetDeadline(time.Now().Add(30 * time.Second))
	atomic.AddInt32(&s.connOpen, 1)
	l.Debugf("%s Openning", s.connStatus())
	err := s.socksServer.ServeConn(conn)
	atomic.AddInt32(&s.connOpen, -1)
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		l.Debugf("%s Closed (error: %s)", s.connStatus(), err)
	} else {
		l.Debugf("%s Closed", s.connStatus())
	}
}

func (s *Server) handleTCPStream(l *chshare.Logger, src io.ReadWriteCloser, remote string) {
	dst, err := net.Dial("tcp", remote)
	if err != nil {
		l.Debugf("Remote failed (%s)", err)
		src.Close()
		return
	}
	atomic.AddInt32(&s.connOpen, 1)
	l.Debugf("%s Open", s.connStatus())
	sent, received := chshare.Pipe(src, dst)
	atomic.AddInt32(&s.connOpen, -1)
	l.Debugf("%s Close (sent %s received %s)", s.connStatus(), sizestr.ToString(sent), sizestr.ToString(received))
}

func (s *Server) connStatus() string {
	return fmt.Sprintf("[%d/%d]", atomic.LoadInt32(&s.connOpen), atomic.LoadInt32(&s.connCount))
}
