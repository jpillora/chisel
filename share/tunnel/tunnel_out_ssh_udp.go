package tunnel

import (
	"context"
	"encoding/gob"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
)

func (t *Tunnel) handleUDP(l *cio.Logger, rwc io.ReadWriteCloser, hostPort string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conns := &udpConns{
		Logger: l,
		m:      map[string]*udpConn{},
		ctx:    ctx,
	}
	defer t.closeAllUDPConnections(conns)
	h := &udpHandler{
		Logger:   l,
		hostPort: hostPort,
		udpChannel: &udpChannel{
			r: gob.NewDecoder(rwc),
			w: gob.NewEncoder(rwc),
			c: rwc,
		},
		udpConns: conns,
		maxMTU:   settings.EnvInt("UDP_MAX_SIZE", 9012),
	}
	h.Debugf("UDP max size: %d bytes", h.maxMTU)
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
	maxMTU int
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
	//  array of listeners
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
	conn.lastUsed.Store(time.Now().UnixMilli())
	return nil
}

func (h *udpHandler) handleRead(p *udpPacket, conn *udpConn) {
	//ensure connection is cleaned up
	defer h.udpConns.remove(conn.id)
	buff := make([]byte, h.maxMTU)
	for {
		//read response
		n, err := conn.Read(buff)
		if err != nil {
			if conn.closedBySweeperOrTunnel.Load() || os.IsTimeout(err) || err == io.EOF {
				break
			}
			h.Debugf("read error: %s", err)
			break
		}
		b := buff[:n]
		//encode back over ssh connection
		err = h.udpChannel.encode(p.Src, b)
		if err != nil {
			h.Debugf("encode error: %s", err)
			return
		}
		conn.lastUsed.Store(time.Now().UnixMilli())
	}
}

type udpConns struct {
	*cio.Logger
	sync.Mutex
	m map[string]*udpConn
	ctx context.Context
	sweeping bool
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
		conn.lastUsed.Store(time.Now().UnixMilli())
		cs.m[id] = conn
		cs.ensureSweeping()
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

func (t *Tunnel) closeAllUDPConnections(cs *udpConns) {
	cs.Lock()
	for id, conn := range cs.m {
		conn.closedBySweeperOrTunnel.Store(true)
		conn.Close()
		delete(cs.m, id)
	}
	cs.Unlock()
}

type udpConn struct {
	id string
	net.Conn
	lastUsed atomic.Int64
	closedBySweeperOrTunnel atomic.Bool
}

func (cs *udpConns) ensureSweeping() {
	if (cs.sweeping) {
		return;
	}
	cs.Debugf("start sweeping udp connections")
	cs.sweeping = true;
	// Evaluate UDP_DEADLINE for backward compatibility,
	// prefer UDP_IDLE_TIMEOUT.
	timeout := settings.EnvDuration("UDP_IDLE_TIMEOUT", settings.EnvDuration("UDP_DEADLINE", 15*time.Second))
	interval := max(min(3*time.Second, timeout/2), 100 * time.Millisecond)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cs.Lock()

				if len(cs.m) == 0 {
					cs.Debugf("stop sweeping udp connections")
					cs.sweeping = false;
					cs.Unlock()
					return;
				}

				now := time.Now()

				for id, conn := range cs.m {
					if now.Sub(time.UnixMilli(conn.lastUsed.Load())) > timeout {
						conn.closedBySweeperOrTunnel.Store(true);
						conn.Close()
						delete(cs.m, id)
					}
				}

				cs.Unlock()
			case <-cs.ctx.Done():
					cs.Debugf("stop sweeping udp connections (shutdown)")
					return
			}
		}
	}()
}
