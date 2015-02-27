package server

import (
	"encoding/binary"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
)

type WebSocket struct {
	id     int
	config *chisel.Config
	conn   net.Conn
}

func NewWebSocket(id int, config *chisel.Config, conn net.Conn) *WebSocket {
	return &WebSocket{
		id:     id,
		config: config,
		conn:   conn,
	}
}

func (w *WebSocket) handle() {

	chisel.Printf("Websocket [%d] Open", w.id)

	// Setup server side of yamux
	session, err := yamux.Server(w.conn, nil)
	if err != nil {
		chisel.Printf("Yamux server: %s", err)
		return
	}

	endpoints := make([]*Endpoint, len(w.config.Remotes))

	// Create an endpoint for each required
	for id, r := range w.config.Remotes {
		addr := r.RemoteHost + ":" + r.RemotePort
		e := NewEndpoint(w, id, addr)
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
			chisel.Printf("Session accept: %s", err)
			continue
		}
		go w.handleStream(stream, endpoints)
	}
}

func (w *WebSocket) handleStream(stream net.Conn, endpoints []*Endpoint) {
	// extract endpoint id
	b := make([]byte, 2)
	n, err := stream.Read(b)
	if err != nil {
		chisel.Printf("Stream initial read: %s", err)
		return
	}
	if n != 2 {
		chisel.Printf("Should read 2 bytes...")
		return
	}
	id := binary.BigEndian.Uint16(b)

	//then pipe
	e := endpoints[id]
	e.sessions <- stream
}

func (w *WebSocket) teardown() {
	chisel.Printf("Websocket [%d] Closed", w.id)
}
