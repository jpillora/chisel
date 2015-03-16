package chserver

import (
	"net"

	"github.com/jpillora/chisel/share"
)

type endpoint struct {
	*chshare.Logger
	id       int
	count    int
	addr     string
	sessions chan net.Conn
}

func newEndpoint(w *webSocket, id int, addr string) *endpoint {
	return &endpoint{
		Logger:   w.Logger.Fork("%s#%d", addr, id+1),
		id:       id,
		addr:     addr,
		sessions: make(chan net.Conn),
	}
}

func (e *endpoint) start() {
	e.Debugf("Enabled")
	//service incoming streams
	for stream := range e.sessions {
		go e.pipe(stream)
	}
}

func (e *endpoint) pipe(src net.Conn) {
	dst, err := net.Dial("tcp", e.addr)
	if err != nil {
		e.Debugf("%s", err)
		src.Close()
		return
	}

	e.count++
	eid := e.count
	e.Debugf("stream#%d: Open", eid)
	s, r := chshare.Pipe(src, dst)
	e.Debugf("stream#%d: Close (sent %d received %d)", eid, s, r)
}
