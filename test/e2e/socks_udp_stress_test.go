package e2e_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/txthinking/socks5"
)

// Opt-in because this deliberately overloads UDP sockets and the relay queue.
// Loss is measured, not asserted: it depends on the host and socket buffers.
func TestSocksUDPOverload(t *testing.T) {
	if os.Getenv("CHISEL_UDP_STRESS") != "1" {
		t.Skip("set CHISEL_UDP_STRESS=1 to run UDP overload measurements")
	}
	t.Setenv("CHISEL_UDP_MAX_SIZE", "")
	t.Setenv("CHISEL_UDP_MAX_CONNS", "")
	t.Setenv("CHISEL_UDP_DEADLINE", "")
	for _, path := range []string{"direct", "forward", "reverse"} {
		t.Run(path, func(t *testing.T) {
			const packets, size = 20000, 1200
			echo := socksUDPEcho(t, "udp4", "127.0.0.1:0")
			quietEcho := socksUDPEcho(t, "udp4", "127.0.0.1:0")
			var addr string
			if path != "direct" {
				addr, _ = setupSocksUDP(t, path == "reverse", availablePort())
			}
			dial := func(destination *net.UDPAddr) net.Conn {
				var c net.Conn
				var err error
				if path == "direct" {
					c, err = net.Dial("udp4", destination.String())
				} else {
					client, e := socks5.NewClient(addr, "", "", 5, 0)
					if e != nil {
						t.Fatal(e)
					}
					c, err = client.Dial("udp", destination.String())
				}
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.Close() })
				if err := benchmarkUDPEcho(c, []byte("warmup"), make([]byte, 128)); err != nil {
					t.Fatal(err)
				}
				return c
			}
			c, quiet := dial(echo), dial(quietEcho)
			var sendingDone atomic.Bool
			type result struct {
				latencies []int64
				duplicate int
				err       error
			}
			results := make(chan result, 1)
			go func() {
				buf := make([]byte, size+64)
				tail := bytes.Repeat([]byte{'x'}, size-16)
				seen := make([]bool, packets)
				r := result{latencies: make([]int64, 0, packets)}
				for {
					c.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
					n, err := c.Read(buf)
					if e, ok := err.(net.Error); ok && e.Timeout() {
						if sendingDone.Load() {
							results <- r
							return
						}
						continue
					}
					if err != nil || n != size {
						r.err = fmt.Errorf("overload reply n=%d: %v", n, err)
						results <- r
						return
					}
					seq := binary.BigEndian.Uint64(buf[:8])
					if seq >= packets || !bytes.Equal(buf[16:n], tail) {
						r.err = fmt.Errorf("corrupted overload reply seq=%d", seq)
						results <- r
						return
					}
					if seen[seq] {
						r.duplicate++
						continue
					}
					seen[seq] = true
					sent := int64(binary.BigEndian.Uint64(buf[8:16]))
					r.latencies = append(r.latencies, time.Now().UnixNano()-sent)
				}
			}()
			quietDone := make(chan error, 1)
			go func() {
				for range 20 {
					if err := benchmarkUDPEcho(quiet, []byte("unrelated client"), make([]byte, 128)); err != nil {
						quietDone <- err
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
				quietDone <- nil
			}()
			payload := bytes.Repeat([]byte{'x'}, size)
			start := time.Now()
			c.SetWriteDeadline(start.Add(10 * time.Second))
			for seq := range packets {
				binary.BigEndian.PutUint64(payload[:8], uint64(seq))
				binary.BigEndian.PutUint64(payload[8:16], uint64(time.Now().UnixNano()))
				if n, err := c.Write(payload); err != nil || n != size {
					t.Fatalf("overload send %d n=%d: %v", seq, n, err)
				}
			}
			elapsed := time.Since(start)
			sendingDone.Store(true)
			r := <-results
			if r.err != nil || r.duplicate != 0 || len(r.latencies) == 0 {
				t.Fatalf("invalid overload results: received=%d duplicates=%d err=%v", len(r.latencies), r.duplicate, r.err)
			}
			if err := <-quietDone; err != nil {
				t.Fatalf("overload blocked unrelated association: %v", err)
			}
			if err := benchmarkUDPEcho(c, []byte("after overload"), make([]byte, 128)); err != nil {
				t.Fatalf("association did not recover after overload: %v", err)
			}
			slices.Sort(r.latencies)
			t.Logf("sent=%d received=%d loss=%.2f%% offered=%.0f packets/s p50=%.1f us p95=%.1f us; 20 unrelated-client probes and recovery passed",
				packets, len(r.latencies), 100*float64(packets-len(r.latencies))/packets, float64(packets)/elapsed.Seconds(),
				float64(r.latencies[len(r.latencies)*50/100])/1000, float64(r.latencies[len(r.latencies)*95/100])/1000)
		})
	}
}
