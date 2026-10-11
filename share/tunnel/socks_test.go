package tunnel

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/armon/go-socks5"
	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

type socksTestRWC struct {
	io.Reader
	io.Writer
}

func (s socksTestRWC) Close() error { return nil }

func TestSocksUDPCodec(t *testing.T) {
	addresses := [][]byte{
		socksIPPort(net.ParseIP("127.0.0.1"), 53),
		socksIPPort(net.ParseIP("::1"), 53),
		append([]byte{3, 9}, append([]byte("localhost"), 0, 53)...),
	}
	for _, address := range addresses {
		packet := append(append([]byte{0, 0, 0}, address...), []byte("data")...)
		a, payload, err := parseSocksUDP(packet)
		if err != nil || a.port != 53 || string(payload) != "data" || !bytes.Equal(a.wire, address) {
			t.Fatalf("decode %v: %v %q %v", packet, a, payload, err)
		}
		for n := 0; n < 3+len(address); n++ {
			if _, _, err := parseSocksUDP(packet[:n]); err == nil {
				t.Fatalf("accepted truncated packet of length %d", n)
			}
		}
	}
	for _, b := range [][]byte{
		{0, 0, 0, 2}, {0, 0, 0, 3, 0, 0, 53},
		{0, 0, 1, 1, 127, 0, 0, 1, 0, 53},
		{1, 0, 0, 1, 127, 0, 0, 1, 0, 53},
		{0, 0, 0, 1, 127, 0, 0, 1, 0, 0},
	} {
		if _, _, err := parseSocksUDP(b); err == nil {
			t.Fatalf("accepted malformed packet %v", b)
		}
	}
}

func FuzzSocksUDPCodec(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 127, 0, 0, 1, 0, 53, 'x'})
	f.Add([]byte{0, 0, 0, 3, 1, 'a', 0, 53})
	f.Fuzz(func(t *testing.T, b []byte) {
		a, payload, err := parseSocksUDP(b)
		if err == nil && (a.port == 0 || len(a.wire)+3+len(payload) != len(b)) {
			t.Fatal("invalid decoded packet")
		}
	})
}

func TestSocksUDPFrames(t *testing.T) {
	// A tiny packet limit can be smaller than an IPv6 reply header. Even
	// then, closed sockets and read deadlines must terminate the reader.
	if socksUDPTooLarge(0, -1, net.ErrClosed) {
		t.Fatal("small packet limit hid a closed socket")
	}
	for _, size := range []uint32{0, 65, 0xffffffff} {
		buf := binary.BigEndian.AppendUint32(nil, size)
		s := &socksUDPStream{ReadWriteCloser: socksTestRWC{Reader: bytes.NewReader(buf)}, max: 64}
		if _, err := s.readPacket(); err == nil {
			t.Fatalf("accepted frame size %d", size)
		}
	}
	var buf bytes.Buffer
	s := &socksUDPStream{ReadWriteCloser: socksTestRWC{Reader: &buf, Writer: &buf}, max: 64}
	if err := s.writePacket([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := s.writePacket([]byte("two")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"one", "two"} {
		b, err := s.readPacket()
		if err != nil || string(b) != want {
			t.Fatalf("read %q, want %q: %v", b, want, err)
		}
	}
	if err := s.writePacket(make([]byte, 65)); err == nil {
		t.Fatal("wrote oversized frame")
	}
}

func TestSocksUDPLimitSettings(t *testing.T) {
	for _, test := range []struct {
		name, size, conns, idle string
		wantSize, wantConns     int
		wantIdle                time.Duration
	}{
		{"defaults", "", "", "", 9012, 100, 15 * time.Second},
		{"configured", "64", "2", "250ms", 64, 2, 250 * time.Millisecond},
		{"zero", "0", "0", "0s", 9012, 100, 15 * time.Second},
		{"negative", "-1", "-1", "-1s", 9012, 100, 15 * time.Second},
		{"invalid", "invalid", "invalid", "invalid", 9012, 100, 15 * time.Second},
		{"size cap", "70000", "2", "1s", 65507, 2, time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CHISEL_UDP_MAX_SIZE", test.size)
			t.Setenv("CHISEL_UDP_MAX_CONNS", test.conns)
			t.Setenv("CHISEL_UDP_DEADLINE", test.idle)
			size, conns, idle := socksUDPLimits()
			if size != test.wantSize || conns != test.wantConns || idle != test.wantIdle {
				t.Fatalf("limits=(%d,%d,%s), want (%d,%d,%s)", size, conns, idle, test.wantSize, test.wantConns, test.wantIdle)
			}
		})
	}
}

func FuzzSocksUDPFrames(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 'x'})
	f.Add([]byte{255, 255, 255, 255})
	f.Fuzz(func(t *testing.T, b []byte) {
		s := &socksUDPStream{ReadWriteCloser: socksTestRWC{Reader: bytes.NewReader(b)}, max: 9012}
		packet, err := s.readPacket()
		if err == nil && (len(packet) == 0 || len(packet) > 9012 || len(packet)+4 > len(b)) {
			t.Fatal("invalid frame decoded")
		}
	})
}

func TestSocksUDPSharedRoutingAndOverflow(t *testing.T) {
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	s := &socksProxy{udp: u, maxSize: 100, maxConns: 2,
		associations: make(map[*socksAssociation]bool), peers: make(map[netip.AddrPort]*socksAssociation), pending: make(map[netip.Addr]*socksAssociation)}
	ip := netip.MustParseAddr("127.0.0.1")
	a := &socksAssociation{ip: ip, packets: make(chan []byte, 16)}
	b := &socksAssociation{ip: ip, packets: make(chan []byte, 16)}
	if !s.register(a, 0) || s.register(b, 0) {
		t.Fatal("pending association policy failed")
	}
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	readDone := make(chan error, 1)
	go func() { readDone <- s.readUDP() }()
	packet := append([]byte{0, 0, 0}, socksIPPort(net.IPv4(127, 0, 0, 1), 53)...)
	t.Run("different source IP", func(t *testing.T) {
		otherIP, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 2)})
		if err != nil {
			t.Skipf("alternate loopback unavailable: %v", err)
		}
		defer otherIP.Close()
		otherIP.WriteToUDP(packet, u.LocalAddr().(*net.UDPAddr))
		select {
		case <-a.packets:
			t.Fatal("different IP claimed a pending association")
		case <-time.After(20 * time.Millisecond):
		}
	})
	peer.WriteToUDP(packet, u.LocalAddr().(*net.UDPAddr))
	select {
	case <-a.packets:
	case <-time.After(time.Second):
		t.Fatal("first packet did not pin")
	}
	if !s.register(b, 0) {
		t.Fatal("pinned association still blocks pending association")
	}
	if s.register(&socksAssociation{ip: ip}, 12345) {
		t.Fatal("association limit bypassed")
	}
	// Fill A's queue, then send enough packets to overflow it. B must still
	// receive its own packet, proving the shared reader does not block on A.
	for i := 0; i < cap(a.packets); i++ {
		a.packets <- packet
	}
	for i := 0; i < 20; i++ {
		peer.WriteToUDP(packet, u.LocalAddr().(*net.UDPAddr))
	}
	other, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	other.WriteToUDP(packet, u.LocalAddr().(*net.UDPAddr))
	select {
	case <-b.packets:
	case <-time.After(time.Second):
		t.Fatal("full queue blocked unrelated peer")
	}
	s.unregister(a)
	s.unregister(b)
	if !s.register(a, uint16(peer.LocalAddr().(*net.UDPAddr).Port)) {
		t.Fatal("closed association retained reservation")
	}
	u.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("UDP reader did not stop")
	}
}

type socksTestSSH struct {
	*mockSSHConn
	open func(string, []byte) (ssh.Channel, <-chan *ssh.Request, error)
}

func (s *socksTestSSH) OpenChannel(kind string, extra []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return s.open(kind, extra)
}

type socksTestTunnel struct{ conn ssh.Conn }

func (s socksTestTunnel) getSSH(context.Context) ssh.Conn { return s.conn }

type socksTestChannel struct{ net.Conn }

func (s socksTestChannel) CloseWrite() error                              { return s.Close() }
func (s socksTestChannel) SendRequest(string, bool, []byte) (bool, error) { return false, nil }
func (s socksTestChannel) Stderr() io.ReadWriter                          { return s.Conn }

func newSocksTestProxy(t *testing.T, conn ssh.Conn) *Proxy {
	t.Helper()
	// Reserve a port in the test; udpsocks itself must never bind port zero.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	p, err := NewProxy(cio.NewLogger("test"), socksTestTunnel{conn}, 0, &settings.Remote{LocalHost: "127.0.0.1", LocalPort: port, LocalProto: "tcp", Socks: true, UDPSocks: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("SOCKS proxy did not stop")
		}
	})
	return p
}

func TestUDPSocksRejectsEphemeralListener(t *testing.T) {
	r := &settings.Remote{LocalHost: "127.0.0.1", LocalPort: "0", LocalProto: "tcp", Socks: true, UDPSocks: true}
	p, err := NewProxy(cio.NewLogger("test"), socksTestTunnel{}, 0, r)
	if p != nil {
		defer p.Close()
	}
	if err == nil {
		t.Fatal("udpsocks accepted an ephemeral listening port")
	}
}

func TestSocksLegacyPeerCompatibility(t *testing.T) {
	legacy, err := socks5.New(&socks5.Config{Logger: nil})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	conn := &socksTestSSH{mockSSHConn: newMockSSHConn(false)}
	conn.open = func(_ string, extra []byte) (ssh.Channel, <-chan *ssh.Request, error) {
		if string(extra) != "socks" {
			// Old Chisel treats unknown destinations as TCP host:port and
			// rejects this token because it lacks a port.
			return nil, nil, &ssh.OpenChannelError{Reason: ssh.ConnectionFailed, Message: "missing port in address"}
		}
		local, remote := net.Pipe()
		wg.Add(1)
		go func() { defer wg.Done(); legacy.ServeConn(remote) }()
		reqs := make(chan *ssh.Request)
		close(reqs)
		return socksTestChannel{local}, reqs, nil
	}
	p := newSocksTestProxy(t, conn)
	endpoint, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	go func() {
		c, err := endpoint.Accept()
		if err == nil {
			c.Write([]byte("legacy TCP"))
			c.Close()
		}
	}()
	for _, command := range []byte{1, 3} {
		c, err := net.Dial("tcp", p.tcp.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(time.Second))
		c.Write([]byte{5, 1, 0})
		greeting := make([]byte, 2)
		if _, err := io.ReadFull(c, greeting); err != nil {
			t.Fatal(err)
		}
		a := endpoint.Addr().(*net.TCPAddr)
		c.Write(append([]byte{5, command, 0}, socksIPPort(a.IP, a.Port)...))
		header := make([]byte, 3)
		if _, err := io.ReadFull(c, header); err != nil {
			t.Fatal(err)
		}
		if _, err := readSocksAddress(c); err != nil {
			t.Fatal(err)
		}
		if command == 1 {
			b := make([]byte, len("legacy TCP"))
			if _, err := io.ReadFull(c, b); err != nil || string(b) != "legacy TCP" || header[1] != socksSuccess {
				t.Fatalf("legacy TCP failed: %q %v %v", b, header, err)
			}
		} else if header[1] != socksUnsupported {
			t.Fatalf("legacy UDP reply %d, want unsupported", header[1])
		}
		c.Close()
	}
	wg.Wait()
}

func TestSocksLegacyPeerAcceptsBeforeDial(t *testing.T) {
	conn := &socksTestSSH{mockSSHConn: newMockSSHConn(false)}
	conn.open = func(_ string, _ []byte) (ssh.Channel, <-chan *ssh.Request, error) {
		local, remote := net.Pipe()
		// Pre-1.12 Chisel accepted channels before dialing. An unknown
		// destination then failed the TCP dial and closed the accepted stream.
		remote.Close()
		reqs := make(chan *ssh.Request)
		close(reqs)
		return socksTestChannel{local}, reqs, nil
	}
	p := newSocksTestProxy(t, conn)
	c, err := net.Dial("tcp", p.tcp.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	c.Write([]byte{5, 1, 0})
	var greeting [2]byte
	if _, err := io.ReadFull(c, greeting[:]); err != nil {
		t.Fatal(err)
	}
	c.Write(append([]byte{5, 3, 0}, socksIPPort(net.IPv4zero, 0)...))
	var reply [3]byte
	if _, err := io.ReadFull(c, reply[:]); err != nil || reply[1] != socksUnsupported {
		t.Fatalf("old peer produced reply %v: %v", reply, err)
	}
	p.socks.mu.Lock()
	defer p.socks.mu.Unlock()
	if len(p.socks.associations) != 0 {
		t.Fatal("unsupported peer retained an association")
	}
}

func TestSocksUDPExitLimitsAndRecovery(t *testing.T) {
	t.Setenv("CHISEL_UDP_MAX_CONNS", "1")
	t.Setenv("CHISEL_UDP_DEADLINE", "50ms")
	echo := func() *net.UDPConn {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		go func() {
			b := make([]byte, 100)
			for {
				n, a, err := c.ReadFromUDP(b)
				if err != nil {
					return
				}
				c.WriteToUDP(b[:n], a)
			}
		}()
		return c
	}
	one, two := echo(), echo()
	client, server := net.Pipe()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tun := New(Config{Logger: cio.NewLogger("test")})
	done := make(chan error, 1)
	go func() { done <- tun.handleSocksUDP(ctx, tun.Logger, server) }()
	s := &socksUDPStream{ReadWriteCloser: client, max: 9012}
	packet := func(c *net.UDPConn) []byte {
		a := c.LocalAddr().(*net.UDPAddr)
		return append(append([]byte{0, 0, 0}, socksIPPort(a.IP, a.Port)...), 'x')
	}
	client.SetDeadline(time.Now().Add(time.Second))
	if err := s.writePacket(packet(one)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.readPacket(); err != nil {
		t.Fatal(err)
	}
	if err := s.writePacket(packet(two)); err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, err := s.readPacket(); err == nil {
		t.Fatal("flow cap did not drop new destination")
	}
	// The flow cap must recover once the existing destination goes idle.
	time.Sleep(100 * time.Millisecond)
	client.SetDeadline(time.Now().Add(time.Second))
	if err := s.writePacket(packet(two)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.readPacket(); err != nil {
		t.Fatalf("idle flow retained capacity: %v", err)
	}
	// Close immediately while a reply reader exists: cancellation must join it.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("exit retained workers on cancellation")
	}
}

func TestSocksUDPReturnsOnlyToObservedAssociation(t *testing.T) {
	conn := &socksTestSSH{mockSSHConn: newMockSSHConn(false)}
	remoteChannels := make(chan net.Conn, 1)
	conn.open = func(_ string, extra []byte) (ssh.Channel, <-chan *ssh.Request, error) {
		if string(extra) != socksUDPDestination {
			t.Errorf("unexpected destination %q", extra)
		}
		local, remote := net.Pipe()
		remoteChannels <- remote
		go writeSocksBytes(remote, []byte(socksUDPReady))
		reqs := make(chan *ssh.Request)
		close(reqs)
		return socksTestChannel{local}, reqs, nil
	}
	p := newSocksTestProxy(t, conn)
	udpClient := func() *net.UDPConn {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	allowed, blocked := udpClient(), udpClient()
	control, err := net.Dial("tcp", p.tcp.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	control.SetDeadline(time.Now().Add(time.Second))
	control.Write([]byte{5, 1, 0})
	var greeting [2]byte
	if _, err := io.ReadFull(control, greeting[:]); err != nil {
		t.Fatal(err)
	}
	port := allowed.LocalAddr().(*net.UDPAddr).Port
	control.Write(append([]byte{5, 3, 0}, socksIPPort(net.IPv4zero, port)...))
	var reply [3]byte
	if _, err := io.ReadFull(control, reply[:]); err != nil || reply[1] != 0 {
		t.Fatalf("association failed: %v %v", reply, err)
	}
	if _, err := readSocksAddress(control); err != nil {
		t.Fatal(err)
	}
	remote := <-remoteChannels
	defer remote.Close()
	remote.SetDeadline(time.Now().Add(time.Second))
	s := &socksUDPStream{ReadWriteCloser: remote, max: 9012}
	blockedAddr := blocked.LocalAddr().(*net.UDPAddr)
	packet := append(append([]byte{0, 0, 0}, socksIPPort(blockedAddr.IP, blockedAddr.Port)...), []byte("response")...)
	// A declared port alone is insufficient evidence for returning a packet.
	if err := s.writePacket(packet); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*net.UDPConn{allowed, blocked} {
		c.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		if _, _, err := c.ReadFromUDP(make([]byte, 100)); err == nil {
			t.Fatal("unobserved endpoint received a return packet")
		}
	}
	allowed.WriteToUDP(packet, p.socks.udp.LocalAddr().(*net.UDPAddr))
	if _, err := s.readPacket(); err != nil {
		t.Fatal(err)
	}
	if err := s.writePacket(packet); err != nil {
		t.Fatal(err)
	}
	allowed.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 100)
	n, _, err := allowed.ReadFromUDP(buf)
	if err != nil || !bytes.Equal(buf[:n], packet) {
		t.Fatalf("observed endpoint did not receive reply: %v", err)
	}
	blocked.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, _, err := blocked.ReadFromUDP(buf); err == nil {
		t.Fatal("exit-controlled address received a return packet")
	}
	control.Close()
	if _, err := remote.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("control EOF did not close SSH channel: %v", err)
	}
}

func TestSocksUDPInvalidFrameClosesAssociation(t *testing.T) {
	for _, size := range []uint32{0, 9013, 0xffffffff} {
		client, server := net.Pipe()
		tun := New(Config{Logger: cio.NewLogger("test")})
		done := make(chan error, 1)
		go func() { done <- tun.handleSocksUDP(context.Background(), tun.Logger, server) }()
		client.SetDeadline(time.Now().Add(time.Second))
		if _, err := client.Write(binary.BigEndian.AppendUint32(nil, size)); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("invalid frame succeeded")
			}
		case <-time.After(time.Second):
			t.Fatal("invalid frame retained association")
		}
		client.Close()
	}
}
