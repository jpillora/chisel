package e2e_test

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/settings"
)

// TestHalfCloseThroughTunnel verifies TCP half-close semantics across
// the tunnel: the client sends a request, closes its write side, and
// the server only replies after reading EOF. With the old Pipe, the
// CloseWrite became a full teardown and the reply was lost.
func TestHalfCloseThroughTunnel(t *testing.T) {
	//target which reads until EOF, then replies, then closes
	targetPort := availablePort()
	tl, err := net.Listen("tcp", "127.0.0.1:"+targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	go func() {
		for {
			c, err := tl.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				b, _ := io.ReadAll(c)
				c.Write(append(b, '!'))
			}(c)
		}
	}()
	//chisel client+server
	tunnelPort := availablePort()
	teardown := simpleSetup(t,
		&chserver.Config{},
		&chclient.Config{
			Remotes: []string{tunnelPort + ":127.0.0.1:" + targetPort},
		},
	)
	defer teardown()
	//send, half-close, then read the reply until EOF
	conn, err := net.Dial("tcp", "127.0.0.1:"+tunnelPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	if got := string(b); got != "hello!" {
		t.Fatalf("got %q, want %q", got, "hello!")
	}
}

// TestDialFailureRejectsChannel verifies that when the exit side cannot
// reach the target, the ssh channel is rejected (instead of accepted
// and silently torn down) so the failure propagates to the client.
func TestDialFailureRejectsChannel(t *testing.T) {
	//allocate a port and leave it closed
	deadPort := availablePort()
	s, err := chserver.NewServer(&chserver.Config{KeySeed: "dial-fail-test"})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+serverPort, "", "")
	defer sc.Close()
	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", deadPort, deadPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})
	ch, _, err := sc.OpenChannel("chisel", []byte("127.0.0.1:"+deadPort))
	if err == nil {
		ch.Close()
		t.Fatal("channel to unreachable target was accepted")
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Logf("rejection reason: %v", err)
	}
}

// TestDialFailureClosesLocalConn verifies the inbound side: a client
// connecting to the local proxy port for a dead target must see the
// connection close promptly with no data, not hang.
func TestDialFailureClosesLocalConn(t *testing.T) {
	deadPort := availablePort()
	tunnelPort := availablePort()
	teardown := simpleSetup(t,
		&chserver.Config{},
		&chclient.Config{
			Remotes: []string{tunnelPort + ":127.0.0.1:" + deadPort},
		},
	)
	defer teardown()
	conn, err := net.Dial("tcp", "127.0.0.1:"+tunnelPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	b := make([]byte, 8)
	n, err := conn.Read(b)
	if err != io.EOF {
		t.Fatalf("expected prompt EOF, got n=%d err=%v", n, err)
	}
}
