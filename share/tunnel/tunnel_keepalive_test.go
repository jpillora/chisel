package tunnel

import (
	"net"
	"testing"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"golang.org/x/crypto/ssh"
)

func newTestTunnel(ka time.Duration) *Tunnel {
	return New(Config{
		Logger:    cio.NewLogger("test"),
		KeepAlive: ka,
	})
}

// mockDeadSSHConn simulates an ssh.Conn whose SendRequest blocks indefinitely,
// as happens when the underlying TCP connection is dead but the OS has not yet
// detected it (e.g. immediately after a sleep/wake cycle with no RST received).
type mockDeadSSHConn struct {
	closed chan struct{}
}

func (m *mockDeadSSHConn) User() string            { return "" }
func (m *mockDeadSSHConn) SessionID() []byte        { return nil }
func (m *mockDeadSSHConn) ClientVersion() []byte    { return nil }
func (m *mockDeadSSHConn) ServerVersion() []byte    { return nil }
func (m *mockDeadSSHConn) RemoteAddr() net.Addr     { return &net.TCPAddr{} }
func (m *mockDeadSSHConn) LocalAddr() net.Addr      { return &net.TCPAddr{} }
func (m *mockDeadSSHConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, net.ErrClosed
}
func (m *mockDeadSSHConn) Wait() error { <-m.closed; return nil }
func (m *mockDeadSSHConn) Close() error {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
	return nil
}

// SendRequest blocks until Close() is called, simulating a dead TCP connection.
func (m *mockDeadSSHConn) SendRequest(_ string, _ bool, _ []byte) (bool, []byte, error) {
	<-m.closed
	return false, nil, net.ErrClosed
}

// TestKeepAliveLoopTimeout verifies that keepAliveLoop calls sshConn.Close()
// when SendRequest does not return within the keepalive interval. This is the
// sleep/wake scenario where the TCP connection is silently dead.
func TestKeepAliveLoopTimeout(t *testing.T) {
	const ka = 50 * time.Millisecond

	mock := &mockDeadSSHConn{closed: make(chan struct{})}
	tun := newTestTunnel(ka)

	go tun.keepAliveLoop(mock)

	select {
	case <-mock.closed:
		// keepAliveLoop detected the dead connection and called sshConn.Close()
	case <-time.After(5 * ka):
		t.Fatal("keepAliveLoop did not close dead connection within timeout (2×keepalive)")
	}
}

// mockHealthySSHConn simulates a normal ssh.Conn that responds to pings immediately.
type mockHealthySSHConn struct {
	closed    chan struct{}
	pingCount int
}

func (m *mockHealthySSHConn) User() string            { return "" }
func (m *mockHealthySSHConn) SessionID() []byte        { return nil }
func (m *mockHealthySSHConn) ClientVersion() []byte    { return nil }
func (m *mockHealthySSHConn) ServerVersion() []byte    { return nil }
func (m *mockHealthySSHConn) RemoteAddr() net.Addr     { return &net.TCPAddr{} }
func (m *mockHealthySSHConn) LocalAddr() net.Addr      { return &net.TCPAddr{} }
func (m *mockHealthySSHConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, net.ErrClosed
}
func (m *mockHealthySSHConn) Wait() error { <-m.closed; return nil }
func (m *mockHealthySSHConn) Close() error {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
	return nil
}
func (m *mockHealthySSHConn) SendRequest(_ string, _ bool, _ []byte) (bool, []byte, error) {
	select {
	case <-m.closed:
		return false, nil, net.ErrClosed
	default:
		m.pingCount++
		return true, []byte("pong"), nil
	}
}

// TestKeepAliveLoopHealthy verifies that keepAliveLoop does NOT close the
// connection when the remote responds to pings normally.
func TestKeepAliveLoopHealthy(t *testing.T) {
	const ka = 30 * time.Millisecond

	mock := &mockHealthySSHConn{closed: make(chan struct{})}
	tun := newTestTunnel(ka)

	go tun.keepAliveLoop(mock)

	// Let a few ping cycles pass.
	time.Sleep(4 * ka)

	select {
	case <-mock.closed:
		t.Fatal("keepAliveLoop closed a healthy connection unexpectedly")
	default:
		if mock.pingCount < 2 {
			t.Fatalf("expected at least 2 pings, got %d", mock.pingCount)
		}
	}

	mock.Close() // clean up
}
