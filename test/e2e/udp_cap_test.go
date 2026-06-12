package e2e_test

import (
	"net"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

// TestUDPConnCapRecovery verifies the UDP conn cap behavior: flows over
// the cap (UDP_MAX_CONNS) are write-only, and — unlike the old behavior
// where over-cap flows were never removed and permanently broke all new
// flows — they are swept once idle, so later flows get read slots again.
func TestUDPConnCapRecovery(t *testing.T) {
	t.Setenv("CHISEL_UDP_MAX_CONNS", "1")
	t.Setenv("CHISEL_UDP_DEADLINE", "300ms")
	//udp echo server
	echoPort := availableUDPPort()
	a, _ := net.ResolveUDPAddr("udp", ":"+echoPort)
	l, err := net.ListenUDP("udp", a)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		b := make([]byte, 128)
		for {
			n, addr, err := l.ReadFrom(b)
			if err != nil {
				return
			}
			l.WriteTo(b[:n], addr)
		}
	}()
	//chisel client+server
	inboundPort := availableUDPPort()
	teardown := simpleSetup(t,
		&chserver.Config{},
		&chclient.Config{
			Remotes: []string{
				inboundPort + ":" + echoPort + "/udp",
			},
		},
	)
	defer teardown()
	//echo sends one packet from a fresh source port (= a new flow on
	//the exit node) and waits for the reply
	echo := func(msg string) (string, error) {
		conn, err := net.Dial("udp4", "localhost:"+inboundPort)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, 128)
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(b)
		if err != nil {
			return "", err
		}
		return string(b[:n]), nil
	}
	//flow 1 takes the only read slot
	if got, err := echo("one"); err != nil || got != "one" {
		t.Fatalf("flow 1 failed: %q %v", got, err)
	}
	//flow 2 is over the cap: write-only, so its reply must be dropped
	if got, err := echo("two"); err == nil {
		t.Fatalf("flow 2 got a reply (%q) despite the conn cap", got)
	}
	//wait past the read deadline (frees flow 1) and the sweep interval
	//(allows flow 2 to be swept on the next packet)
	time.Sleep(1200 * time.Millisecond)
	//flow 3's packet sweeps the idle write-only flow 2 and gets a
	//read slot — before the sweep existed, every flow from here on
	//was permanently write-only
	if got, err := echo("three"); err != nil || got != "three" {
		t.Fatalf("flow 3 did not recover after sweep: %q %v", got, err)
	}
}
