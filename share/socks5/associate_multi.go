package socks5

import (
	"context"
	"github.com/jpillora/chisel/share/socks5/scope"
	"net"
)

type MultiUDPPortAssociate struct {
	ctxServer   ContextGo
	listenIP    net.IP
	udpNet      string
	connFactory RemoteUDPConnFactory
	log         ErrorLogger
}

// Creates new MultiUDPPortAssociate.
func MakeMultiUDPPortAssociate(
	ctxServer ContextGo, listenIP net.IP, udpNet string, connFactory RemoteUDPConnFactory, log ErrorLogger,
) *MultiUDPPortAssociate {
	return &MultiUDPPortAssociate{
		ctxServer:   ctxServer,
		listenIP:    listenIP,
		udpNet:      udpNet,
		connFactory: connFactory,
		log:         log,
	}
}

func (m *MultiUDPPortAssociate) OnAssociate(ctx context.Context, conn net.Conn) error {
	from, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return onAssociateSendError(conn, err)
	}

	c, err := net.ListenUDP(m.udpNet, &net.UDPAddr{IP: m.listenIP})
	if err != nil {
		return onAssociateSendError(conn, err)
	}

	ctxClient, _ := scope.Group(ctx)
	defer ctxClient.AddCloser(c).WaitQuietly()

	one := makeOneClientUDPRemotes(ctxClient)
	ctxClient.Go(func() error { return m.serveOneUDPPort(c, from, one) })

	udpAddr := &AddrSpec{IP: m.listenIP, Port: c.LocalAddr().(*net.UDPAddr).Port}
	return onAssociateReplyUdpAddrAndWaitForClose(conn, udpAddr, m.log)
}

func (m *MultiUDPPortAssociate) serveOneUDPPort(udpConn *net.UDPConn, clientIP string, one *oneClientUDPRemotes) error {
	sendBack := MakeSendBackTo(udpConn)

	buffer := make([]byte, m.connFactory.MaxUDPPacketSize())
	for {
		n, src, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}

		fromIP := src.IP.String()
		if fromIP != clientIP {
			m.log.Printf("udp packet from unauthorized IP: %s != %s", fromIP, clientIP)
			continue
		}

		if err := one.forwardClientPktToRemote(m.ctxServer, src, buffer[:n], m.connFactory, sendBack); err != nil {
			m.log.Printf("%v", err)
		}
	}
}
