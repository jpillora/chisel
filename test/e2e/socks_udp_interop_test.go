package e2e_test

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/txthinking/socks5"
)

// Exercise negotiation, UDP ASSOCIATE, and datagram encoding/decoding through
// an independent client, without using our SOCKS wire helpers.
func TestSocksUDPThirdPartyClient(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			addr, _ := setupSocksUDP(t, reverse, availablePort())
			for _, target := range []struct {
				name, network, address, host string
			}{
				{"IPv4", "udp4", "127.0.0.1:0", "127.0.0.1"},
				{"domain", "udp", ":0", "localhost"},
				{"IPv6", "udp6", "[::1]:0", "::1"},
			} {
				t.Run(target.name, func(t *testing.T) {
					echo := socksUDPEcho(t, target.network, target.address)
					client, err := socks5.NewClient(addr, "", "", 3, 3)
					if err != nil {
						t.Fatal(err)
					}
					conn, err := client.Dial("udp", net.JoinHostPort(target.host, strconv.Itoa(echo.Port)))
					if err != nil {
						t.Fatalf("third-party UDP ASSOCIATE: %v", err)
					}
					defer conn.Close()
					// Inspect the socket the client opened from Chisel's reply.
					// Every association must use our configured relay port.
					relay := conn.(*socks5.Client).UDPConn.RemoteAddr().String()
					if relay != addr {
						t.Fatalf("third-party client connected to relay %s, want %s", relay, addr)
					}
					buf := make([]byte, 65535)
					for _, size := range []int{1, 1024, 8192} {
						payload := bytes.Repeat([]byte{'x'}, size)
						if n, err := conn.Write(payload); err != nil || n != size {
							t.Fatalf("third-party UDP write (%d bytes): n=%d err=%v", size, n, err)
						}
						n, err := conn.Read(buf)
						if err != nil || !bytes.Equal(buf[:n], payload) {
							t.Fatalf("third-party UDP echo (%d bytes): n=%d err=%v", size, n, err)
						}
					}
				})
			}
		})
	}
}
