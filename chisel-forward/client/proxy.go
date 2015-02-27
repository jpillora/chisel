package client

import (
	"encoding/binary"
	"log"
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
		log.Printf("Proxy %s failed: %s", p.remote, err)
		return
	}

	log.Printf("Proxy %s enabled", p.remote)
	for {
		src, err := l.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		go p.accept(src)
	}
}

func (p *Proxy) accept(src net.Conn) {
	p.count++
	c := p.count

	log.Printf("[#%d] accept conn %d", p.id, c)

	dst, err := p.openStream()
	if err != nil {
		log.Println(err)
		return
	}

	//write endpoint id
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(p.id))
	dst.Write(b)

	//then pipe
	chisel.Pipe(src, dst)

	log.Printf("[#%d] close conn %d", p.id, c)
}
