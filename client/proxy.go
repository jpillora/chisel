package chclient

import (
	"io"
	"net"

	"github.com/cicavey/chisel/share"
)

type Proxy struct {
	*chshare.Logger
	client *Client
	id     int
	count  int
	remote *chshare.Remote
}

func NewProxy(c *Client, id int, remote *chshare.Remote) *Proxy {
	return &Proxy{
		Logger: c.Logger.Fork("%s:%s#%d", remote.RemoteHost, remote.RemotePort, id+1),
		client: c,
		id:     id,
		remote: remote,
	}
}

func (p *Proxy) start() {

	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		p.Infof("%s", err)
		return
	}

	p.Debugf("Enabled")
	for {
		src, err := l.Accept()
		if err != nil {
			p.Infof("Accept error: %s", err)
			return
		}
		go p.accept(src)
	}
}

func (p *Proxy) accept(src io.ReadWriteCloser) {
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)

	l.Debugf("Open")

	if p.client.sshConn == nil {
		l.Debugf("No server connection")
		src.Close()
		return
	}

	remoteAddr := p.remote.RemoteHost + ":" + p.remote.RemotePort
	dst, err := chshare.OpenStream(p.client.sshConn, remoteAddr)
	if err != nil {
		l.Infof("Stream error: %s", err)
		src.Close()
		return
	}

	//then pipe
	s, r := chshare.Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}
