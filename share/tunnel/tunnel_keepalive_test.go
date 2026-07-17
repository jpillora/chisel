package tunnel

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"golang.org/x/crypto/ssh"
)

//mockSSHConn implements ssh.Conn for keepalive tests.
//When dead, SendRequest blocks until the connection is closed, simulating
//a dead TCP connection where no RST ever arrives (e.g. after OS sleep/wake).
//When healthy, SendRequest replies "pong" immediately.
type mockSSHConn struct {
	dead      bool
	closeOnce sync.Once
	closed    chan struct{}
	pings     atomic.Int32
}

var _ ssh.Conn = (*mockSSHConn)(nil)

func newMockSSHConn(dead bool) *mockSSHConn {
	return &mockSSHConn{dead: dead, closed: make(chan struct{})}
}

func (m *mockSSHConn) SendRequest(string, bool, []byte) (bool, []byte, error) {
	if m.dead {
		<-m.closed
		return false, nil, net.ErrClosed
	}
	select {
	case <-m.closed:
		return false, nil, net.ErrClosed
	default:
		m.pings.Add(1)
		return true, []byte("pong"), nil
	}
}

func (m *mockSSHConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, net.ErrClosed
}

func (m *mockSSHConn) Close() error {
	m.closeOnce.Do(func() { close(m.closed) })
	return nil
}

func (m *mockSSHConn) Wait() error           { <-m.closed; return nil }
func (m *mockSSHConn) User() string          { return "" }
func (m *mockSSHConn) SessionID() []byte     { return nil }
func (m *mockSSHConn) ClientVersion() []byte { return nil }
func (m *mockSSHConn) ServerVersion() []byte { return nil }
func (m *mockSSHConn) RemoteAddr() net.Addr  { return &net.TCPAddr{} }
func (m *mockSSHConn) LocalAddr() net.Addr   { return &net.TCPAddr{} }

//TestKeepAliveLoopTimeout verifies that keepAliveLoop closes the
//connection when a ping receives no reply within the ping timeout
//(the silently-dead connection scenario).
func TestKeepAliveLoopTimeout(t *testing.T) {
	conn := newMockSSHConn(true)
	tun := New(Config{
		Logger:    cio.NewLogger("test"),
		KeepAlive: 50 * time.Millisecond,
	})
	go tun.keepAliveLoop(conn)
	select {
	case <-conn.closed:
		//dead connection detected and closed
	case <-time.After(2 * time.Second):
		t.Fatal("keepAliveLoop did not close dead connection")
	}
}

//TestKeepAliveLoopHealthy verifies that keepAliveLoop keeps pinging a
//responsive connection without closing it.
func TestKeepAliveLoopHealthy(t *testing.T) {
	conn := newMockSSHConn(false)
	tun := New(Config{
		Logger:    cio.NewLogger("test"),
		KeepAlive: 20 * time.Millisecond,
	})
	go tun.keepAliveLoop(conn)
	time.Sleep(150 * time.Millisecond)
	select {
	case <-conn.closed:
		t.Fatal("keepAliveLoop closed a healthy connection")
	default:
	}
	if n := conn.pings.Load(); n < 2 {
		t.Fatalf("expected at least 2 pings, got %d", n)
	}
	conn.Close()
}
