package chclient

import (
	"fmt"
	"io"
	"net"

	"github.com/jpillora/chisel/share"
	"golang.org/x/crypto/ssh"
)

type GetSSHConn func() ssh.Conn

type tcpProxy struct {
	*chshare.Logger
	ssh    GetSSHConn
	id     int
	count  int
	remote *chshare.Remote
}

func newTCPProxy(logger *chshare.Logger, ssh GetSSHConn, index int, remote *chshare.Remote) *tcpProxy {
	id := index + 1
	return &tcpProxy{
		Logger: logger.Fork("tunnel#%d %s", id, remote),
		ssh:    ssh,
		id:     id,
		remote: remote,
	}
}

func (p *tcpProxy) start() error {
	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		return fmt.Errorf("%s: %s", p.Logger.Prefix(), err)
	}
	go p.listen(l)
	return nil
}

func (p *tcpProxy) listen(l net.Listener) {
	p.Infof("Listening")
	for {
		src, err := l.Accept()
		if err != nil {
			p.Infof("Accept error: %s", err)
			return
		}
		go p.accept(src)
	}
}

func (p *tcpProxy) accept(src io.ReadWriteCloser) {
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	if p.ssh() == nil {
		l.Debugf("No remote connection")
		src.Close()
		return
	}
	dst, err := chshare.OpenStream(p.ssh(), p.remote.Remote())
	if err != nil {
		l.Infof("Stream error: %s", err)
		src.Close()
		return
	}
	//then pipe
	s, r := chshare.Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}
