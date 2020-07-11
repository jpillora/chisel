package tunnel

import (
	"encoding/gob"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpillora/chisel/share/cio"
)

func (t *Tunnel) handleUDP(l *cio.Logger, rwc io.ReadWriteCloser) error {
	h := &udpHandler{
		Logger: l,
		udpOutbound: &udpOutbound{
			r: gob.NewDecoder(rwc),
			w: gob.NewEncoder(rwc),
			c: rwc,
		},
		udpConns: &udpConns{
			m: map[string]*udpConn{},
		},
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
	*udpOutbound
	*udpConns
	sent, recv int64
}

func (h *udpHandler) handleWrite(p *udpPacket) error {
	if err := h.r.Decode(&p); err != nil {
		return err
	}
	//dial now, we know we must write
	conn, exists, err := h.udpConns.dial(p.Src, p.Dst)
	if err != nil {
		return err
	}
	if !exists {
		//however, we dont know if we must read...
		//TODO limit concurrent reads
		go h.handleRead(p, conn)
	}
	n, err := conn.Write(p.Payload)
	if err != nil {
		return err
	}
	//stats
	atomic.AddInt64(&h.sent, int64(n))
	return nil
}

func (h *udpHandler) handleRead(p *udpPacket, conn *udpConn) {
	//ensure connection is cleaned up
	defer h.udpConns.remove(conn.id)
	const maxMTU = 9012
	buff := make([]byte, maxMTU)
	for {
		//response must arrive within 15 seconds
		//TODO configurable
		const deadline = 15 * time.Second
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
		err = h.udpOutbound.encode(p.Src, p.Dst, b)
		if err != nil {
			h.Debugf("encode error: %s", err)
			return
		}
		//stats
		atomic.AddInt64(&h.recv, int64(n))
	}
}

type udpConns struct {
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
			Conn: c,
		}
		conn.Conn = c
		cs.m[id] = conn
	}
	return conn, ok, nil
}

func (cs *udpConns) remove(id string) {
	cs.Lock()
	delete(cs.m, id)
	cs.Unlock()
}

type udpConn struct {
	id string
	net.Conn
}
