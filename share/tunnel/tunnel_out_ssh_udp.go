package tunnel

import (
	"encoding/gob"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
)

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
		maxMTU:   settings.EnvInt("UDP_MAX_SIZE", 9012),
		maxConns: settings.EnvInt("UDP_MAX_CONNS", 100),
		deadline: settings.EnvDuration("UDP_DEADLINE", 15*time.Second),
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
	maxMTU    int
	maxConns  int
	deadline  time.Duration
	lastSweep time.Time
}

func (h *udpHandler) handleWrite(p *udpPacket) error {
	if err := h.r.Decode(&p); err != nil {
		return err
	}
	//remove idle write-only connections,
	//making room for new flows to become readable again
	h.sweepWriteOnly()
	//dial now, we know we must write
	conn, exists, err := h.udpConns.dial(p.Src, h.hostPort)
	if err != nil {
		return err
	}
	//however, we dont know if we must read...
	//spawn up to <max-conns> (UDP_MAX_CONNS) go-routines
	//to wait for replies; flows beyond the cap are write-only
	//(replies are dropped) and are swept once idle
	if !exists {
		if h.udpConns.len() <= h.maxConns {
			go h.handleRead(p, conn)
		} else {
			conn.writeOnly = true
			h.Debugf("exceeded max udp connections (%d), flow %s is write-only", h.maxConns, p.Src)
		}
	}
	if _, err := conn.Write(p.Payload); err != nil {
		//write failures (broken flow, ICMP unreachable, conn
		//closed by its reader) drop the flow, not the channel
		h.Debugf("write error (%s): %s", p.Src, err)
		h.udpConns.remove(conn.id)
		conn.Close()
		return nil
	}
	if conn.writeOnly {
		conn.lastWrite = time.Now()
	}
	return nil
}

//sweepWriteOnly closes and removes write-only connections which have
//been idle for longer than the read deadline. it runs in the write
//goroutine (at most once per second) so it cannot race in-flight writes.
func (h *udpHandler) sweepWriteOnly() {
	now := time.Now()
	if now.Sub(h.lastSweep) < time.Second {
		return
	}
	h.lastSweep = now
	h.udpConns.sweepWriteOnly(now.Add(-h.deadline))
}

func (h *udpHandler) handleRead(p *udpPacket, conn *udpConn) {
	//ensure connection is cleaned up and closed
	defer func() {
		h.udpConns.remove(conn.id)
		conn.Close()
	}()
	buff := make([]byte, h.maxMTU)
	for {
		//response must arrive within the deadline
		conn.SetReadDeadline(time.Now().Add(h.deadline))
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

//sweepWriteOnly closes and removes write-only connections
//whose last write is older than the given time
func (cs *udpConns) sweepWriteOnly(olderThan time.Time) {
	cs.Lock()
	defer cs.Unlock()
	for id, conn := range cs.m {
		if conn.writeOnly && conn.lastWrite.Before(olderThan) {
			conn.Close()
			delete(cs.m, id)
		}
	}
}

type udpConn struct {
	id string
	net.Conn
	//write-only flows (over the conn cap) have no read goroutine;
	//these fields are only touched by the single write goroutine
	writeOnly bool
	lastWrite time.Time
}
