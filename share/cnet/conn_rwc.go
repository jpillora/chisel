package cnet

import (
	"io"
	"net"
	"time"
)

type rwcConn struct {
	io.ReadWriteCloser
	buff []byte
}

type closeWriter interface {
	CloseWrite() error
}

//NewRWCConn converts a RWC into a net.Conn
func NewRWCConn(rwc io.ReadWriteCloser) net.Conn {
	c := rwcConn{
		ReadWriteCloser: rwc,
	}
	return &c
}

func (c *rwcConn) LocalAddr() net.Addr {
	return c
}

func (c *rwcConn) RemoteAddr() net.Addr {
	return c
}

func (c *rwcConn) Network() string {
	return "tcp"
}

func (c *rwcConn) String() string {
	return ""
}

func (c *rwcConn) SetDeadline(t time.Time) error {
	return nil //no-op
}

func (c *rwcConn) SetReadDeadline(t time.Time) error {
	return nil //no-op
}

func (c *rwcConn) SetWriteDeadline(t time.Time) error {
	return nil //no-op
}

//CloseWrite propagates half-closes to the underlying
//connection when supported (e.g. ssh.Channel). When the underlying
//stream cannot half-close, it falls back to a full Close — sacrificing
//any in-flight reverse traffic — so the peer observes EOF rather than
//cio.Pipe hanging on a half-close that never happened
func (c *rwcConn) CloseWrite() error {
	if cw, ok := c.ReadWriteCloser.(closeWriter); ok {
		return cw.CloseWrite()
	}
	//underlying stream can't half-close; fall back to a full close so the
	//peer still observes EOF (otherwise cio.Pipe treats the no-op as a
	//successful half-close and the opposite copy can block forever)
	return c.Close()
}
