package e2e_test

import (
	"fmt"
	"io"
	"net"
	"testing"

	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

// TestACLAppliesAfterUserRemoval verifies the user is re-resolved per
// channel: once removed (e.g. by an authfile reload), a still-connected
// client is denied new tunnels. Established semantics are documented:
// the session itself is not killed.
func TestACLAppliesAfterUserRemoval(t *testing.T) {
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
			c.Write([]byte("OK"))
			c.Close()
		}
	}()
	s, err := chserver.NewServer(&chserver.Config{KeySeed: "acl-reload-test"})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	if err := s.AddUser("user", "pass", fmt.Sprintf(`^127\.0\.0\.1:%s$`, targetPort)); err != nil {
		t.Fatal(err)
	}
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+serverPort, "user", "pass")
	defer sc.Close()
	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", targetPort, targetPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})
	target := net.JoinHostPort("127.0.0.1", targetPort)
	//while the user exists, channels open fine
	ch, reqs, err := sc.OpenChannel("chisel", []byte(target))
	if err != nil {
		t.Fatalf("channel rejected while user exists: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	b := make([]byte, 2)
	if _, err := io.ReadFull(ch, b); err != nil || string(b) != "OK" {
		t.Fatalf("read through tunnel: %q %v", b, err)
	}
	ch.Close()
	//remove the user (as an authfile reload would)
	s.DeleteUser("user")
	//the connected client's next channel must be denied
	ch2, _, err := sc.OpenChannel("chisel", []byte(target))
	if err == nil {
		ch2.Close()
		t.Fatal("channel accepted after user removal")
	}
	t.Logf("channel correctly rejected after removal: %v", err)
}
