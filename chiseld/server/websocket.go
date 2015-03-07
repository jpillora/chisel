package server

import (
	"encoding/binary"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
)

type webSocket struct {
	*chisel.Logger
	id     int
	config *chisel.Config
	conn   net.Conn
}

func newWebSocket(s *Server, config *chisel.Config, conn net.Conn) *webSocket {
	id := s.wsCount
	return &webSocket{
		Logger: s.Logger.Fork("conn#%d", id),
		id:     id,
		config: config,
		conn:   conn,
	}
}

func (w *webSocket) handle() {

	w.Debugf("Open")

	// queue teardown
	defer w.teardown()

	// Setup server side of yamux
	session, err := yamux.Server(w.conn, nil)
	if err != nil {
		w.Debugf("Yamux server: %s", err)
		return
	}

	endpoints := make([]*endpoint, len(w.config.Remotes))

	// Create an endpoint for each required
	for id, r := range w.config.Remotes {
		addr := r.RemoteHost + ":" + r.RemotePort
		e := newEndpoint(w, id, addr)
		go e.start()
		endpoints[id] = e
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			if session.IsClosed() {
				break
			}
			w.Debugf("Session accept: %s", err)
			continue
		}
		go w.handleStream(stream, endpoints)
	}
}

func (w *webSocket) handleStream(stream net.Conn, endpoints []*endpoint) {
	// extract endpoint id
	b := make([]byte, 2)
	n, err := stream.Read(b)
	if err != nil {
		stream.Close()
		w.Debugf("Stream initial read: %s", err)
		return
	}
	if n != 2 {
		stream.Close()
		w.Debugf("Should read 2 bytes...")
		return
	}
	id := binary.BigEndian.Uint16(b)

	if int(id) >= len(endpoints) {
		stream.Close()
		w.Debugf("Invalid endpoint index: %d [total endpoints %d]", id, len(endpoints))
		return
	}

	// pass this stream to the
	// desired endpoint
	e := endpoints[id]
	e.sessions <- stream
}

func (w *webSocket) teardown() {
	w.Debugf("Closed")
}
