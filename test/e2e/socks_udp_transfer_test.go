package e2e_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TFTP starts at one server port, receives DATA from a new transfer port,
// and sends ACKs to that port while retaining its own transfer ID (source
// port). Exercise two associations to ensure their exit sockets stay isolated.
func TestSocksUDPTransferPort(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, kind := range []string{"udp4", "udp6", "different-IP"} {
			t.Run(fmt.Sprintf("reverse=%t/%s", reverse, kind), func(t *testing.T) {
				network := kind
				if kind == "different-IP" {
					network = "udp4"
				}
				host := "127.0.0.1"
				if network == "udp6" {
					host = "::1"
				}
				listen := func() *net.UDPConn {
					c, err := net.ListenUDP(network, &net.UDPAddr{IP: net.ParseIP(host)})
					if err != nil {
						if network == "udp6" {
							t.Skipf("IPv6 unavailable: %v", err)
						}
						if host == "127.0.0.2" && (errors.Is(err, syscall.EADDRNOTAVAIL) ||
							(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10049)))) {
							t.Skipf("alternate loopback unavailable: %v", err)
						}
						t.Fatal(err)
					}
					t.Cleanup(func() { c.Close() })
					return c
				}
				service := listen()
				if kind == "different-IP" {
					host = "127.0.0.2"
				}
				transfer := listen()
				serviceAddr := service.LocalAddr().(*net.UDPAddr)
				transferAddr := transfer.LocalAddr().(*net.UDPAddr)
				proxyAddr, _ := setupSocksUDP(t, reverse, availablePort())
				clients := [2]*net.UDPConn{}
				relays := [2]*net.UDPAddr{}
				peers := [2]*net.UDPAddr{}
				read := func(c *net.UDPConn, want []byte) *net.UDPAddr {
					t.Helper()
					c.SetReadDeadline(time.Now().Add(3 * time.Second))
					buf := make([]byte, 1024)
					n, from, err := c.ReadFromUDP(buf)
					if err != nil || !bytes.Equal(buf[:n], want) {
						t.Fatalf("datagram %x, want %x: %v", buf[:n], want, err)
					}
					return from
				}
				send := func(c *net.UDPConn, to *net.UDPAddr, data []byte) {
					t.Helper()
					if _, err := c.WriteToUDP(data, to); err != nil {
						t.Fatal(err)
					}
				}
				request := func(i int, to *net.UDPAddr, data []byte) {
					packet := append(append([]byte{0, 0, 0}, socksWireAddress(to.IP.String(), to.Port)...), data...)
					send(clients[i], relays[i], packet)
				}
				rrq := []byte("\x00\x01test\x00octet\x00")
				for i := range clients {
					clients[i] = socksUDPClient(t, "127.0.0.1")
					_, relay, code := socksUDPControl(t, proxyAddr, clients[i].LocalAddr().(*net.UDPAddr).Port)
					if code != 0 {
						t.Fatalf("ASSOCIATE failed: %d", code)
					}
					relays[i] = relay
					request(i, serviceAddr, rrq)
					peers[i] = read(service, rrq)
				}
				if peers[0].String() == peers[1].String() {
					t.Fatal("associations share an exit source socket")
				}
				for block := uint16(1); block <= 2; block++ {
					data := binary.BigEndian.AppendUint16([]byte{0, 3}, block)
					if block == 1 {
						data = append(data, bytes.Repeat([]byte{'x'}, 512)...)
					} else {
						data = append(data, 'y') // final short DATA block
					}
					for i := range clients {
						send(transfer, peers[i], data)
						clients[i].SetReadDeadline(time.Now().Add(3 * time.Second))
						buf := make([]byte, 1024)
						n, relay, err := clients[i].ReadFromUDP(buf)
						if err != nil {
							t.Fatalf("DATA block %d reply from new server port: %v", block, err)
						}
						if relay.String() != relays[i].String() || n < 3 || !bytes.Equal(buf[:3], []byte{0, 0, 0}) {
							t.Fatalf("invalid relay packet from %s: %x", relay, buf[:n])
						}
						r := bytes.NewReader(buf[3:n])
						host, port, err := readSocksWireAddress(r)
						if err != nil || host != transferAddr.IP.String() || port != transferAddr.Port {
							t.Fatalf("reply source %s:%d, want %s: %v", host, port, transferAddr, err)
						}
						payload, _ := io.ReadAll(r)
						if !bytes.Equal(payload, data) {
							t.Fatalf("wrong DATA block: %x", payload)
						}
						ack := binary.BigEndian.AppendUint16([]byte{0, 4}, block)
						request(i, &net.UDPAddr{IP: net.ParseIP(host), Port: port}, ack)
						if peer := read(transfer, ack); peer.String() != peers[i].String() {
							t.Fatalf("source changed during transfer: %s -> %s", peers[i], peer)
						}
					}
				}
			})
		}
	}
}

func TestSocksUDPSourcePortSurvivesIdle(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			t.Setenv("CHISEL_UDP_DEADLINE", "50ms")
			addr, _ := setupSocksUDP(t, reverse, availablePort())
			client := socksUDPClient(t, "127.0.0.1")
			service := socksUDPClient(t, "127.0.0.1")
			_, relay, code := socksUDPControl(t, addr, 0)
			if code != 0 {
				t.Fatal(code)
			}
			to := service.LocalAddr().(*net.UDPAddr)
			packet := append(append([]byte{0, 0, 0}, socksWireAddress(to.IP.String(), to.Port)...), 'x')
			probe := func() *net.UDPAddr {
				if _, err := client.WriteToUDP(packet, relay); err != nil {
					t.Fatal(err)
				}
				service.SetReadDeadline(time.Now().Add(3 * time.Second))
				_, peer, err := service.ReadFromUDP(make([]byte, 32))
				if err != nil {
					t.Fatal(err)
				}
				return peer
			}
			before := probe()
			time.Sleep(150 * time.Millisecond)
			if after := probe(); after.String() != before.String() {
				t.Fatalf("idle expiration changed source: %s -> %s", before, after)
			}
		})
	}
}
