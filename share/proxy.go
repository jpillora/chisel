package chshare

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/jpillora/sizestr"
	"golang.org/x/crypto/ssh"
)

type GetSSHConn func() ssh.Conn

type L4Proxy struct {
	*Logger
	ssh    GetSSHConn
	id     int
	count  int
	remote *Remote
}

func NewL4Proxy(logger *Logger, ssh GetSSHConn, index int, remote *Remote) *L4Proxy {
	id := index + 1
	return &L4Proxy{
		Logger: logger.Fork("proxy#%d:%s", id, remote),
		ssh:    ssh,
		id:     id,
		remote: remote,
	}
}

func (p *L4Proxy) Start(ctx context.Context) error {
	if p.remote.Stdio {
		go p.listenStdio(ctx)
		return nil
	}
	if p.remote.LocalProto == "udp" {
		return p.listenUDP(ctx)
	}
	return p.listenTCP(ctx)
}

func (p *L4Proxy) listenStdio(ctx context.Context) {
	for {
		p.accept(Stdio)
		select {
		case <-ctx.Done():
			return
		default:
			// the connection is not ready yet, keep waiting
		}
	}
}

func (p *L4Proxy) listenUDP(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	panic("TODO")
	return nil
}

func (p *L4Proxy) listenUDPInner(ctx context.Context, conn *net.UDPConn) {
	p.Infof("Listening")
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
			p.Infof("Closed")
		}
	}()
	//TODO
	//keep single ssh channel open,
	//pass all messages through
	const maxMTU = 9012
	buff := make([]byte, maxMTU)
	for {
		n, err := conn.Read(buff)
		if err == io.EOF {
			break
		}
		if err != nil {
			p.Infof("Connection error: %s", err)
			break
		}
		b := buff[:n]
		p.Infof("Message: %s", string(b))
	}

}

func (p *L4Proxy) listenTCP(ctx context.Context) error {
	l, err := net.Listen("tcp", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		return fmt.Errorf("%s: %s", p.Logger.Prefix(), err)
	}
	go p.listenTCPInner(ctx, l)
	return nil

}

func (p *L4Proxy) listenTCPInner(ctx context.Context, l net.Listener) {
	p.Infof("Listening")
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.Close()
			p.Infof("Closed")
		case <-done:
		}
	}()
	for {
		src, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				//listener closed
			default:
				p.Infof("Accept error: %s", err)
			}
			close(done)
			return
		}
		go p.accept(src)
	}
}

func (p *L4Proxy) accept(src io.ReadWriteCloser) {
	defer src.Close()
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	sshConn := p.ssh()
	if sshConn == nil {
		l.Debugf("No remote connection")
		return
	}
	//ssh request for tcp connection for this proxy's remote
	dst, reqs, err := sshConn.OpenChannel("chisel", []byte(p.remote.Remote()))
	if err != nil {
		l.Infof("Stream error: %s", err)
		return
	}
	go ssh.DiscardRequests(reqs)
	//then pipe
	s, r := Pipe(src, dst)
	l.Debugf("Close (sent %s received %s)", sizestr.ToString(s), sizestr.ToString(r))
}

//TCPProxy makes this package backward compatible
type TCPProxy = L4Proxy

//NewTCPProxy makes this package backward compatible
var NewTCPProxy = NewL4Proxy
