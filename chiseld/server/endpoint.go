package server

import (
	"net"

	"github.com/jpillora/chisel"
)

type endpoint struct {
	*chisel.Logger
	id       int
	count    int
	addr     string
	sessions chan net.Conn
}

func newEndpoint(w *webSocket, id int, addr string) *endpoint {
	return &endpoint{
		Logger:   w.Logger.Fork("#%d %s", id, addr),
		id:       id,
		addr:     addr,
		sessions: make(chan net.Conn),
	}
}

func (e *endpoint) start() {
	e.Infof("Activate")
	//service incoming streams
	for stream := range e.sessions {
		go e.pipe(stream)
	}
}

func (e *endpoint) pipe(src net.Conn) {
	dst, err := net.Dial("tcp", e.addr)
	if err != nil {
		e.Infof("%s", err)
		src.Close()
		return
	}

	e.count++
	eid := e.count
	e.Infof("Open #%d", eid)
	chisel.Pipe(src, dst)
	e.Infof("Closed #%d", eid)
}
