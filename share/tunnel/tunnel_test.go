package tunnel

import (
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type blockingConn struct {
	ssh.Conn
	release chan struct{}
}

func (c *blockingConn) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	<-c.release
	return true, []byte("pong"), nil
}

func TestPingWithTimeout(t *testing.T) {
	conn := &blockingConn{release: make(chan struct{})}
	defer close(conn.release)

	timeout := 50 * time.Millisecond
	start := time.Now()
	err := pingWithTimeout(conn, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed < timeout {
		t.Fatalf("returned before the timeout elapsed: %s < %s", elapsed, timeout)
	}
	if elapsed > timeout+500*time.Millisecond {
		t.Fatalf("ping did not time out promptly: %s", elapsed)
	}
}
