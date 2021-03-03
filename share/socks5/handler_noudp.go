package socks5

import (
	"context"
	"fmt"
	"net"
)

type NoUDPHandler struct {
	tcpHandler
}

func (n *NoUDPHandler) OnAssociate(_ context.Context, conn net.Conn, _ *Request) error {
	if err := sendReply(conn, ReplyCommandNotSupported, nil); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}
	return nil
}
