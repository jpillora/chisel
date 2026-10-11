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
	addr     *net.UDPAddr
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
	sockets  map[string]*net.UDPConn // one unconnected socket per address family
	workers  sync.WaitGroup
}

func (t *Tunnel) handleSocksUDP(ctx context.Context, logger *cio.Logger, stream io.ReadWriteCloser) error {
	size, conns, deadline := socksUDPLimits()
	ctx, cancel := context.WithCancel(ctx)
	h := &socksUDPHandler{
		stream: &socksUDPStream{ReadWriteCloser: stream, max: size},
		logger: logger, ctx: ctx, maxConns: conns, deadline: deadline,
		flows: make(map[string]*socksUDPFlow), sockets: make(map[string]*net.UDPConn),
	}
	stop := context.AfterFunc(ctx, func() { stream.Close() })
	defer func() {
		cancel()
		stop()
		stream.Close()
		h.mu.Lock()
		for _, conn := range h.sockets {
			conn.Close()
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

// Expiration bounds destination state without closing the association's
// sockets: protocols such as TFTP must retain their source port even when
// the remote endpoint changes. Caller holds h.mu.
func (h *socksUDPHandler) expire(now time.Time) {
	for key, f := range h.flows {
		if now.Sub(f.lastSeen) >= h.deadline {
			delete(h.flows, key)
		}
	}
}

func (h *socksUDPHandler) resolve(addr socksAddress) (*net.UDPAddr, error) {
	ip := net.ParseIP(addr.host)
	zone := ""
	if ip == nil {
		ctx, cancel := context.WithTimeout(h.ctx, settings.EnvDuration("DIAL_TIMEOUT", 30*time.Second))
		defer cancel()
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, addr.host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, &net.DNSError{Err: "no addresses", Name: addr.host, IsNotFound: true}
		}
		ip, zone = ips[0].IP, ips[0].Zone
	}
	return &net.UDPAddr{IP: ip, Port: int(addr.port), Zone: zone}, nil
}

func (h *socksUDPHandler) write(addr socksAddress, payload []byte) {
	key := addr.destination()
	h.mu.Lock()
	h.expire(time.Now())
	f := h.flows[key]
	full := len(h.flows) >= h.maxConns
	h.mu.Unlock()
	if f == nil {
		if full {
			return // hard cap: no unbounded destination state
		}
		resolved, err := h.resolve(addr)
		if err != nil {
			h.logger.Debugf("SOCKS UDP resolve %s: %s", key, err)
			return
		}
		f = &socksUDPFlow{addr: resolved}
	}
	network := "udp6"
	if f.addr.IP.To4() != nil {
		network = "udp4"
	}
	h.mu.Lock()
	conn := h.sockets[network]
	if conn == nil {
		var err error
		conn, err = net.ListenUDP(network, nil)
		if err != nil {
			h.mu.Unlock()
			h.logger.Debugf("SOCKS UDP listen %s: %s", network, err)
			return
		}
		h.sockets[network] = conn
		h.workers.Add(1)
		go h.readReplies(network, conn)
	}
	f.lastSeen = time.Now()
	h.flows[key] = f
	h.mu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(h.deadline))
	if _, err := conn.WriteToUDP(payload, f.addr); err != nil {
		h.logger.Debugf("SOCKS UDP write %s: %s", key, err)
		h.mu.Lock()
		delete(h.flows, key)
		h.mu.Unlock()
	}
}

// Prefer an exact endpoint match when refreshing destination activity. For
// replies from a new port, refresh the latest request to that host without
// allocating additional destination state. Replies from other hosts are also
// relayed but do not refresh destination state. Caller holds h.mu.
func (h *socksUDPHandler) replyFlow(addr *net.UDPAddr) *socksUDPFlow {
	var latest *socksUDPFlow
	for _, f := range h.flows {
		if !f.addr.IP.Equal(addr.IP) || f.addr.Zone != addr.Zone {
			continue
		}
		if f.addr.Port == addr.Port {
			return f
		}
		if latest == nil || f.lastSeen.After(latest.lastSeen) {
			latest = f
		}
	}
	return latest
}

func (h *socksUDPHandler) readReplies(network string, conn *net.UDPConn) {
	defer h.workers.Done()
	defer func() {
		h.mu.Lock()
		if h.sockets[network] == conn {
			delete(h.sockets, network)
		}
		h.mu.Unlock()
		conn.Close()
	}()
	buf := make([]byte, h.stream.max+1)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if socksUDPTooLarge(n, h.stream.max, err) {
			continue
		}
		if err != nil {
			return
		}
		header := append([]byte{0, 0, 0}, socksIPPort(addr.IP, addr.Port)...)
		if n > h.stream.max-len(header) {
			continue
		}
		h.mu.Lock()
		now := time.Now()
		h.expire(now)
		f := h.replyFlow(addr)
		if f != nil {
			f.lastSeen = now
		}
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
