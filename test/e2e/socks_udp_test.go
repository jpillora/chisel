package e2e_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

func socksWireAddress(host string, port int) []byte {
	var b []byte
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			b = append([]byte{1}, v4...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		b = append([]byte{3, byte(len(host))}, []byte(host)...)
	}
	return binary.BigEndian.AppendUint16(b, uint16(port))
}

func readSocksWireAddress(r io.Reader) (string, int, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return "", 0, err
	}
	n := 0
	switch atyp[0] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		if _, err := io.ReadFull(r, atyp[:]); err != nil {
			return "", 0, err
		}
		n = int(atyp[0])
	default:
		return "", 0, fmt.Errorf("unexpected SOCKS address type %d", atyp[0])
	}
	b := make([]byte, n+2)
	_, err := io.ReadFull(r, b)
	return net.IP(b[:n]).String(), int(binary.BigEndian.Uint16(b[n:])), err
}

func socksUDPControl(t *testing.T, addr string, sourcePort int) (net.Conn, *net.UDPAddr, byte) {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	var greeting [2]byte
	if _, err := io.ReadFull(c, greeting[:]); err != nil || greeting != [2]byte{5, 0} {
		t.Fatalf("SOCKS greeting %v: %v", greeting, err)
	}
	if _, err := c.Write(append([]byte{5, 3, 0}, socksWireAddress("0.0.0.0", sourcePort)...)); err != nil {
		t.Fatal(err)
	}
	var reply [3]byte
	if _, err := io.ReadFull(c, reply[:]); err != nil {
		t.Fatal(err)
	}
	host, port, err := readSocksWireAddress(c)
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Time{})
	return c, &net.UDPAddr{IP: net.ParseIP(host), Port: port}, reply[1]
}

func socksUDPClient(t *testing.T, host string) *net.UDPConn {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// Reserve UDP on a TCP-available port. Choosing a random UDP port can collide
// with an existing TCP socket and make a bind-failure test fail before it
// reaches the UDP behavior it is intended to exercise.
func socksUDPBlocker(t *testing.T) *net.UDPConn {
	t.Helper()
	for range 10 {
		addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:"+availablePort())
		if err != nil {
			t.Fatal(err)
		}
		c, err := net.ListenUDP("udp4", addr)
		if err != nil {
			continue
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	t.Fatal("cannot reserve UDP on a TCP-available port")
	return nil
}

func socksUDPEcho(t testing.TB, network, addr string) *net.UDPAddr {
	t.Helper()
	a, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.ListenUDP(network, a)
	if err != nil {
		if network == "udp6" {
			t.Skipf("IPv6 unavailable: %s", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, peer, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			c.WriteToUDP(buf[:n], peer)
		}
	}()
	return c.LocalAddr().(*net.UDPAddr)
}

func socksUDPRoundTrip(t *testing.T, c *net.UDPConn, relay *net.UDPAddr, host string, port int, payload string) {
	t.Helper()
	packet := append(append([]byte{0, 0, 0}, socksWireAddress(host, port)...), []byte(payload)...)
	if _, err := c.WriteToUDP(packet, relay); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 65535)
	n, from, err := c.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if from.Port != relay.Port || len(buf[:n]) < 3 || !bytes.Equal(buf[:3], []byte{0, 0, 0}) {
		t.Fatalf("invalid UDP relay response: %v from %s", buf[:n], from)
	}
	r := bytes.NewReader(buf[3:n])
	_, responsePort, err := readSocksWireAddress(r)
	if err != nil || responsePort != port {
		t.Fatalf("responder port %d, want %d: %v", responsePort, port, err)
	}
	b, _ := io.ReadAll(r)
	if string(b) != payload {
		t.Fatalf("UDP response %q, want %q", b, payload)
	}
}

func setupSocksUDP(t testing.TB, reverse bool, port string) (string, *chclient.Client) {
	t.Helper()
	return setupSocksUDPOn(t, reverse, "127.0.0.1", port)
}

func setupSocksUDPOn(t testing.TB, reverse bool, host, port string) (string, *chclient.Client) {
	t.Helper()
	return setupSocksRemoteOn(t, reverse, host, port, "udpsocks")
}

func setupSocksRemoteOn(t testing.TB, reverse bool, host, port, remoteType string) (string, *chclient.Client) {
	t.Helper()
	remote := net.JoinHostPort(host, port) + ":" + remoteType
	if reverse {
		remote = "R:" + remote
	}
	layout := testLayout{
		server: &chserver.Config{Socks5: !reverse, Reverse: reverse},
		client: &chclient.Config{Remotes: []string{remote}},
	}
	_, c, teardown := layout.setup(t)
	t.Cleanup(teardown)
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), c
}

func TestSocksTCPOnly(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, occupied := range []bool{false, true} {
			t.Run(fmt.Sprintf("reverse=%t/udpOccupied=%t", reverse, occupied), func(t *testing.T) {
				port := availablePort()
				if occupied {
					blocker := socksUDPBlocker(t)
					port = strconv.Itoa(blocker.LocalAddr().(*net.UDPAddr).Port)
				}
				addr, _ := setupSocksRemoteOn(t, reverse, "127.0.0.1", port, "socks")
				if !occupied {
					// A plain SOCKS proxy must leave this UDP port available.
					u, err := net.ListenPacket("udp", addr)
					if err != nil {
						t.Fatalf("socks bound UDP: %v", err)
					}
					t.Cleanup(func() { u.Close() })
				}
				_, _, code := socksUDPControl(t, addr, 0)
				if code != 7 {
					t.Fatalf("plain socks UDP ASSOCIATE code=%d, want unsupported (7)", code)
				}
				socksTCPRoundTrip(t, addr)
			})
		}
	}
}

func socksTCPRoundTrip(t *testing.T, addr string) {
	t.Helper()
	endpoint, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	go func() {
		c, err := endpoint.Accept()
		if err == nil {
			c.Write([]byte("TCP"))
			c.Close()
		}
	}()
	dialer, _ := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	c, err := dialer.Dial("tcp", endpoint.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	b, err := io.ReadAll(c)
	if err != nil || string(b) != "TCP" {
		t.Fatalf("TCP response %q: %v", b, err)
	}
}

func TestSocksUDP(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			addr, _ := setupSocksUDP(t, reverse, availablePort())
			udp := socksUDPClient(t, "127.0.0.1")
			_, relay, code := socksUDPControl(t, addr, 0)
			if code != 0 || relay.String() != addr {
				t.Fatalf("UDP ASSOCIATE code=%d relay=%s, want %s", code, relay, addr)
			}
			v4 := socksUDPEcho(t, "udp4", "127.0.0.1:0")
			socksUDPRoundTrip(t, udp, relay, v4.IP.String(), v4.Port, "IPv4")
			// A dual-stack endpoint permits whichever localhost address the
			// exit resolver selects, without needing external DNS/networking.
			domain := socksUDPEcho(t, "udp", ":0")
			socksUDPRoundTrip(t, udp, relay, "localhost", domain.Port, "domain")
			t.Run("IPv6", func(t *testing.T) {
				v6 := socksUDPEcho(t, "udp6", "[::1]:0")
				socksUDPRoundTrip(t, udp, relay, "::1", v6.Port, "IPv6")
			})
			// TCP and UDP share the SOCKS port and work concurrently.
			endpoint, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer endpoint.Close()
			go func() {
				c, err := endpoint.Accept()
				if err == nil {
					c.Write([]byte("TCP"))
					c.Close()
				}
			}()
			dialer, _ := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
			c, err := dialer.Dial("tcp", endpoint.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(time.Second))
			b, err := io.ReadAll(c)
			if err != nil || string(b) != "TCP" {
				t.Fatalf("TCP response %q: %v", b, err)
			}
			socksUDPRoundTrip(t, udp, relay, v4.IP.String(), v4.Port, "still UDP")
		})
	}
}

func TestSocksUDPAssociations(t *testing.T) {
	addr, _ := setupSocksUDP(t, false, availablePort())
	echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
	u1, u2 := socksUDPClient(t, "127.0.0.1"), socksUDPClient(t, "127.0.0.1")
	c1, relay, code := socksUDPControl(t, addr, 0)
	if code != 0 {
		t.Fatalf("first pending association rejected: %d", code)
	}
	_, _, code = socksUDPControl(t, addr, 0)
	if code != 1 {
		t.Fatalf("ambiguous pending association accepted: %d", code)
	}
	socksUDPRoundTrip(t, u1, relay, echo.IP.String(), echo.Port, "one")
	c2, relay2, code := socksUDPControl(t, addr, 0)
	if code != 0 {
		t.Fatalf("second association rejected after pinning: %d", code)
	}
	if relay.String() != addr || relay2.String() != addr {
		t.Fatalf("associations must share configured UDP port %s, got %s and %s", addr, relay, relay2)
	}
	socksUDPRoundTrip(t, u2, relay, echo.IP.String(), echo.Port, "two")
	socksUDPRoundTrip(t, u1, relay, echo.IP.String(), echo.Port, "one again")
	_, _, code = socksUDPControl(t, addr, u1.LocalAddr().(*net.UDPAddr).Port)
	if code != 1 {
		t.Fatalf("conflicting reservation accepted: %d", code)
	}
	c1.Close()
	// Wait for the source-port reservation to be released by control EOF.
	deadline := time.Now().Add(2 * time.Second)
	for {
		c, _, result := socksUDPControl(t, addr, u1.LocalAddr().(*net.UDPAddr).Port)
		if result == 0 {
			c.Close()
			break
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("closed control retained its reservation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	socksUDPRoundTrip(t, u2, relay, echo.IP.String(), echo.Port, "surviving association")
	c2.Close()
}

func TestSocksUDPRejectsUnassociatedAndMalformedPackets(t *testing.T) {
	addr, _ := setupSocksUDP(t, false, availablePort())
	echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
	allowed, blocked := socksUDPClient(t, "127.0.0.1"), socksUDPClient(t, "127.0.0.1")
	_, relay, code := socksUDPControl(t, addr, allowed.LocalAddr().(*net.UDPAddr).Port)
	if code != 0 {
		t.Fatalf("declared source port rejected: %d", code)
	}
	packet := append(append([]byte{0, 0, 0}, socksWireAddress(echo.IP.String(), echo.Port)...), 'x')
	if _, err := blocked.WriteToUDP(packet, relay); err != nil {
		t.Fatal(err)
	}
	blocked.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := blocked.ReadFromUDP(make([]byte, 100)); err == nil {
		t.Fatal("unassociated source received a response")
	}
	for _, invalid := range [][]byte{{0}, {0, 0, 0, 2}, append([]byte{0, 0, 1}, packet[3:]...), append([]byte{1, 0, 0}, packet[3:]...), make([]byte, 10000)} {
		allowed.WriteToUDP(invalid, relay)
	}
	allowed.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := allowed.ReadFromUDP(make([]byte, 11000)); err == nil {
		t.Fatal("malformed packet received a response")
	}
	socksUDPRoundTrip(t, allowed, relay, echo.IP.String(), echo.Port, "valid after invalid")
}

func TestSocksUDPBindFailureKeepsTCP(t *testing.T) {
	blocker := socksUDPBlocker(t)
	port := strconv.Itoa(blocker.LocalAddr().(*net.UDPAddr).Port)
	addr, _ := setupSocksUDP(t, false, port)
	_, _, code := socksUDPControl(t, addr, 0)
	if code != 1 {
		t.Fatalf("UDP bind failure reply %d, want 1", code)
	}
	// The greeting and CONNECT must still reach the exit's existing TCP engine.
	endpoint, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	go func() {
		c, err := endpoint.Accept()
		if err == nil {
			c.Close()
		}
	}()
	dialer, _ := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	c, err := dialer.Dial("tcp", endpoint.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestSocksUDPDisconnectClosesControl(t *testing.T) {
	addr, client := setupSocksUDP(t, false, availablePort())
	control, _, code := socksUDPControl(t, addr, 0)
	if code != 0 {
		t.Fatal(code)
	}
	client.Close()
	control.SetReadDeadline(time.Now().Add(time.Second))
	_, err := control.Read(make([]byte, 1))
	if err != io.EOF {
		t.Fatalf("control did not close on shutdown: %v", err)
	}
	// Both ports must be released after the proxy has stopped.
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("TCP port retained: %v", err)
	}
	tcp.Close()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("UDP port retained: %v", err)
	}
	udp.Close()
}

func TestSocksUDPChannelACL(t *testing.T) {
	for _, grant := range []string{`^socks$`, `^127\.0\.0\.1:80$`} {
		t.Run(grant, func(t *testing.T) {
			addr := startSocksACLServer(t, "socks-udp-acl", grant)
			sc, _, _ := dialChiselSSH(t, addr, "user", "pass")
			defer sc.Close()
			sendConfig(t, sc, nil)
			ch, reqs, err := sc.OpenChannel("chisel", []byte("socks-udp-v1"))
			if grant != `^socks$` {
				if err == nil {
					ch.Close()
					t.Fatal("UDP channel bypassed socks ACL")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			go ssh.DiscardRequests(reqs)
			defer ch.Close()
		})
	}
	server, err := chserver.NewServer(&chserver.Config{Socks5: true})
	if err != nil {
		t.Fatal(err)
	}
	server.AddUser("user", "pass", `^socks$`)
	port := availablePort()
	if err := server.Start("127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+port, "user", "pass")
	defer sc.Close()
	sendConfig(t, sc, nil)
	ch, reqs, err := sc.OpenChannel("chisel", []byte("socks-udp-v1"))
	if err != nil {
		t.Fatal(err)
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()
	ready := make([]byte, len("chisel-socks-udp-v1\n"))
	if _, err := io.ReadFull(ch, ready); err != nil || string(ready) != "chisel-socks-udp-v1\n" {
		t.Fatalf("exit did not confirm UDP support: %q %v", ready, err)
	}
	echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
	roundTripSocksUDPChannel(t, ch, echo, "before removal")
	server.DeleteUser("user")
	if next, _, err := sc.OpenChannel("chisel", []byte("socks-udp-v1")); err == nil {
		next.Close()
		t.Fatal("removed user opened a UDP channel")
	}
	roundTripSocksUDPChannel(t, ch, echo, "existing association after removal")
}

func roundTripSocksUDPChannel(t *testing.T, ch ssh.Channel, echo *net.UDPAddr, payload string) {
	t.Helper()
	packet := append(append([]byte{0, 0, 0}, socksWireAddress(echo.IP.String(), echo.Port)...), []byte(payload)...)
	frame := binary.BigEndian.AppendUint32(nil, uint32(len(packet)))
	if _, err := ch.Write(append(frame, packet...)); err != nil {
		t.Fatal(err)
	}
	type result struct {
		packet []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		var header [4]byte
		if _, err := io.ReadFull(ch, header[:]); err != nil {
			done <- result{err: err}
			return
		}
		size := binary.BigEndian.Uint32(header[:])
		if size > 9012 {
			done <- result{err: fmt.Errorf("oversized reply %d", size)}
			return
		}
		b := make([]byte, size)
		_, err := io.ReadFull(ch, b)
		done <- result{b, err}
	}()
	select {
	case r := <-done:
		if r.err != nil || !bytes.Equal(r.packet, packet) {
			t.Fatalf("UDP channel response %v: %v", r.packet, r.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("UDP channel timed out")
	}
}

func TestSocksUDPConcurrentDestinations(t *testing.T) {
	addr, _ := setupSocksUDP(t, false, availablePort())
	u := socksUDPClient(t, "127.0.0.1")
	_, relay, code := socksUDPControl(t, addr, 0)
	if code != 0 {
		t.Fatal(code)
	}
	echoes := []*net.UDPAddr{socksUDPEcho(t, "udp4", "127.0.0.1:0"), socksUDPEcho(t, "udp4", "127.0.0.1:0")}
	const packets = 12 // stay below the deliberately bounded queue
	for i := 0; i < packets; i++ {
		echo := echoes[i%len(echoes)]
		packet := append(append([]byte{0, 0, 0}, socksWireAddress(echo.IP.String(), echo.Port)...), []byte(fmt.Sprint(i))...)
		if _, err := u.WriteToUDP(packet, relay); err != nil {
			t.Fatal(err)
		}
	}
	u.SetReadDeadline(time.Now().Add(3 * time.Second))
	seen := make(map[int]bool)
	for i := 0; i < packets; i++ {
		b := make([]byte, 100)
		n, _, err := u.ReadFromUDP(b)
		if err != nil {
			t.Fatal(err)
		}
		r := bytes.NewReader(b[3:n])
		_, port, err := readSocksWireAddress(r)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := io.ReadAll(r)
		id, err := strconv.Atoi(string(payload))
		if err != nil || id < 0 || id >= packets || seen[id] || port != echoes[id%len(echoes)].Port {
			t.Fatalf("misrouted response %q from port %d", payload, port)
		}
		seen[id] = true
	}
}

func TestSocksUDPListenerAddresses(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::1"} {
		t.Run(host, func(t *testing.T) {
			if host == "::1" {
				probe, err := net.Listen("tcp6", "[::1]:0")
				if err != nil {
					t.Skipf("IPv6 unavailable: %v", err)
				}
				probe.Close()
			}
			addr, _ := setupSocksUDPOn(t, false, host, availablePort())
			controlHost, _, _ := net.SplitHostPort(addr)
			u := socksUDPClient(t, controlHost)
			_, relay, code := socksUDPControl(t, addr, 0)
			if code != 0 || relay.String() != addr {
				t.Fatalf("relay %s code %d, want concrete address %s", relay, code, addr)
			}
			echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
			socksUDPRoundTrip(t, u, relay, echo.IP.String(), echo.Port, "listener address")
		})
	}
}

func TestSocksUDPDisabledExit(t *testing.T) {
	port := availablePort()
	layout := testLayout{server: &chserver.Config{}, client: &chclient.Config{Remotes: []string{"127.0.0.1:" + port + ":udpsocks"}}}
	_, _, teardown := layout.setup(t)
	defer teardown()
	_, _, code := socksUDPControl(t, "127.0.0.1:"+port, 0)
	if code != 2 {
		t.Fatalf("SOCKS-disabled exit reply %d, want ruleset failure", code)
	}
}

func TestSocksLegacyTCPChannel(t *testing.T) {
	addr := startSocksACLServer(t, "legacy-socks-tcp", `^socks$`)
	sc, _, _ := dialChiselSSH(t, addr, "user", "pass")
	defer sc.Close()
	sendConfig(t, sc, nil)
	ch, reqs, err := sc.OpenChannel("chisel", []byte("socks"))
	if err != nil {
		t.Fatal(err)
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()
	endpoint, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	go func() {
		c, err := endpoint.Accept()
		if err == nil {
			c.Write([]byte("old client"))
			c.Close()
		}
	}()
	ch.Write([]byte{5, 1, 0})
	var greeting [2]byte
	if _, err := io.ReadFull(ch, greeting[:]); err != nil {
		t.Fatal(err)
	}
	a := endpoint.Addr().(*net.TCPAddr)
	ch.Write(append([]byte{5, 1, 0}, socksWireAddress(a.IP.String(), a.Port)...))
	var header [3]byte
	if _, err := io.ReadFull(ch, header[:]); err != nil || header[1] != 0 {
		t.Fatalf("CONNECT reply %v: %v", header, err)
	}
	if _, _, err := readSocksWireAddress(ch); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, len("old client"))
	if _, err := io.ReadFull(ch, b); err != nil || string(b) != "old client" {
		t.Fatalf("legacy TCP response %q: %v", b, err)
	}
}

func TestSocksTCPHalfClose(t *testing.T) {
	addr, _ := setupSocksUDP(t, false, availablePort())
	endpoint, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	go func() {
		c, err := endpoint.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		b, err := io.ReadAll(c)
		if err == nil {
			c.Write(append(b, '!'))
		}
	}()
	dialer, _ := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	c, err := dialer.Dial("tcp", endpoint.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("half-close")); err != nil {
		t.Fatal(err)
	}
	if err := c.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(c)
	if err != nil || string(b) != "half-close!" {
		t.Fatalf("SOCKS half-close response %q: %v", b, err)
	}
}
