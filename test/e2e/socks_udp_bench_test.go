package e2e_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/txthinking/socks5"
)

// BenchmarkSocksUDP measures warmed UDP echo round trips over loopback.
// The SOCKS paths use an independent client and the real encrypted tunnel.
// MB/s counts delivered application payload once per echo, not both wire
// directions. Timed-out requests count as loss, with a 100ms client timeout.
func BenchmarkSocksUDP(b *testing.B) {
	b.Setenv("CHISEL_UDP_MAX_SIZE", "")
	b.Setenv("CHISEL_UDP_MAX_CONNS", "")
	b.Setenv("CHISEL_UDP_DEADLINE", "")
	for _, path := range []string{"direct", "forward", "reverse"} {
		for _, size := range []int{64, 1200, 8192} {
			for _, workers := range []int{1, 16} {
				b.Run(fmt.Sprintf("%s/bytes=%d/clients=%d", path, size, workers), func(b *testing.B) {
					echo := socksUDPEcho(b, "udp4", "127.0.0.1:0")
					var addr string
					if path != "direct" {
						addr, _ = setupSocksUDP(b, path == "reverse", availablePort())
					}
					payload := bytes.Repeat([]byte{'x'}, size)
					conns := make([]net.Conn, workers)
					for i := range conns {
						var err error
						if path == "direct" {
							conns[i], err = net.Dial("udp4", echo.String())
						} else {
							client, e := socks5.NewClient(addr, "", "", 5, 0)
							if e != nil {
								b.Fatal(e)
							}
							conns[i], err = client.Dial("udp", echo.String())
						}
						if err != nil {
							b.Fatal(err)
						}
						c := conns[i]
						b.Cleanup(func() { c.Close() })
						// Pin each wildcard association before opening the next one.
						if err := benchmarkUDPEcho(c, payload, make([]byte, size+64)); err != nil {
							b.Fatal(err)
						}
					}
					samples := make([][]int64, workers)
					buffers := make([][]byte, workers)
					payloads := make([][]byte, workers)
					losses := make([]int, workers)
					for i := range samples {
						samples[i] = make([]int64, 0, 2048)
						buffers[i] = make([]byte, size+64)
						payloads[i] = bytes.Clone(payload)
					}
					errors := make(chan error, workers)
					var wg sync.WaitGroup
					sampleEvery := max(1, b.N/workers/2048)
					b.ReportAllocs()
					b.ResetTimer()
					for worker := range workers {
						wg.Add(1)
						go func() {
							defer wg.Done()
							for i, n := worker, 0; i < b.N; i, n = i+workers, n+1 {
								binary.BigEndian.PutUint64(payloads[worker][:8], uint64(i))
								start := time.Now()
								if err := benchmarkUDPMeasured(conns[worker], payloads[worker], buffers[worker]); err != nil {
									if e, ok := err.(net.Error); ok && e.Timeout() {
										losses[worker]++
										continue
									}
									errors <- err
									return
								}
								if n%sampleEvery == 0 && len(samples[worker]) < cap(samples[worker]) {
									samples[worker] = append(samples[worker], time.Since(start).Nanoseconds())
								}
							}
						}()
					}
					wg.Wait()
					b.StopTimer()
					close(errors)
					for err := range errors {
						b.Fatal(err)
					}
					var all []int64
					var lost int
					for i, sample := range samples {
						all = append(all, sample...)
						lost += losses[i]
					}
					if len(all) == 0 {
						b.Fatal("all measured datagrams were lost")
					}
					slices.Sort(all)
					b.ReportMetric(float64(all[len(all)*50/100])/1000, "p50-us")
					b.ReportMetric(float64(all[len(all)*95/100])/1000, "p95-us")
					b.ReportMetric(float64(b.N-lost)/b.Elapsed().Seconds(), "roundtrips/s")
					b.ReportMetric(float64(b.N-lost)*float64(size)/b.Elapsed().Seconds()/1e6, "MB/s")
					b.ReportMetric(100*float64(lost)/float64(b.N), "loss-percent")
				})
			}
		}
	}
}

func benchmarkUDPMeasured(c net.Conn, payload, buf []byte) error {
	if err := c.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return err
	}
	if n, err := c.Write(payload); err != nil || n != len(payload) {
		return fmt.Errorf("UDP write n=%d: %v", n, err)
	}
	for {
		n, err := c.Read(buf)
		if err != nil {
			return err
		}
		if n != len(payload) {
			return fmt.Errorf("UDP echo n=%d, want %d", n, len(payload))
		}
		if !bytes.Equal(buf[:8], payload[:8]) {
			continue // late reply to an operation already counted as lost
		}
		if !bytes.Equal(buf[:n], payload) {
			return fmt.Errorf("UDP echo payload corruption")
		}
		return nil
	}
}

func benchmarkUDPEcho(c net.Conn, payload, buf []byte) error {
	if err := c.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	if n, err := c.Write(payload); err != nil || n != len(payload) {
		return fmt.Errorf("UDP write n=%d: %v", n, err)
	}
	n, err := c.Read(buf)
	if err != nil || !bytes.Equal(buf[:n], payload) {
		return fmt.Errorf("UDP echo n=%d: %v", n, err)
	}
	return nil
}
