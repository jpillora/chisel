package link

import (
	"context"
	"github.com/valkyrie-io/connector-tunnel/shared"
	"io"
	"net"
	"sync"

	"github.com/jpillora/sizestr"
	"github.com/valkyrie-io/connector-tunnel/shared/settings"
	"golang.org/x/crypto/ssh"
)

// sshConnection exposes a subset of Link to subtypes
type sshConnection interface {
	get(ctx context.Context) ssh.Conn
}

// Gate is the inbound portion of a Link
type Gate struct {
	*shared.Logger
	sshConn sshConnection
	id      int
	tcp     *net.TCPListener
	remote  *settings.Remote
	count   int
	mu      sync.Mutex
	dialer  net.Dialer
}

// InitGate creates a Gate
func InitGate(logger *shared.Logger, sshTun sshConnection, index int, remote *settings.Remote) (*Gate, error) {
	id := index + 1
	p := &Gate{
		sshConn: sshTun,
		Logger:  logger.Fork("proxy#%s", remote.String()),
		id:      id,
		remote:  remote,
	}
	return p, p.listen()
}

func (p *Gate) listen() error {
	if p.remote.LocalProto == "tcp" {
		addr, err := net.ResolveTCPAddr("tcp", p.remote.LocalHost+":"+p.remote.LocalPort)
		if err != nil {
			return p.Errorf("resolve: %s", err)
		}
		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return p.Errorf("tcp: %s", err)
		}
		p.Infof("Listening")
		p.tcp = l
	} else {
		return p.Errorf("unknown local proto")
	}
	return nil
}

// Run enables the proxy and blocks while its active,
// close the proxy by cancelling the context.
func (p *Gate) Run(ctx context.Context) error {
	done := make(chan struct{})
	//implements missing net.ListenContext
	go func() {
		select {
		case <-ctx.Done():
			p.tcp.Close()
		case <-done:
		}
	}()
	for {
		err2, done2 := p.acceptConnections(ctx, done)
		if done2 {
			return err2
		}
	}
}

func (p *Gate) acceptConnections(ctx context.Context, done chan struct{}) (error, bool) {
	src, err := p.tcp.Accept()
	if err != nil {
		select {
		case <-ctx.Done():
			//listener closed
			err = nil
		default:
			p.Infof("Accept error: %s", err)
		}
		close(done)
		return err, true
	}
	go p.pipeRemote(ctx, src)
	return nil, false
}

func (p *Gate) pipeRemote(ctx context.Context, src io.ReadWriteCloser) {
	defer src.Close()

	p.mu.Lock()
	p.count++
	cid := p.count
	p.mu.Unlock()

	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	sshConn := p.sshConn.get(ctx)
	if sshConn == nil {
		l.Debugf("No remote connection")
		return
	}
	//ssh request for tcp connection for this proxy's remote
	dst, reqs, err := sshConn.OpenChannel("valkyrie", []byte(p.remote.Remote()))
	if err != nil {
		l.Infof("Stream error: %s", err)
		return
	}
	go ssh.DiscardRequests(reqs)
	//then pipe
	s, r := Pipe(src, dst)
	l.Debugf("Close (sent %s received %s)", sizestr.ToString(s), sizestr.ToString(r))
}
