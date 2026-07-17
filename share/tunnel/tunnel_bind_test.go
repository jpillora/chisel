package tunnel

import (
	"context"
	"net"
	"testing"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
)

// TestBindRemotesUnbindsOnPartialFailure verifies that when one remote
// fails to bind, the previously bound listeners are released instead of
// staying bound for the process lifetime.
func TestBindRemotesUnbindsOnPartialFailure(t *testing.T) {
	//occupy a port so the second remote fails to bind
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	_, occupiedPort, _ := net.SplitHostPort(blocker.Addr().String())
	//pick a free port for the first remote
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, freePort, _ := net.SplitHostPort(probe.Addr().String())
	probe.Close()
	//bind: first remote succeeds, second fails
	tun := New(Config{Logger: cio.NewLogger("test"), Inbound: true})
	remotes := []*settings.Remote{}
	for _, s := range []string{
		"127.0.0.1:" + freePort + ":127.0.0.1:1",
		"127.0.0.1:" + occupiedPort + ":127.0.0.1:1",
	} {
		r, err := settings.DecodeRemote(s)
		if err != nil {
			t.Fatal(err)
		}
		remotes = append(remotes, r)
	}
	if err := tun.BindRemotes(context.Background(), remotes); err == nil {
		t.Fatal("expected bind error for occupied port")
	}
	//the first remote's port must be free again
	l, err := net.Listen("tcp", "127.0.0.1:"+freePort)
	if err != nil {
		t.Fatalf("port %s still bound after failed BindRemotes: %v", freePort, err)
	}
	l.Close()
}
