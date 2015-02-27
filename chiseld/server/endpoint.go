package server

import (
	"log"
	"net"

	"github.com/jpillora/chisel"
)

type Endpoint struct {
	id      int
	count   int
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
	//waiting for incoming streams
	for stream := range e.session {
		go e.pipe(stream)
	}
}

func (e *Endpoint) pipe(src net.Conn) {
	dst, err := net.Dial("tcp", e.addr)
	if err != nil {
		log.Println(err)
		src.Close()
		return
	}

	e.count++
	c := e.count
	log.Printf("[#%d] openned connection %d", e.id, c)
	chisel.Pipe(src, dst)
	log.Printf("[#%d] closed connection %d", e.id, c)
}
