package server

import (
	"net"

	"github.com/jpillora/chisel"
)

type endpoint struct {
	w        *webSocket
	id       int
	count    int
	addr     string
	sessions chan net.Conn
}

func newEndpoint(w *webSocket, id int, addr string) *endpoint {
	return &endpoint{
		w:        w,
		id:       id,
		addr:     addr,
		sessions: make(chan net.Conn),
	}
}

func (e *endpoint) start() {
	chisel.Printf("Websocket [%d] Proxy [%d] Activate (%s)", e.w.id, e.id, e.addr)
	//waiting for incoming streams
	for stream := range e.sessions {
		go e.pipe(stream)
	}
}

func (e *endpoint) pipe(src net.Conn) {
	dst, err := net.Dial("tcp", e.addr)
	if err != nil {
		chisel.Printf("%s", err)
		src.Close()
		return
	}

	e.count++
	c := e.count
	chisel.Printf("Websocket [%d] Proxy [%d] Connection [%d] Open", e.w.id, e.id, c)
	chisel.Pipe(src, dst)
	chisel.Printf("Websocket [%d] Proxy [%d] Connection [%d] Closed", e.w.id, e.id, c)
}
