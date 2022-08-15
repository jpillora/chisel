package tunnel

import (
	"context"
	"encoding/gob"
	"fmt"
	"github.com/jpillora/chisel/share/cio"
	"github.com/meteorite/scope"
	"github.com/meteorite/socks5"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
)

type socksHandler struct {
	p            *Proxy
	udpLocalAddr *socks5.AddrSpec
	sl           *log.Logger
	udp          *socks5.SingleUDPPortAssociate
}

func newSocksHandler(p *Proxy, localUDPAddr *net.UDPAddr, sl *log.Logger) *socksHandler {
	return &socksHandler{
		p: p,
		udpLocalAddr: &socks5.AddrSpec{
			IP:   localUDPAddr.IP,
			Port: localUDPAddr.Port,
		},
		sl: sl,
	}
}

func (h *socksHandler) OnStartServe(ctxServer socks5.ContextGo, _ net.Listener) error {
	h.udp = socks5.MakeSingleUDPPortAssociate(h.udpLocalAddr, h, h.sl)
	return h.udp.ListenAndServeUDPPort(ctxServer, "udp")
}

func (h *socksHandler) ErrLog() socks5.ErrorLogger {
	return h.sl
}

func (h *socksHandler) OnConnect(ctx context.Context, conn net.Conn, req *socks5.Request) error {
	return h.p.pipeRemote(ctx, conn, req.DestAddr.Address()+"/sot", func(dst ssh.Channel) error {
		code := []byte{0}
		_, err := dst.Read(code)
		if err != nil {
			return fmt.Errorf("can't receive socks code from server: %w", err)
		}
		if code[0] != socks5.ReplySucceeded {
			if err = req.SendError(conn, code[0]); err != nil {
				return fmt.Errorf("failed to send reply to client: %w", err)
			}
			return fmt.Errorf("can't connect to destination server (code: %d)", code[0])
		}
		if err := req.SendConnectSuccess(conn); err != nil {
			return fmt.Errorf("failed to send reply to client: %w", err)
		}
		return nil
	})
}

func (h *socksHandler) OnAssociate(_ context.Context, conn net.Conn, _ *socks5.Request) error {
	return h.udp.OnAssociate(conn)
}

func (h *socksHandler) MaxUDPPacketSize() uint {
	return maxMTU
}


type socksUdpConnector struct {
	*cio.Logger
	outbound *udpChannel
}

func (h *socksHandler) MakeRemoteUDPConn(
	ctxClient socks5.ContextGo, _ socks5.ContextGo, sendBack socks5.UDPSendBack, onBroken func(),
) (socks5.RemoteUDPConn, error) {
	sshConn := h.p.sshTun.getSSH(ctxClient.Ctx())
	if sshConn == nil {
		return nil, fmt.Errorf("ssh-conn nil")
	}
	dstSpec := "/sou" //just "/sou" since the remote destination address is sent with each packet
	rwc, reqs, err := sshConn.OpenChannel("chisel", []byte(dstSpec))
	if err != nil {
		return nil, fmt.Errorf("ssh-chan error: %w", err)
	}
	ctxClient.GoNoError(func() { ssh.DiscardRequests(reqs) })

	c := &socksUdpConnector{
		Logger: h.p.Logger,
		outbound: &udpChannel{
			r: gob.NewDecoder(rwc),
			w: gob.NewEncoder(rwc),
			c: rwc,
		},
	}
	ctxClient.GoNoError(func() {
		defer onBroken()
		defer scope.Closer(ctxClient.Ctx(), c.outbound.c).Close()

		for {
			//receive from channel, including source address
			p := udpPacket{}
			c.Debugf("reading next udp packet from ssh channel to remote")
			if err := c.outbound.decode(&p); err != nil {
				c.Debugf("decode error: %s", err)
				return
			}

			//parse source address
			fromAddr, err := socks5.ParseHostPort(p.Src)
			if err != nil {
				c.Debugf("error parsing received packet source spec: %s: %s", p.Src, err)
				continue
			}

			//write back to inbound udp
			err = sendBack(fromAddr, p.Payload)
			if err != nil {
				c.Debugf("send back error: %s", err)
				return
			}
		}
	})
	c.Debugf("new ssh channel for udp is created")
	return c, nil
}

func (c *socksUdpConnector) Send(_ context.Context, data []byte, remoteAddr *socks5.AddrSpec) error {
	return c.outbound.encode(remoteAddr.Address(), data)
}
func (c *socksUdpConnector) Close() error {
	return c.outbound.c.Close()
}
