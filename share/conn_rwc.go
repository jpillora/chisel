package chshare

import (
	"io"
	"net"
	"time"
)

type rwcConn struct {
	io.ReadWriteCloser
	buff []byte
}

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
