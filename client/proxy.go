package chclient

import (
	"encoding/binary"
	"net"

	"github.com/jpillora/chisel/share"
)

type Proxy struct {
	*chshare.Logger
	id         int
	count      int
	remote     *chshare.Remote
	openStream func() (net.Conn, error)
}

func NewProxy(c *Client, id int, remote *chshare.Remote, openStream func() (net.Conn, error)) *Proxy {
	return &Proxy{
		Logger:     c.Logger.Fork("%s:%s#%d", remote.RemoteHost, remote.RemotePort, id+1),
		id:         id,
		remote:     remote,
		openStream: openStream,
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

func (p *Proxy) accept(src net.Conn) {
	p.count++
	cid := p.count
	clog := p.Fork("conn#%d", cid)

	clog.Debugf("Open")

	dst, err := p.openStream()
	if err != nil {
		clog.Debugf("Stream error: %s", err)
		src.Close()
		return
	}

	//write endpoint id
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(p.id))
	dst.Write(b)

	//then pipe
	s, r := chshare.Pipe(src, dst)

	clog.Debugf("Close (sent %d received %d)", s, r)
}
