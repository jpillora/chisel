package socks5

import (
	"context"
	"net"
)

type MultiPortUDPHandler struct {
	udpHandler

	udp *MultiUDPPortAssociate
}

func (m *MultiPortUDPHandler) OnStartServe(ctxServer ContextGo, l net.Listener) error {
	if err := m.udpHandler.OnStartServe(ctxServer, l); err != nil {
		return err
	}
	m.udp = MakeMultiUDPPortAssociate(ctxServer, m.UDPListenIP, m.UDPListenNet, m, m.Logger)
	return nil
}

func (m *MultiPortUDPHandler) OnAssociate(ctx context.Context, conn net.Conn, _ *Request) error {
	return m.udp.OnAssociate(ctx, conn)
}
