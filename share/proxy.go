package chshare

import (
	"fmt"
	"io"
	"net"

	"golang.org/x/crypto/ssh"
)

type GetSSHConn func() ssh.Conn

type TCPProxy struct {
	*Logger
	ssh    GetSSHConn
	id     int
	count  int
	remote *Remote
}

func NewTCPProxy(logger *Logger, ssh GetSSHConn, index int, remote *Remote) *TCPProxy {
	id := index + 1
	return &TCPProxy{
		Logger: logger.Fork("tunnel#%d %s", id, remote),
		ssh:    ssh,
		id:     id,
		remote: remote,
	}
}

func (p *TCPProxy) Start() error {
	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		return fmt.Errorf("%s: %s", p.Logger.Prefix(), err)
	}
	go p.listen(l)
	return nil
}

func (p *TCPProxy) listen(l net.Listener) {
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

func (p *TCPProxy) accept(src io.ReadWriteCloser) {
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	if p.ssh() == nil {
		l.Debugf("No remote connection")
		src.Close()
		return
	}
	dst, err := OpenStream(p.ssh(), p.remote.Remote())
	if err != nil {
		l.Infof("Stream error: %s", err)
		src.Close()
		return
	}
	//then pipe
	s, r := Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}
