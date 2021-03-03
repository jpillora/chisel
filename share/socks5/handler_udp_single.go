package socks5

import (
	"context"
	"net"
)

type SinglePortUDPHandler struct {
	udpHandler

	// used for udp associate, defaults to automatically chosen free UDP port
	UDPListenPort int

	udp *SingleUDPPortAssociate
}

func (s *SinglePortUDPHandler) OnStartServe(ctxServer ContextGo, l net.Listener) error {
	if err := s.udpHandler.OnStartServe(ctxServer, l); err != nil {
		return err
	}

	udpAddr := &AddrSpec{IP: s.UDPListenIP, Port: s.UDPListenPort}
	s.udp = MakeSingleUDPPortAssociate(udpAddr, s, s.Logger)

	if err := s.udp.ListenAndServeUDPPort(ctxServer, s.UDPListenNet); err != nil {
		return err
	}
	s.UDPListenPort = udpAddr.Port // save auto-chosen port, if it was chosen automatically

	return nil
}

func (s *SinglePortUDPHandler) OnAssociate(_ context.Context, conn net.Conn, _ *Request) error {
	return s.udp.OnAssociate(conn)
}
