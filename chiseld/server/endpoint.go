package server

import (
	"log"
	"net"

	"github.com/jpillora/chisel"
)

type Endpoint struct {
	id      int
	addr    string
	session chan net.Conn
}

func NewEndpoint(id int, addr string) *Endpoint {
	return &Endpoint{
		id:      id,
		addr:    addr,
		session: make(chan net.Conn),
	}
}

func (e *Endpoint) start() {

	laddr, _ := net.ResolveTCPAddr("tcp4", "127.0.0.1")
	raddr, err := net.ResolveTCPAddr("tcp4", e.addr)
	if err != nil {
		log.Fatal(err)
		return
	}

	for src := range e.session {

		dst, err := net.DialTCP("tcp4", laddr, raddr)
		if err != nil {
			log.Println(err)
			src.Close()
			continue
		}

		chisel.Pipe(src, dst)
	}
}
