package chclient

import (
	"io"
	"net"

	"github.com/cicavey/chisel/share"
)

type tcpProxy struct {
	*chshare.Logger
	client *Client
	id     int
	count  int
	remote *chshare.Remote
}

func newTCPProxy(c *Client, index int, remote *chshare.Remote) *tcpProxy {
	id := index + 1
	return &tcpProxy{
		Logger: c.Logger.Fork("tunnel#%d %s", id, remote),
		client: c,
		id:     id,
		remote: remote,
	}
}

func (p *tcpProxy) start() {
	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		p.Infof("%s", err)
		return
	}
	p.Infof("Enabled")
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
	if p.client.sshConn == nil {
		l.Debugf("No server connection")
		src.Close()
		return
	}
	dst, err := chshare.OpenStream(p.client.sshConn, p.remote.Remote())
	if err != nil {
		l.Infof("Stream error: %s", err)
		src.Close()
		return
	}
	//then pipe
	s, r := chshare.Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}
