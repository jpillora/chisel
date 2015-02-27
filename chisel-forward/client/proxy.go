package client

import (
	"encoding/binary"
	"net"

	"github.com/jpillora/chisel"
)

type Proxy struct {
	id         int
	count      int
	remote     *chisel.Remote
	openStream func() (net.Conn, error)
}

func NewProxy(id int, remote *chisel.Remote, openStream func() (net.Conn, error)) *Proxy {
	return &Proxy{
		id:         id,
		remote:     remote,
		openStream: openStream,
	}
}

func (p *Proxy) start() {

	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		chisel.Printf("Proxy [%d] Failed to start: %s", p.id, err)
		return
	}

	chisel.Printf("Proxy [%d] Enabled (%s)", p.id, p.remote)
	for {
		src, err := l.Accept()
		if err != nil {
			chisel.Printf("%s", err)
			return
		}
		go p.accept(src)
	}
}

func (p *Proxy) accept(src net.Conn) {
	p.count++
	c := p.count

	chisel.Printf("Proxy [%d] Connection [%d] Open", p.id, c)

	dst, err := p.openStream()
	if err != nil {
		chisel.Printf("%s", err)
		return
	}

	//write endpoint id
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(p.id))
	dst.Write(b)

	//then pipe
	s, r := chisel.Pipe(src, dst)

	chisel.Printf("Proxy [%d] Connection [%d] Closed (sent %d received %d)", p.id, c, s, r)
}
