package socks5

import (
	"context"
	"fmt"
	"github.com/jpillora/chisel/share/socks5/scope"
	"golang.org/x/sync/errgroup"
	"io"
	"log"
	"net"
	"os"
	"strings"
)

type Handler interface {
	// Called, when Serve is called on socks server. Server will be really started only after returning from this func.
	// Returned error will abort socks server starting.
	OnStartServe(ctxServer ContextGo, tcp net.Listener) error

	// Must return valid non-nil ErrorLogger
	// May be called only after OnStartServe is called
	ErrLog() ErrorLogger

	// Called on every "connect" query from client. May block if needed, but must obey ctx cancellation.
	// Returned error will only abort current client's connection and will not stop server.
	OnConnect(ctx context.Context, conn net.Conn, req *Request) error

	// Called on every "associate" query from client. May block if needed, but must obey ctx cancellation
	// Returned error will only abort current client's connection and will not stop server.
	OnAssociate(ctx context.Context, conn net.Conn, req *Request) error
}

type tcpHandler struct {
	// Optional function for dialing out for connect
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	// can be used to provide a custom log target.
	// Defaults to stdout.
	Logger ErrorLogger
}

func (t *tcpHandler) OnStartServe(_ ContextGo, _ net.Listener) error {
	if t.Dial == nil {
		t.Dial = func(ctx context.Context, net_, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, net_, addr)
		}
	}

	// Ensure we have a log target
	if t.Logger == nil {
		t.Logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	return nil
}

func (t *tcpHandler) ErrLog() ErrorLogger {
	return t.Logger
}

func (t *tcpHandler) OnConnect(ctx context.Context, conn net.Conn, req *Request) error {
	// Attempt to connect
	target, err := t.Dial(ctx, "tcp", req.realDestAddr.Address())
	if err != nil {
		resp := DialErrorToSocksCode(err)
		if err := sendReply(conn, resp, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %v", req.DestAddr, err)
	}
	defer scope.Closer(ctx, target).Close()

	// Send success
	if err := sendReply(conn, ReplySucceeded, nil); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	// Start proxying
	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error { return proxy(target, req.bufConn) })
	g.Go(func() error { return proxy(conn, target) })
	return g.Wait()
}

type closeWriter interface {
	CloseWrite() error
}

// proxy is used to transmit data from src to destination
func proxy(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if tcpConn, ok := dst.(closeWriter); ok {
		_ = tcpConn.CloseWrite()
		//if e := tcpConn.CloseWrite(); e != nil {
		//	d.Server.config.Logger.Printf("error in CloseWrite: %v", e)
		//}
	}
	return err
}

type udpHandler struct {
	tcpHandler

	// used for udp associate, defaults to TCP listen IP
	UDPListenIP net.IP

	// used for udp associate, defaults to analogous to TCP net
	UDPListenNet string

	// max size of UDP packet data in bytes (used for allocated buffers size)
	// Defaults to 2048
	UDPMaxPacketSize uint
}

func (u *udpHandler) OnStartServe(ctxServer ContextGo, l net.Listener) error {
	if err := u.tcpHandler.OnStartServe(ctxServer, l); err != nil {
		return err
	}

	if len(u.UDPListenIP) == 0 || u.UDPListenIP.IsUnspecified() {
		host, _, _ := net.SplitHostPort(l.Addr().String())
		u.UDPListenIP = net.ParseIP(host)
	}

	if u.UDPListenNet == "" {
		u.UDPListenNet = strings.ReplaceAll(l.Addr().Network(), "tcp", "udp")
	}

	if u.UDPMaxPacketSize <= 0 {
		u.UDPMaxPacketSize = 2048
	}

	if u.Logger == nil {
		u.Logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	return nil
}


type defaultRemoteUDPConn struct {
	conn net.PacketConn
}

func (c *defaultRemoteUDPConn) Send(_ context.Context, data []byte, remoteAddr *AddrSpec) error {
	rUDPAddr, err := net.ResolveUDPAddr("udp", remoteAddr.Address())
	if err != nil {
		return err
	}
	_, err = c.conn.WriteTo(data, rUDPAddr)
	return err
}

func (c *defaultRemoteUDPConn) Close() error {
	return c.conn.Close()
}

func (u *udpHandler) MakeRemoteUDPConn(
	ctxClient ContextGo, _ ContextGo, sendBack UDPSendBack, onBroken func(),
) (RemoteUDPConn, error) {
	// reserve outbound UDP port for sending outbound packets and receiving replies
	var err error
	remoteConn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}

	// launch backward traffic handler
	ctxClient.GoNoError(func() {
		defer onBroken()
		defer scope.Closer(ctxClient.Ctx(), remoteConn).Close()

		p := make([]byte, u.UDPMaxPacketSize)
		for {
			// recv reply from remote host
			n, remoteAddr, err := remoteConn.ReadFrom(p)
			if err != nil {
				return
			}

			// parse remote addr, from where reply packet was received
			remoteUDPAddr, ok := remoteAddr.(*net.UDPAddr)
			if !ok {
				remoteUDPAddr, err = net.ResolveUDPAddr("udp", remoteUDPAddr.String())
				if err != nil {
					continue
				}
			}

			// send reply packet back to socks client
			err = sendBack(&AddrSpec{IP: remoteUDPAddr.IP, Port: remoteUDPAddr.Port}, p[:n])
			if err != nil {
				return
			}
		}
	})

	return &defaultRemoteUDPConn{remoteConn}, nil
}

func (u *udpHandler) MaxUDPPacketSize() uint {
	return u.UDPMaxPacketSize
}
