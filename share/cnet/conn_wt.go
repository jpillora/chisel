package cnet

import (
	"net"

	"github.com/quic-go/webtransport-go"
)

type wtConn struct {
	webtransport.Stream
}

// NewWebTransportConn converts a webtransport.Stream into a net.Conn
func NewWebTransportConn(webtransportConn webtransport.Stream) net.Conn {
	c := wtConn{
		Stream: webtransportConn,
	}
	return &c
}

func (c *wtConn) LocalAddr() net.Addr {
	return c
}

func (c *wtConn) RemoteAddr() net.Addr {
	return c
}

func (c *wtConn) Network() string {
	return "udp"
}

func (c *wtConn) String() string {
	return ""
}
