package chserver

import (
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

// handleClientHandler is the main http websocket handler for the chisel server
func (s *Server) handleClientHandler(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	if upgrade == "websocket" && protocol == chshare.ProtocolVersion {
		s.handleWebsocket(w, r)
		return
	}
	//proxy target was provided
	if s.reverseProxy != nil {
		s.reverseProxy.ServeHTTP(w, r)
		return
	}
	//no proxy defined, provide access to health/version checks
	switch r.URL.String() {
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
	if s.users.Len() > 0 {
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
		connID := s.connStats.New()
		if socks {
			go s.handleSocksStream(clientLog.Fork("socks#%05d", connID), stream)
		} else {
			go HandleTCPStream(clientLog.Fork(" tcp#%05d", connID), &s.connStats, stream, remote)
		}
	}
}

func (s *Server) handleSocksStream(l *chshare.Logger, src io.ReadWriteCloser) {
	conn := chshare.NewRWCConn(src)
	// conn.SetDeadline(time.Now().Add(30 * time.Second))
	s.connStats.Open()
	l.Debugf("%s Opening", s.connStats.Status())
	err := s.socksServer.ServeConn(conn)
	s.connStats.Close()
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		l.Debugf("%s Closed (error: %s)", s.connStats.Status(), err)
	} else {
		l.Debugf("%s Closed", s.connStats.Status())
	}
}

func HandleTCPStream(l *chshare.Logger, connStats *shshare.ConnStats, src io.ReadWriteCloser, remote string) {
	dst, err := net.Dial("tcp", remote)
	if err != nil {
		l.Debugf("Remote failed (%s)", err)
		src.Close()
		return
	}
	connStats.Open()
	l.Debugf("%s Open", connStats.Status())
	sent, received := chshare.Pipe(src, dst)
	connStats.Close()
	l.Debugf("%s Close (sent %s received %s)", connStats.Status(), sizestr.ToString(sent), sizestr.ToString(received))
}
