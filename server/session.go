package chserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	chshare "github.com/jpillora/chisel/share"
	"github.com/jpillora/sizestr"
	"golang.org/x/crypto/ssh"
)

type session struct {
	id int
	*chshare.Logger
	server    *Server
	user      *chshare.User
	connCount int32
	connOpen  int32
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func newSession(server *Server, id int) *session {
	return &session{
		id:     id,
		Logger: server.Fork("session#%d", id),
		server: server,
	}

}

func (s *session) handle(w http.ResponseWriter, req *http.Request) error {
	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return fmt.Errorf("Websocket upgrade failed: %s", err)
	}
	conn := chshare.NewWebSocketConn(wsConn)
	// perform SSH handshake on net.Conn
	s.Debugf("Handshaking...")
	sshConfig := *s.server.sshConfig
	sshConfig.PasswordCallback = s.authUser
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, &sshConfig)
	if err != nil {
		return fmt.Errorf("SSH handshake failed: %s", err)
	}
	//load user
	// var user *chshare.User
	// if len(s.Users) > 0 {
	// 	sid := string(sshConn.SessionID())
	// 	user = s.sessions[sid]
	// 	defer delete(s.sessions, sid)
	// }
	//verify configuration
	s.Debugf("Verifying configuration")
	//wait for request, with timeout
	var r *ssh.Request
	select {
	case r = <-reqs:
	case <-time.After(10 * time.Second):
		sshConn.Close()
		return fmt.Errorf("Timeout waiting for config verification")
	}
	reply := func(err error) error {
		r.Reply(false, []byte(err.Error()))
		return err
	}
	if r.Type != "config" {
		return reply(fmt.Errorf("expecting config request"))
	}
	c, err := chshare.DecodeConfig(r.Payload)
	if err != nil {
		return reply(fmt.Errorf("invalid config"))
	}
	if c.Version != chshare.BuildVersion {
		v := c.Version
		if v == "" {
			v = "<unknown>"
		}
		s.Infof("Client version (%s) differs from server version (%s)",
			v, chshare.BuildVersion)
	}
	//success!
	r.Reply(true, nil)
	//prepare connection logger
	s.Debugf("Open")
	go s.handleSSHRequests(reqs)
	go s.handleSSHChannels(chans)
	sshConn.Wait()
	s.Debugf("Close")
	return nil
}

//
func (s *session) authUser(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
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

func (s *session) handleSSHRequests(reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "ping":
			r.Reply(true, nil)
		default:
			s.Debugf("Unknown request: %s", r.Type)
		}
	}
}

func (s *session) handleSSHChannels(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		remote := string(ch.ExtraData())
		socks := remote == "socks"
		//dont accept socks when --socks5 isn't enabled
		if socks && s.server.socksServer == nil {
			s.Debugf("Denied socks request, please enable --socks5")
			ch.Reject(ssh.Prohibited, "SOCKS5 is not enabled on the server")
			continue
		}
		//accept rest
		stream, reqs, err := ch.Accept()
		if err != nil {
			s.Debugf("Failed to accept stream: %s", err)
			continue
		}
		go ssh.DiscardRequests(reqs)
		//handle stream type
		connID := atomic.AddInt32(&s.connCount, 1)
		if socks {
			go s.handleSocksStream(s.Fork("socks#%05d", connID), stream)
		} else {
			go s.handleTCPStream(s.Fork(" tcp#%05d", connID), stream, remote)
		}
	}
}

//if user is provided, ensure they have
//access to the desired remotes
// if user != nil {
// 	for _, r := range c.Remotes {
// 		if r.Socks {
// 			continue //socks user access performed elsewhere
// 		}
// 		addr := r.RemoteHost + ":" + r.RemotePort
// 		if !user.HasAccess(addr) {
// 			failed(s.Errorf("access to '%s' denied", addr))
// 			return
// 		}
// 	}
// }

type chiselCtx string

func (s *session) handleSocksStream(l *chshare.Logger, src io.ReadWriteCloser) {
	conn := chshare.NewRWCConn(src)
	atomic.AddInt32(&s.connOpen, 1)
	l.Debugf("%s Openning", s.connStatus())
	ctx := context.WithValue(context.Background(), chiselCtx("user"), nil)

	err := s.socksServer.ServeConnContext(conn, ctx)
	atomic.AddInt32(&s.connOpen, -1)
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		l.Debugf("%s Closed (error: %s)", s.connStatus(), err)
	} else {
		l.Debugf("%s Closed", s.connStatus())
	}
}

func (s *session) handleTCPStream(l *chshare.Logger, src io.ReadWriteCloser, remote string) {
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
