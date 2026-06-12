package tunnel

import (
	"net"
	"testing"
	"time"

	"github.com/jpillora/chisel/share/cio"
)

// TestUDPSweepWriteOnly verifies that only stale write-only connections
// are swept; fresh write-only conns and reader-owned conns must remain.
func TestUDPSweepWriteOnly(t *testing.T) {
	pipe := func() net.Conn {
		a, _ := net.Pipe()
		return a
	}
	now := time.Now()
	cs := &udpConns{
		Logger: cio.NewLogger("test"),
		m: map[string]*udpConn{
			"stale":  {id: "stale", Conn: pipe(), writeOnly: true, lastWrite: now.Add(-time.Minute)},
			"fresh":  {id: "fresh", Conn: pipe(), writeOnly: true, lastWrite: now},
			"reader": {id: "reader", Conn: pipe()},
		},
	}
	cs.sweepWriteOnly(now.Add(-30 * time.Second))
	if _, ok := cs.m["stale"]; ok {
		t.Fatal("stale write-only conn was not swept")
	}
	if _, ok := cs.m["fresh"]; !ok {
		t.Fatal("fresh write-only conn was wrongly swept")
	}
	if _, ok := cs.m["reader"]; !ok {
		t.Fatal("reader conn was wrongly swept")
	}
}
