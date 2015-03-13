package chiselclient

import (
	"io"
	"net"

	"github.com/jpillora/chisel"
)

type Proxy struct {
	*chisel.Logger
	client *Client
	id     int
	count  int

	remote *chisel.Remote
}

func NewProxy(c *Client, id int, remote *chisel.Remote) *Proxy {
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
	dst, err := chisel.OpenStream(p.client.sshConn, remoteAddr)
	if err != nil {
		l.Debugf("Stream error: %s", err)
		src.Close()
		return
	}

	//then pipe
	s, r := chisel.Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}
