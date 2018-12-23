package chshare

import (
	"context"
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

func (p *TCPProxy) Start(ctx context.Context) error {
	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		return fmt.Errorf("%s: %s", p.Logger.Prefix(), err)
	}
	go p.listen(ctx, l)
	return nil
}

func (p *TCPProxy) listen(ctx context.Context, l net.Listener) {
	p.Infof("Listening")
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.Debugf("Context canceled; closing listener")
			l.Close()
		case <-done:
		}
	}()

	for {
		src, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				p.Infof("Stop listening; remote disconnected")
			default:
				p.Infof("Accept error: %s", err)
			}
			close(done)
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
