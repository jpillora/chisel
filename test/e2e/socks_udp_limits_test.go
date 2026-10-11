package e2e_test

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestSocksUDPAssociationLimit(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, limit := range []int{2, 100} {
			t.Run(fmt.Sprintf("reverse=%t/limit=%d", reverse, limit), func(t *testing.T) {
				value := ""
				if limit != 100 {
					value = strconv.Itoa(limit)
				}
				t.Setenv("CHISEL_UDP_MAX_CONNS", value)
				addr, _ := setupSocksUDP(t, reverse, availablePort())
				echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
				controls := make([]net.Conn, limit)
				clients := make([]*net.UDPConn, limit)
				var relay *net.UDPAddr
				for i := range controls {
					clients[i] = socksUDPClient(t, "127.0.0.1")
					var code byte
					controls[i], relay, code = socksUDPControl(t, addr, clients[i].LocalAddr().(*net.UDPAddr).Port)
					if code != 0 || relay.String() != addr {
						t.Fatalf("association %d/%d code=%d relay=%s", i+1, limit, code, relay)
					}
				}
				extra := socksUDPClient(t, "127.0.0.1")
				_, _, code := socksUDPControl(t, addr, extra.LocalAddr().(*net.UDPAddr).Port)
				if code != 1 {
					t.Fatalf("association %d exceeded cap with reply %d", limit+1, code)
				}
				// Exhaustion must preserve existing UDP and ordinary CONNECT.
				socksUDPRoundTrip(t, clients[0], relay, echo.IP.String(), echo.Port, "at capacity")
				socksTCPRoundTrip(t, addr)
				controls[0].Close()
				deadline := time.Now().Add(3 * time.Second)
				for {
					c, nextRelay, code := socksUDPControl(t, addr, extra.LocalAddr().(*net.UDPAddr).Port)
					if code == 0 {
						socksUDPRoundTrip(t, extra, nextRelay, echo.IP.String(), echo.Port, "recovered capacity")
						c.Close()
						break
					}
					c.Close()
					if code != 1 || time.Now().After(deadline) {
						t.Fatalf("association capacity did not recover: code=%d", code)
					}
					time.Sleep(10 * time.Millisecond)
				}
			})
		}
	}
}

func TestSocksUDPDestinationLimit(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, limit := range []int{2, 100} {
			t.Run(fmt.Sprintf("reverse=%t/limit=%d", reverse, limit), func(t *testing.T) {
				value, idle := "", 2*time.Second
				if limit != 100 {
					value, idle = strconv.Itoa(limit), 500*time.Millisecond
				}
				t.Setenv("CHISEL_UDP_MAX_CONNS", value)
				t.Setenv("CHISEL_UDP_DEADLINE", idle.String())
				addr, _ := setupSocksUDP(t, reverse, availablePort())
				u := socksUDPClient(t, "127.0.0.1")
				_, relay, code := socksUDPControl(t, addr, u.LocalAddr().(*net.UDPAddr).Port)
				if code != 0 {
					t.Fatal(code)
				}
				var first *net.UDPAddr
				for i := range limit {
					echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
					if i == 0 {
						first = echo
					}
					socksUDPRoundTrip(t, u, relay, echo.IP.String(), echo.Port, fmt.Sprintf("destination %d", i))
				}
				blocked, observed := socksUDPLimitEcho(t, "udp4", "127.0.0.1:0", 0)
				packet := append(append([]byte{0, 0, 0}, socksWireAddress(blocked.IP.String(), blocked.Port)...), 'x')
				assertSocksUDPDrop(t, u, relay, packet, observed)
				// Existing flows continue; another association has its own cap.
				socksUDPRoundTrip(t, u, relay, first.IP.String(), first.Port, "existing destination")
				other := socksUDPClient(t, "127.0.0.1")
				_, otherRelay, code := socksUDPControl(t, addr, other.LocalAddr().(*net.UDPAddr).Port)
				if code != 0 {
					t.Fatal(code)
				}
				socksUDPRoundTrip(t, other, otherRelay, blocked.IP.String(), blocked.Port, "independent cap")
				<-observed
				if limit == 2 {
					// Expiration must free capacity without replacing the control.
					time.Sleep(idle + 150*time.Millisecond)
					socksUDPRoundTrip(t, u, relay, blocked.IP.String(), blocked.Port, "after idle expiry")
				}
			})
		}
	}
}

func TestSocksUDPPacketSizeLimit(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, test := range []struct {
			name, setting, network, address string
			limit                           int
		}{
			{"small", "64", "udp4", "127.0.0.1:0", 64},
			{"default", "", "udp4", "127.0.0.1:0", 9012},
			{"IPv6", "64", "udp6", "[::1]:0", 64},
			{"clamped", "70000", "udp4", "127.0.0.1:0", 65507},
		} {
			t.Run(fmt.Sprintf("reverse=%t/%s", reverse, test.name), func(t *testing.T) {
				if test.limit == 65507 {
					// Probe the OS independently of Chisel. macOS defaults to a
					// smaller UDP send limit; relay failures must still fail below.
					probe := socksUDPClient(t, "127.0.0.1")
					_, err := probe.WriteToUDP(make([]byte, test.limit), probe.LocalAddr().(*net.UDPAddr))
					probe.Close()
					if errors.Is(err, syscall.EMSGSIZE) ||
						(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10040))) {
						t.Skipf("OS cannot send a %d-byte UDP datagram: %v", test.limit, err)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("CHISEL_UDP_MAX_SIZE", test.setting)
				echo, observed := socksUDPLimitEcho(t, test.network, test.address, test.limit)
				addr, _ := setupSocksUDP(t, reverse, availablePort())
				u := socksUDPClient(t, "127.0.0.1")
				_, relay, code := socksUDPControl(t, addr, u.LocalAddr().(*net.UDPAddr).Port)
				if code != 0 {
					t.Fatal(code)
				}
				header := append([]byte{0, 0, 0}, socksWireAddress(echo.IP.String(), echo.Port)...)
				boundary := bytes.Repeat([]byte{'x'}, test.limit-len(header))
				socksUDPRoundTrip(t, u, relay, echo.IP.String(), echo.Port, string(boundary))
				<-observed
				if test.limit < 65507 {
					packet := append(append([]byte{}, header...), append(boundary, 'x')...)
					assertSocksUDPDrop(t, u, relay, packet, observed)
				}
				// This request fits; the endpoint deliberately replies one byte
				// above the limit. The exit must drop the oversized response.
				packet := append(append([]byte{}, header...), []byte("oversized reply")...)
				if _, err := u.WriteToUDP(packet, relay); err != nil {
					t.Fatal(err)
				}
				select {
				case <-observed:
				case <-time.After(time.Second):
					t.Fatal("reply-size probe did not reach endpoint")
				}
				assertSocksUDPTimeout(t, u)
				socksUDPRoundTrip(t, u, relay, echo.IP.String(), echo.Port, "valid after oversized packets")
			})
		}
	}
}

// The endpoint records every request, so a missing reply cannot hide an
// over-limit request that was incorrectly forwarded by the exit.
func socksUDPLimitEcho(t *testing.T, network, address string, maxSize int) (*net.UDPAddr, <-chan []byte) {
	t.Helper()
	a, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.ListenUDP(network, a)
	if err != nil {
		if network == "udp6" {
			t.Skipf("IPv6 unavailable: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	observed := make(chan []byte, 4)
	go func() {
		buf := make([]byte, 65535)
		for {
			n, peer, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			payload := append([]byte{}, buf[:n]...)
			select {
			case observed <- payload:
			default:
			}
			if maxSize > 0 && string(payload) == "oversized reply" {
				headerLen := 3 + len(socksWireAddress(c.LocalAddr().(*net.UDPAddr).IP.String(), c.LocalAddr().(*net.UDPAddr).Port))
				payload = bytes.Repeat([]byte{'y'}, maxSize-headerLen+1)
			}
			c.WriteToUDP(payload, peer)
		}
	}()
	return c.LocalAddr().(*net.UDPAddr), observed
}

func assertSocksUDPDrop(t *testing.T, u *net.UDPConn, relay *net.UDPAddr, packet []byte, observed <-chan []byte) {
	t.Helper()
	if _, err := u.WriteToUDP(packet, relay); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-observed:
		t.Fatalf("over-limit packet reached the destination: %d bytes", len(payload))
	case <-time.After(75 * time.Millisecond):
	}
	assertSocksUDPTimeout(t, u)
}

func assertSocksUDPTimeout(t *testing.T, u *net.UDPConn) {
	t.Helper()
	u.SetReadDeadline(time.Now().Add(75 * time.Millisecond))
	_, _, err := u.ReadFromUDP(make([]byte, 65535))
	if e, ok := err.(net.Error); !ok || !e.Timeout() {
		t.Fatalf("expected dropped datagram, got %v", err)
	}
}
