package cnet

import (
	"net"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jpillora/chisel/share/settings"
)

type wsConn struct {
	*websocket.Conn
	buff []byte
}

const (
	// x/crypto/ssh rejects transport packets whose length field exceeds this,
	// so a full wire packet tops out around 256 KiB plus the 4-byte length and
	// a MAC. Doubling leaves the exact framing overhead unspecified rather than
	// tracking a dependency's internals, while retaining a finite bound on
	// messages received before SSH authentication.
	sshMaxTransportPacket     = 256 * 1024
	defaultWebSocketReadLimit = 2 * sshMaxTransportPacket
)

// NewWebSocketConn converts a websocket.Conn into a net.Conn
func NewWebSocketConn(websocketConn *websocket.Conn) net.Conn {
	websocketConn.SetReadLimit(configuredWebSocketReadLimit())
	c := wsConn{
		Conn: websocketConn,
	}
	return &c
}

func configuredWebSocketReadLimit() int64 {
	// Only 0 disables the limit. Treat negative values as invalid rather than
	// letting gorilla interpret them as another unlimited setting.
	limit := settings.EnvInt("WS_READ_LIMIT", defaultWebSocketReadLimit)
	if limit < 0 {
		limit = defaultWebSocketReadLimit
	}
	return int64(limit)
}

// Read is not threadsafe though that's okay since there
// should never be more than one reader
func (c *wsConn) Read(dst []byte) (int, error) {
	ldst := len(dst)
	//use buffer or read new message
	var src []byte
	if len(c.buff) > 0 {
		src = c.buff
		c.buff = nil
	} else if _, msg, err := c.Conn.ReadMessage(); err == nil {
		src = msg
	} else {
		return 0, err
	}
	//copy src->dest
	var n int
	if len(src) > ldst {
		//copy as much as possible of src into dst
		n = copy(dst, src[:ldst])
		//copy remainder into buffer
		r := src[ldst:]
		lr := len(r)
		c.buff = make([]byte, lr)
		copy(c.buff, r)
	} else {
		//copy all of src into dst
		n = copy(dst, src)
	}
	//return bytes copied
	return n, nil
}

func (c *wsConn) Write(b []byte) (int, error) {
	if err := c.Conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
		return 0, err
	}
	n := len(b)
	return n, nil
}

func (c *wsConn) SetDeadline(t time.Time) error {
	if err := c.Conn.SetReadDeadline(t); err != nil {
		return err
	}
	return c.Conn.SetWriteDeadline(t)
}
