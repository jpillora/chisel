package e2e_test

import (
	"bytes"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/settings"
)

// syncBuffer is a goroutine-safe writer for capturing log output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestConnCloseBeforeConfig verifies the server survives a client which
// completes the SSH handshake but drops the connection before sending the
// "config" request (issue #608 bug 2). The closed request channel yields a
// nil *ssh.Request; without a nil guard the handler panics (recovered by
// net/http, which logs "http: panic serving" via the global log package).
func TestConnCloseBeforeConfig(t *testing.T) {
	// capture the global logger, where net/http reports recovered handler panics
	buf := &syncBuffer{}
	log.SetOutput(buf)
	defer log.SetOutput(os.Stderr)

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "preconfig-test",
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
	serverAddr := "127.0.0.1:" + port

	// handshake then hang up without sending the config request
	sc, _, _ := dialChiselSSH(t, serverAddr, "user", "pass")
	sc.Close()

	// give the handler time to observe the closed request channel,
	// failing fast if the panic appears
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "panic") {
			t.Fatalf("server panicked on pre-config close:\n%s", buf.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	// server must still serve a well-behaved client afterwards
	sc2, _, _ := dialChiselSSH(t, serverAddr, "user", "pass")
	defer sc2.Close()
	r, err := settings.DecodeRemote("0.0.0.0:" + availablePort() + ":127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc2, []*settings.Remote{r})
}
