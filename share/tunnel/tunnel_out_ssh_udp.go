package tunnel

import (
	"context"
	"encoding/gob"
	"github.com/meteorite/scope"
	"golang.org/x/sync/errgroup"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
)

const maxMTU = 9012

func (t *Tunnel) handleUDP(l *cio.Logger, rwc io.ReadWriteCloser, hostPort string) error {
	conns := &udpConns{
		Logger: l,
		m:      map[string]*udpConn{},
	}
	defer conns.closeAll()
	h := &udpHandler{
		Logger:   l,
		hostPort: hostPort,
		udpChannel: &udpChannel{
			r: gob.NewDecoder(rwc),
			w: gob.NewEncoder(rwc),
			c: rwc,
		},
		udpConns: conns,
	}
	for {
		p := udpPacket{}
		if err := h.handleWrite(&p); err != nil {
			return err
		}
	}
}

type udpHandler struct {
	*cio.Logger
	hostPort string
	*udpChannel
	*udpConns
}

func (h *udpHandler) handleWrite(p *udpPacket) error {
	if err := h.r.Decode(&p); err != nil {
		return err
	}
	//dial now, we know we must write
	conn, exists, err := h.udpConns.dial(p.Src, h.hostPort)
	if err != nil {
		return err
	}
	//however, we dont know if we must read...
	//spawn up to <max-conns> go-routines to wait
	//for a reply.
	//TODO configurable
	//TODO++ dont use go-routines, switch to pollable
	//  array of listeners where all listeners are
	//  sweeped periodically, removing the idle ones
	const maxConns = 100
	if !exists {
		if h.udpConns.len() <= maxConns {
			go h.handleRead(p, conn)
		} else {
			h.Debugf("exceeded max udp connections (%d)", maxConns)
		}
	}
	_, err = conn.Write(p.Payload)
	if err != nil {
		return err
	}
	return nil
}

func (h *udpHandler) handleRead(p *udpPacket, conn *udpConn) {
	//ensure connection is cleaned up
	defer h.udpConns.remove(conn.id)
	buff := make([]byte, maxMTU)
	for {
		//response must arrive within 15 seconds
		deadline := settings.EnvDuration("UDP_DEADLINE", 15*time.Second)
		conn.SetReadDeadline(time.Now().Add(deadline))
		//read response
		n, err := conn.Read(buff)
		if err != nil {
			if !os.IsTimeout(err) && err != io.EOF {
				h.Debugf("read error: %s", err)
			}
			break
		}
		b := buff[:n]
		//encode back over ssh connection
		err = h.udpChannel.encode(p.Src, b)
		if err != nil {
			h.Debugf("encode error: %s", err)
			return
		}
	}
}

type udpConns struct {
	*cio.Logger
	sync.Mutex
	m map[string]*udpConn
}

func (cs *udpConns) dial(id, addr string) (*udpConn, bool, error) {
	cs.Lock()
	defer cs.Unlock()
	conn, ok := cs.m[id]
	if !ok {
		c, err := net.Dial("udp", addr)
		if err != nil {
			return nil, false, err
		}
		conn = &udpConn{
			id:   id,
			Conn: c, // cnet.MeterConn(cs.Logger.Fork(addr), c),
		}
		cs.m[id] = conn
	}
	return conn, ok, nil
}

func (cs *udpConns) len() int {
	cs.Lock()
	l := len(cs.m)
	cs.Unlock()
	return l
}

func (cs *udpConns) remove(id string) {
	cs.Lock()
	delete(cs.m, id)
	cs.Unlock()
}

func (cs *udpConns) closeAll() {
	cs.Lock()
	for id, conn := range cs.m {
		conn.Close()
		delete(cs.m, id)
	}
	cs.Unlock()
}

type udpConn struct {
	id string
	net.Conn
}


func (t *Tunnel) handleSocksUDP(l *cio.Logger, rwc io.ReadWriteCloser) error {
	udpChannel := &udpChannel{
		r: gob.NewDecoder(rwc),
		w: gob.NewEncoder(rwc),
		c: rwc,
	}

	g, ctx := errgroup.WithContext(context.Background())

	// reserve UDP port for this outbound socks connector
	// we listen worldwide to make our NAT full cone and allow STUN scenario and other similar scenarios
	remoteConn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return err
	}

	// close both UDP port and SSH channel, when either forward or backward handler encounters an error and finishes
	defer scope.Closer(ctx, remoteConn, udpChannel.c).Close()

	// launch backward traffic handler
	g.Go(func() error {
		buff := make([]byte, maxMTU)
		for {
			// receive next backward UDP packet
			n, remoteAddr, err := remoteConn.ReadFrom(buff)
			if err != nil {
				l.Debugf("receive return packet error: %s", err)
				return err
			}

			// encode it back over ssh connection with remote host:port, which returned it
			err = udpChannel.encode(remoteAddr.String(), buff[:n])
			if err != nil {
				l.Debugf("encode error: %s", err)
				return err
			}
		}
	})

	// launch forward traffic handler
	g.Go(func() error {
		for {
			// receive next forward packet from ssh
			p := udpPacket{}
			if err := udpChannel.decode(&p); err != nil {
				return err
			}

			// send it to remote host
			dst := p.Src  // for socks UDP scenario, this is really a destination, where we should send this packet
			rUDPAddr, err := net.ResolveUDPAddr("udp", dst)
			if err != nil {
				return err
			}
			if _, err = remoteConn.WriteTo(p.Payload, rUDPAddr); err != nil {
				return err
			}
		}
	})

	return g.Wait()
}
