package tunnel

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/jpillora/sizestr"
	"github.com/valkyrie-io/connector-tunnel/common/logging"
	"github.com/valkyrie-io/connector-tunnel/common/settings"
	"golang.org/x/crypto/ssh"
)

// Proxy is the inbound portion of a SSHTunnel
type Proxy struct {
	*logging.Logger
	tunnel *SSHTunnel
	id     int
	count  int
	remote *settings.Remote
	dialer net.Dialer
	tcp    *net.TCPListener
	mu     sync.Mutex
}

// newProxy creates a Proxy
func newProxy(logger *logging.Logger, tunnel *SSHTunnel, index int, remote *settings.Remote) (*Proxy, error) {
	id := index + 1
	p := &Proxy{
		Logger: logger.Fork("proxy#%s", remote.String()),
		tunnel: tunnel,
		id:     id,
		remote: remote,
	}
	return p, p.listen()
}

func (p *Proxy) listen() error {
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

func (p *Proxy) runTCP(ctx context.Context) error {
	done := make(chan struct{})
	go p.handleContextChange(ctx, done)
	for {
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
			return err
		}
		go p.pipeRemote(ctx, src)
	}
}

func (p *Proxy) handleContextChange(ctx context.Context, done chan struct{}) {
	select {
	case <-ctx.Done():
		p.tcp.Close()
	case <-done:
	}
}

func (p *Proxy) pipeRemote(ctx context.Context, src io.ReadWriteCloser) {
	defer src.Close()
	cid := p.atomicIncreaseConnCounter()
	l := p.Fork("conn#%d", cid)

	dst, reqs, done := p.connectAndOpenSSHChannel(ctx, l)
	if done {
		return
	}
	p.closeSSHChannel(reqs, src, dst, l)
}

func (p *Proxy) atomicIncreaseConnCounter() int {
	p.mu.Lock()
	p.count++
	cid := p.count
	p.mu.Unlock()
	return cid
}

func (p *Proxy) connectAndOpenSSHChannel(ctx context.Context, l *logging.Logger) (ssh.Channel, <-chan *ssh.Request, bool) {
	sshConn, done := p.connectSSH(ctx, l)
	if done {
		return nil, nil, true
	}
	//ssh request for tcp connection for this proxy's remote
	dst, reqs, err := sshConn.OpenChannel("valkyrie", []byte(p.remote.Remote()))
	if err != nil {
		l.Infof("Stream error: %s", err)
		return nil, nil, true
	}
	return dst, reqs, false
}

func (p *Proxy) connectSSH(ctx context.Context, l *logging.Logger) (ssh.Conn, bool) {
	l.Debugf("Open")
	sshConn := p.tunnel.getSSH(ctx)
	if sshConn == nil {
		l.Debugf("No remote connection")
		return nil, true
	}
	return sshConn, false
}

func (p *Proxy) closeSSHChannel(reqs <-chan *ssh.Request, src io.ReadWriteCloser, dst ssh.Channel, l *logging.Logger) {
	go ssh.DiscardRequests(reqs)
	s, r := Pipe(src, dst)
	l.Debugf("Close (sent %s received %s)", sizestr.ToString(s), sizestr.ToString(r))
}
