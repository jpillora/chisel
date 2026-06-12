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

//NewWebSocketConn converts a websocket.Conn into a net.Conn
func NewWebSocketConn(websocketConn *websocket.Conn) net.Conn {
	//ssh packets are at most ~35KB, so cap inbound messages to
	//prevent pre-auth memory exhaustion from oversized frames.
	//tune with WS_READ_LIMIT (0 disables the limit)
	websocketConn.SetReadLimit(int64(settings.EnvInt("WS_READ_LIMIT", 64*1024)))
	c := wsConn{
		Conn: websocketConn,
	}
	return &c
}

//Read is not threadsafe though that's okay since there
//should never be more than one reader
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
