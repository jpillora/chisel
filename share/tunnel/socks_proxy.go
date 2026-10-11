package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

// socksProxy owns one UDP socket per TCP listener. Associations are isolated
// by observed UDP endpoints, never by addresses supplied by the exit node.
type socksProxy struct {
	proxy             *Proxy
	udp               *net.UDPConn
	maxSize, maxConns int
	mu                sync.Mutex
	associations      map[*socksAssociation]bool
	peers             map[netip.AddrPort]*socksAssociation
	pending           map[netip.Addr]*socksAssociation
	workers           sync.WaitGroup
}

type socksAssociation struct {
	ip      netip.Addr
	key     netip.AddrPort
	peer    *net.UDPAddr
	packets chan []byte
	stream  *socksUDPStream
}

func newSocksProxy(p *Proxy) *socksProxy {
	size, conns, _ := socksUDPLimits()
	s := &socksProxy{
		proxy: p, maxSize: size, maxConns: conns,
		associations: make(map[*socksAssociation]bool),
		peers:        make(map[netip.AddrPort]*socksAssociation),
		pending:      make(map[netip.Addr]*socksAssociation),
	}
	addr := p.tcp.Addr().(*net.TCPAddr)
	var err error
	s.udp, err = net.ListenUDP("udp", &net.UDPAddr{IP: addr.IP, Port: addr.Port, Zone: addr.Zone})
	if err != nil {
		p.Infof("SOCKS UDP unavailable on %s: %s (TCP remains available)", addr, err)
	} else {
		p.Infof("Listening for SOCKS UDP on %s", s.udp.LocalAddr())
	}
	return s
}

func (s *socksProxy) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(ctx, func() { s.proxy.Close() })
	var udpWorkers sync.WaitGroup
	udpError := make(chan error, 1)
	if s.udp != nil {
		udpWorkers.Add(1)
		go func() {
			defer udpWorkers.Done()
			if err := s.readUDP(); err != nil && ctx.Err() == nil {
				s.proxy.Infof("SOCKS UDP read error: %s", err)
				udpError <- err
				cancel()
			}
		}()
	}
	err := s.proxy.runTCP(ctx)
	cancel()
	stop()
	s.proxy.Close()
	udpWorkers.Wait()
	s.workers.Wait()
	if err == nil {
		select {
		case err = <-udpError:
		default:
		}
	}
	return err
}

func socksPeerIP(ip net.IP, zone string) netip.Addr {
	a, _ := netip.AddrFromSlice(ip)
	a = a.Unmap()
	if zone != "" && a.Is6() {
		a = a.WithZone(zone)
	}
	return a
}

func (s *socksProxy) readUDP() error {
	// The extra byte detects oversize packets even when ReadFromUDP truncates.
	buf := make([]byte, s.maxSize+1)
	for {
		n, peer, err := s.udp.ReadFromUDP(buf)
		if socksUDPTooLarge(n, s.maxSize, err) {
			continue
		}
		if err != nil {
			return err
		}
		if _, _, err := parseSocksUDP(buf[:n]); err != nil {
			continue
		}
		ip := socksPeerIP(peer.IP, peer.Zone)
		key := netip.AddrPortFrom(ip, uint16(peer.Port))
		s.mu.Lock()
		a := s.peers[key]
		if a == nil {
			a = s.pending[ip]
			if a != nil {
				delete(s.pending, ip)
				a.key = key
				s.peers[key] = a
			}
		}
		if a != nil {
			if a.peer == nil {
				// ReadFromUDP's address is local evidence; retain our own copy.
				copyPeer := *peer
				copyPeer.IP = append(net.IP(nil), peer.IP...)
				a.peer = &copyPeer
			}
			select {
			case a.packets <- append([]byte(nil), buf[:n]...):
			default: // a stalled association cannot block this shared listener
			}
		}
		s.mu.Unlock()
	}
}

func (s *socksProxy) register(a *socksAssociation, port uint16) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.associations) >= s.maxConns {
		return false
	}
	if port == 0 {
		if s.pending[a.ip] != nil {
			return false
		}
		s.pending[a.ip] = a
	} else {
		a.key = netip.AddrPortFrom(a.ip, port)
		if s.peers[a.key] != nil {
			return false
		}
		s.peers[a.key] = a
	}
	s.associations[a] = true
	return true
}

func (s *socksProxy) unregister(a *socksAssociation) {
	s.mu.Lock()
	delete(s.associations, a)
	if s.pending[a.ip] == a {
		delete(s.pending, a.ip)
	}
	if s.peers[a.key] == a {
		delete(s.peers, a.key)
	}
	s.mu.Unlock()
}

func (s *socksProxy) serve(ctx context.Context, src *net.TCPConn) {
	defer s.workers.Done()
	defer src.Close()
	stop := context.AfterFunc(ctx, func() { src.Close() })
	defer stop()
	// Bound unfinished handshakes without imposing a TCP control idle timeout.
	src.SetDeadline(time.Now().Add(settings.EnvDuration("DIAL_TIMEOUT", 30*time.Second)))
	if err := s.handle(ctx, src); err != nil && ctx.Err() == nil {
		s.proxy.Debugf("SOCKS connection: %s", err)
	}
}

func (s *socksProxy) handle(ctx context.Context, src *net.TCPConn) error {
	var greeting [2]byte
	if _, err := io.ReadFull(src, greeting[:]); err != nil {
		return err
	}
	if greeting[0] != 5 || greeting[1] == 0 {
		return errors.New("invalid SOCKS greeting")
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(src, methods); err != nil {
		return err
	}
	noAuth := false
	for _, method := range methods {
		noAuth = noAuth || method == 0
	}
	if !noAuth {
		writeSocksBytes(src, []byte{5, 255})
		return errors.New("no supported SOCKS authentication method")
	}
	if err := writeSocksBytes(src, []byte{5, 0}); err != nil {
		return err
	}
	var header [3]byte
	if _, err := io.ReadFull(src, header[:]); err != nil {
		return err
	}
	if header[0] != 5 || header[2] != 0 {
		socksReply(src, socksFailure, nil)
		return errors.New("invalid SOCKS request")
	}
	addr, err := readSocksAddress(src)
	if err != nil {
		code := byte(socksFailure)
		if errors.Is(err, errSocksAddress) {
			code = socksBadAddress
		}
		socksReply(src, code, nil)
		return err
	}
	if header[1] != 1 && header[1] != 3 {
		return socksReply(src, socksUnsupported, nil)
	}
	if header[1] == 3 && s.udp == nil {
		return socksReply(src, socksFailure, nil)
	}
	conn := s.proxy.sshTun.getSSH(ctx)
	if conn == nil {
		return socksReply(src, socksFailure, nil)
	}
	destination := "socks"
	if header[1] == 3 {
		destination = socksUDPDestination
	}
	ch, reqs, err := conn.OpenChannel("chisel", []byte(destination))
	if err != nil {
		code := byte(socksFailure)
		var channelErr *ssh.OpenChannelError
		if errors.As(err, &channelErr) && channelErr.Reason == ssh.Prohibited {
			code = socksDenied
		} else if header[1] == 3 && errors.As(err, &channelErr) && channelErr.Reason == ssh.ConnectionFailed {
			code = socksUnsupported
		}
		if header[1] == 3 {
			s.proxy.Infof("SOCKS UDP channel unavailable: %s (UDP requires support on both Chisel peers)", err)
		} else {
			s.proxy.Infof("SOCKS TCP channel unavailable: %s", err)
		}
		socksReply(src, code, nil)
		return err
	}
	defer ch.Close()
	go ssh.DiscardRequests(reqs)
	stop := context.AfterFunc(ctx, func() { ch.Close() })
	defer stop()
	if header[1] == 1 {
		// Replay a no-auth greeting on the unchanged legacy TCP SOCKS channel.
		if err := writeSocksBytes(ch, []byte{5, 1, 0}); err != nil {
			return err
		}
		var reply [2]byte
		if _, err := io.ReadFull(ch, reply[:]); err != nil {
			return err
		}
		if reply != [2]byte{5, 0} {
			return errors.New("unexpected exit SOCKS authentication reply")
		}
		if err := writeSocksBytes(ch, append(header[:], addr.wire...)); err != nil {
			return err
		}
		src.SetDeadline(time.Time{})
		cio.Pipe(src, ch)
		return nil
	}
	// An old exit may accept the channel then fail its TCP dial. It must
	// never cause a successful UDP reply, even briefly. Bound a silent exit.
	timer := time.AfterFunc(settings.EnvDuration("DIAL_TIMEOUT", 30*time.Second), func() { ch.Close() })
	ready := make([]byte, len(socksUDPReady))
	_, err = io.ReadFull(ch, ready)
	timedOut := !timer.Stop()
	if err != nil || timedOut || string(ready) != socksUDPReady {
		s.proxy.Infof("SOCKS UDP unavailable: exit did not confirm support (upgrade both Chisel peers)")
		return socksReply(src, socksUnsupported, nil)
	}
	return s.associate(ctx, src, ch, addr.port)
}

func (s *socksProxy) associate(ctx context.Context, src *net.TCPConn, ch ssh.Channel, port uint16) error {
	peer := src.RemoteAddr().(*net.TCPAddr)
	a := &socksAssociation{
		ip: socksPeerIP(peer.IP, peer.Zone), packets: make(chan []byte, 16),
		stream: &socksUDPStream{ReadWriteCloser: ch, max: s.maxSize},
	}
	if !s.register(a, port) {
		return socksReply(src, socksFailure, nil)
	}
	defer s.unregister(a)
	local := src.LocalAddr().(*net.TCPAddr)
	relay := s.udp.LocalAddr().(*net.UDPAddr)
	if err := socksReply(src, socksSuccess, socksIPPort(local.IP, relay.Port)); err != nil {
		return err
	}
	src.SetDeadline(time.Time{})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	finish := func() { cancel(); ch.Close(); src.Close() }
	// All workers unblock on control EOF, channel EOF, or proxy cancellation.
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer finish()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-a.packets:
				if err := a.stream.writePacket(b); err != nil {
					return
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		defer finish()
		for {
			b, err := a.stream.readPacket()
			if err != nil {
				return
			}
			if _, _, err := parseSocksUDP(b); err != nil {
				continue
			}
			s.mu.Lock()
			// Never resolve or route using the exit's packet address: it is
			// the responder encoded in the SOCKS header, not the local peer.
			if s.associations[a] && a.peer != nil && ctx.Err() == nil {
				s.udp.WriteToUDP(b, a.peer)
			}
			s.mu.Unlock()
		}
	}()
	io.Copy(io.Discard, src)
	finish()
	wg.Wait()
	return nil
}
