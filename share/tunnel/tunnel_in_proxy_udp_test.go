package tunnel

import (
	"context"
	"encoding/gob"
	"net"
	"testing"
	"time"

	"github.com/jpillora/chisel/share/cio"
)

func TestUDPListenerReturnsOnlyToObservedPeers(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	allowed, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer allowed.Close()
	blocked, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &udpListener{
		Logger:       cio.NewLogger("test"),
		inbound:      listener,
		outbound:     &udpChannel{r: gob.NewDecoder(server), w: gob.NewEncoder(server), c: server},
		peers:        map[string]udpPeer{},
		peerDeadline: time.Second,
		maxPeers:     2,
	}
	u.rememberPeer(allowed.LocalAddr().(*net.UDPAddr))
	runErr := make(chan error, 1)
	go func() { runErr <- u.runOutbound(ctx) }()

	encode := gob.NewEncoder(client)
	if err := encode.Encode(udpPacket{Src: blocked.LocalAddr().String(), Payload: []byte("blocked")}); err != nil {
		t.Fatal(err)
	}
	blocked.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := blocked.ReadFromUDP(make([]byte, 64)); err == nil {
		t.Fatal("unobserved peer received a return packet")
	} else if e, ok := err.(net.Error); !ok || !e.Timeout() {
		t.Fatalf("read from unobserved peer: %v", err)
	}

	if err := encode.Encode(udpPacket{Src: allowed.LocalAddr().String(), Payload: []byte("allowed")}); err != nil {
		t.Fatal(err)
	}
	allowed.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	n, _, err := allowed.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "allowed" {
		t.Fatalf("got %q, want allowed", got)
	}

	cancel()
	client.Close()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runOutbound did not stop")
	}
}
