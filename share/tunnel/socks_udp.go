package tunnel

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
)

type socksUDPFlow struct {
	conn     net.Conn
	lastSeen time.Time // protected by the handler mutex
}

type socksUDPHandler struct {
	stream   *socksUDPStream
	logger   *cio.Logger
	ctx      context.Context
	maxConns int
	deadline time.Duration
	mu       sync.Mutex
	flows    map[string]*socksUDPFlow
	workers  sync.WaitGroup
}

func (t *Tunnel) handleSocksUDP(ctx context.Context, logger *cio.Logger, stream io.ReadWriteCloser) error {
	size, conns, deadline := socksUDPLimits()
	ctx, cancel := context.WithCancel(ctx)
	h := &socksUDPHandler{
		stream: &socksUDPStream{ReadWriteCloser: stream, max: size},
		logger: logger, ctx: ctx, maxConns: conns, deadline: deadline,
		flows: make(map[string]*socksUDPFlow),
	}
	stop := context.AfterFunc(ctx, func() { stream.Close() })
	defer func() {
		cancel()
		stop()
		stream.Close()
		h.mu.Lock()
		for key, f := range h.flows {
			delete(h.flows, key)
			f.conn.Close()
		}
		h.mu.Unlock()
		h.workers.Wait()
	}()
	for {
		b, err := h.stream.readPacket()
		if err != nil {
			return err
		}
		addr, payload, err := parseSocksUDP(b)
		if err != nil {
			continue // malformed datagrams do not end a valid association
		}
		h.write(addr, payload)
	}
}

func (h *socksUDPHandler) write(addr socksAddress, payload []byte) {
	key := addr.destination()
	h.mu.Lock()
	f := h.flows[key]
	full := len(h.flows) >= h.maxConns
	h.mu.Unlock()
	if f == nil {
		if full {
			return // hard cap: no write-only sockets or unbounded reader state
		}
		d := net.Dialer{Timeout: settings.EnvDuration("DIAL_TIMEOUT", 30*time.Second)}
		conn, err := d.DialContext(h.ctx, "udp", key)
		if err != nil {
			h.logger.Debugf("SOCKS UDP dial %s: %s", key, err)
			return
		}
		f = &socksUDPFlow{conn: conn, lastSeen: time.Now()}
		f.conn.SetReadDeadline(f.lastSeen.Add(h.deadline))
		h.mu.Lock()
		h.flows[key] = f
		h.mu.Unlock()
		h.workers.Add(1)
		go h.readReplies(key, f)
	}
	// Refresh activity before writing, so a reader timeout cannot expire an
	// otherwise active send-only flow. UDP errors affect only this destination.
	h.mu.Lock()
	if h.flows[key] != f {
		h.mu.Unlock()
		return
	}
	f.lastSeen = time.Now()
	f.conn.SetReadDeadline(f.lastSeen.Add(h.deadline))
	h.mu.Unlock()
	f.conn.SetWriteDeadline(time.Now().Add(h.deadline))
	if _, err := f.conn.Write(payload); err != nil {
		h.logger.Debugf("SOCKS UDP write %s: %s", key, err)
		h.remove(key, f)
	}
}

func (h *socksUDPHandler) remove(key string, f *socksUDPFlow) {
	h.mu.Lock()
	if h.flows[key] == f {
		delete(h.flows, key)
	}
	h.mu.Unlock()
	f.conn.Close()
}

func (h *socksUDPHandler) readReplies(key string, f *socksUDPFlow) {
	defer h.workers.Done()
	defer h.remove(key, f)
	addr := f.conn.RemoteAddr().(*net.UDPAddr)
	header := append([]byte{0, 0, 0}, socksIPPort(addr.IP, addr.Port)...)
	buf := make([]byte, h.stream.max+1)
	for {
		n, err := f.conn.Read(buf)
		if socksUDPTooLarge(n, h.stream.max-len(header), err) {
			continue
		}
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Timeout() {
				h.mu.Lock()
				active := h.flows[key] == f && time.Since(f.lastSeen) < h.deadline
				if active {
					f.conn.SetReadDeadline(f.lastSeen.Add(h.deadline))
				} else if h.flows[key] == f {
					delete(h.flows, key)
				}
				h.mu.Unlock()
				if active {
					continue
				}
			}
			return
		}
		h.mu.Lock()
		f.lastSeen = time.Now()
		f.conn.SetReadDeadline(f.lastSeen.Add(h.deadline))
		h.mu.Unlock()
		packet := make([]byte, len(header)+n)
		copy(packet, header)
		copy(packet[len(header):], buf[:n])
		if err := h.stream.writePacket(packet); err != nil {
			h.stream.Close() // unblock the association's request reader
			return
		}
	}
}
