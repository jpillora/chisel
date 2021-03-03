package socks5

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/jpillora/chisel/share/socks5/scope"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type RemoteUDPConn interface {
	Send(ctx context.Context, data []byte, remoteAddr *AddrSpec) error
	Close() error
}

type ContextGo interface {
	Ctx() context.Context
	GoNoError(f func())
	Go(f func() error)
	Cancel()
}

type UDPSendBack func(remoteAddr *AddrSpec, data []byte) error

type RemoteUDPConnFactory interface {
	MakeRemoteUDPConn(ctxClient ContextGo, ctxServer ContextGo, sendBack UDPSendBack, onBroken func()) (RemoteUDPConn, error)
	MaxUDPPacketSize() uint
}

/*********************************************************
    UDP PACKAGE to proxy
    +----+------+------+----------+----------+----------+
    |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
    +----+------+------+----------+----------+----------+
    | 2  |  1   |  1   | Variable |    2     | Variable |
    +----+------+------+----------+----------+----------+
**********************************************************/

func ParseUdpPacket(pkt []byte) (*AddrSpec, []byte, error) {
	if len(pkt) <= 2+1+1+1+2 {
		return nil, nil, fmt.Errorf("short UDP package header, %d bytes only", len(pkt))
	}
	if pkt[0] != 0 || pkt[1] != 0 {
		return nil, nil, fmt.Errorf("unsupported socks UDP package header (RSV != 0)")
	}
	if pkt[2] != 0 {
		return nil, nil, errors.New("UDP fragments not supported")
	}
	r := bytes.NewReader(pkt[3:])
	addr, err := readAddrSpec(r)
	return addr, pkt[3+int(r.Size())-r.Len():], err
}

func makeUDPResp(addr *AddrSpec, data []byte) []byte {
	msg := make([]byte, 3+addr.SerializedSize()+len(data))
	msg[0] = 0
	msg[1] = 0
	msg[2] = 0
	sz, _ := addr.SerializeTo(msg[3:])
	copy(msg[3+sz:], data)
	return msg
}

func MakeSendBackTo(udpConn *net.UDPConn) UDPSendBackTo {
	return func(remote *AddrSpec, data []byte, client *net.UDPAddr) error {
		_, err := udpConn.WriteToUDP(makeUDPResp(remote, data), client)
		return err
	}
}

func onAssociateReplyUdpAddrAndWaitForClose(conn net.Conn, udpAddr *AddrSpec, log ErrorLogger) error {
	if err := sendReply(conn, ReplySucceeded, udpAddr); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	// unset timeout for Read
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	// wait for connection to close
	log.Printf("onAssociate: tcp connection blocked")
	_, err := io.Copy(ioutil.Discard, conn)
	if err != nil {
		log.Printf("onAssociate: tcp connection unblocked: %s", err)
	} else {
		log.Printf("onAssociate: tcp connection unblocked")
	}

	return nil
}

func onAssociateSendError(conn net.Conn, err error) error {
	if e := sendReply(conn, ReplyServerFailure, nil); e != nil {
		return fmt.Errorf("failed to send reply: %v", e)
	}
	return err
}

type remoteUDPWrapper struct {
	conn   RemoteUDPConn
	lastTS int64
}

func (w *remoteUDPWrapper) refreshLastTS() {
	atomic.StoreInt64(&w.lastTS, time.Now().Unix())
}

type oneClientUDPRemotes struct {
	ctxClient *scope.ContextGroup

	mtx              sync.Mutex
	remotesByCliPort map[uint16]*remoteUDPWrapper
}

func makeOneClientUDPRemotes(ctxClient *scope.ContextGroup) *oneClientUDPRemotes {
	return &oneClientUDPRemotes{
		ctxClient:        ctxClient,
		remotesByCliPort: make(map[uint16]*remoteUDPWrapper),
	}
}

func (o *oneClientUDPRemotes) delRemote(clientPort uint16, toDel *remoteUDPWrapper) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	w := o.remotesByCliPort[clientPort]
	if w == toDel {
		delete(o.remotesByCliPort, clientPort)
	}
}

func (o *oneClientUDPRemotes) stopAll() {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	for _, ci := range o.remotesByCliPort {
		_ = ci.conn.Close()
	}
}

func (o *oneClientUDPRemotes) getOrAddRemote(
	ctxServer ContextGo, client *net.UDPAddr, connFactory RemoteUDPConnFactory, sendBack UDPSendBackTo,
) (*remoteUDPWrapper, error) {
	clientPort := uint16(client.Port)

	o.mtx.Lock()
	defer o.mtx.Unlock()

	w, found := o.remotesByCliPort[clientPort]
	if found {
		w.refreshLastTS()
		return w, nil
	}

	if len(o.remotesByCliPort) >= 4096 {
		var oldestPort uint16
		var oldestConn *remoteUDPWrapper
		for port, ci := range o.remotesByCliPort {
			if oldestConn == nil || atomic.LoadInt64(&ci.lastTS) < atomic.LoadInt64(&oldestConn.lastTS) {
				oldestPort = port
				oldestConn = ci
			}
		}
		if oldestConn != nil {
			delete(o.remotesByCliPort, oldestPort)
			_ = oldestConn.conn.Close()
		}
	}

	var err error
	w = &remoteUDPWrapper{nil, time.Now().Unix()}
	w.conn, err = connFactory.MakeRemoteUDPConn(o.ctxClient, ctxServer, func(remoteAddr *AddrSpec, data []byte) error {
		w.refreshLastTS()
		return sendBack(remoteAddr, data, client)
	}, func() { o.delRemote(clientPort, w) })
	if err != nil {
		return nil, err
	}
	o.remotesByCliPort[clientPort] = w
	return w, nil
}

func (o *oneClientUDPRemotes) forwardClientPktToRemote(
	ctxServer ContextGo, src *net.UDPAddr, pkt []byte, connFactory RemoteUDPConnFactory, sendBack UDPSendBackTo,
) error {
	destAddr, data, err := ParseUdpPacket(pkt)
	if err != nil {
		return fmt.Errorf("error parsing udp packet: %w", err)
	}

	w, err := o.getOrAddRemote(ctxServer, src, connFactory, sendBack)
	if err != nil {
		return fmt.Errorf("can't get remote: %w", err)
	}
	w.refreshLastTS()
	if err = w.conn.Send(o.ctxClient.Ctx(), data, destAddr); err != nil {
		o.delRemote(uint16(src.Port), w)
		_ = w.conn.Close()
		return fmt.Errorf("error sending to remote: %w", err)
	}
	return nil
}
