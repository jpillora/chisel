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
		Logger: s.Logger.Fork("websocket %d", id),
		id:     id,
		config: config,
		conn:   conn,
	}
}

func (w *webSocket) handle() {

	w.Infof("Open")

	// Setup server side of yamux
	session, err := yamux.Server(w.conn, nil)
	if err != nil {
		w.Infof("Yamux server: %s", err)
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
				w.teardown()
				break
			}
			w.Infof("Session accept: %s", err)
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
		w.Infof("Stream initial read: %s", err)
		return
	}
	if n != 2 {
		w.Infof("Should read 2 bytes...")
		return
	}
	id := binary.BigEndian.Uint16(b)

	if int(id) >= len(endpoints) {
		w.Infof("Invalid endpoint id")
		return
	}

	//then pipe
	e := endpoints[id]
	e.sessions <- stream
}

func (w *webSocket) teardown() {
	w.Infof("Closed")
}
