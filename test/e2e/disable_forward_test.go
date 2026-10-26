package e2e_test

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/settings"
)

// TestForwardRemotesWithDisableForward runs a client with both a forward and a
// reverse remote. With --disable-forward the session stays up and the
// reverse remote works, but forward connections are refused.
func TestForwardRemotesWithDisableForward(t *testing.T) {
	for _, disableForward := range []bool{false, true} {
		t.Run(fmt.Sprintf("disableForward=%v", disableForward), func(t *testing.T) {
			fwdPort := availablePort()
			revPort := availablePort()
			teardown := simpleSetup(t,
				&chserver.Config{Reverse: true, NoForward: disableForward},
				&chclient.Config{
					Remotes: []string{fwdPort + ":$FILEPORT", "R:" + revPort + ":$FILEPORT"},
				})
			defer teardown()
			if result, err := post("http://localhost:"+revPort, "foo"); err != nil || result != "foo!" {
				t.Fatalf("reverse remote: got %q, %v", result, err)
			}
			result, err := post("http://localhost:"+fwdPort, "foo")
			if !disableForward && (err != nil || result != "foo!") {
				t.Fatalf("forward remote: got %q, %v", result, err)
			}
			if disableForward && err == nil {
				t.Fatalf("forward remote was not refused: got %q", result)
			}
		})
	}
}

// TestOutboundChannelsRejectedWithDisableForward opens channels directly
// to confirm the server never dials a target with --disable-forward.
func TestOutboundChannelsRejectedWithDisableForward(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		for {
			conn, err := target.Accept()
			if err != nil {
				return
			}
			conn.Close()
			select {
			case accepted <- struct{}{}:
			default:
			}
		}
	}()

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed:   "disable-forward-channel",
		Reverse:   true,
		Socks5:    true,
		NoForward: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	port := availablePort()
	if err := s.Start("127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+port, "", "")
	defer sc.Close()
	//forward remotes in the config are accepted; their channels are not
	var remotes []*settings.Remote
	for _, spec := range []string{availablePort() + ":" + target.Addr().String(), "stdio:" + target.Addr().String()} {
		r, err := settings.DecodeRemote(spec)
		if err != nil {
			t.Fatal(err)
		}
		remotes = append(remotes, r)
	}
	sendConfig(t, sc, remotes)

	for _, dest := range []string{target.Addr().String(), target.Addr().String() + "/udp", "socks"} {
		ch, _, err := sc.OpenChannel("chisel", []byte(dest))
		if err == nil {
			ch.Close()
			t.Fatalf("channel to %q was accepted", dest)
		}
		if !strings.Contains(err.Error(), "Denied outbound connection") {
			t.Fatalf("channel to %q rejected for the wrong reason: %v", dest, err)
		}
	}
	select {
	case <-accepted:
		t.Fatal("server dialed the target")
	case <-time.After(200 * time.Millisecond):
	}
}
