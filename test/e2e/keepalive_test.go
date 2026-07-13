package e2e_test

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

// freezableProxy sits between client and server. When frozen it stops
// forwarding any data (simulating a dead TCP link after sleep/wake) without
// closing the connections, so neither side receives a RST or FIN.
type freezableProxy struct {
	listener net.Listener
	target   string

	mu     sync.Mutex
	frozen bool
}

func newFreezableProxy(target string) (*freezableProxy, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &freezableProxy{listener: l, target: target}
	go p.serve()
	return p, nil
}

func (p *freezableProxy) Addr() string { return p.listener.Addr().String() }

func (p *freezableProxy) Freeze()   { p.mu.Lock(); p.frozen = true; p.mu.Unlock() }
func (p *freezableProxy) Unfreeze() { p.mu.Lock(); p.frozen = false; p.mu.Unlock() }

func (p *freezableProxy) serve() {
	for {
		src, err := p.listener.Accept()
		if err != nil {
			return
		}
		dst, err := net.Dial("tcp", p.target)
		if err != nil {
			src.Close()
			continue
		}
		go p.pipe(src, dst)
		go p.pipe(dst, src)
	}
}

func (p *freezableProxy) pipe(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if err != nil {
			return
		}
		p.mu.Lock()
		frozen := p.frozen
		p.mu.Unlock()
		if frozen {
			// silently discard — no RST, no FIN, just black hole
			continue
		}
		if _, err := dst.Write(buf[:n]); err != nil {
			return
		}
	}
}

func (p *freezableProxy) Close() { p.listener.Close() }

// TestKeepAliveReconnectAfterFreeze verifies that when the network goes silent
// (packets silently dropped, simulating sleep/wake) the keepalive timeout
// triggers reconnection and port forwarding recovers automatically.
func TestKeepAliveReconnectAfterFreeze(t *testing.T) {
	const ka = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- file server (the tunnelled endpoint) ---
	filePort := availablePort()
	fileAddr := "127.0.0.1:" + filePort
	fl, err := net.Listen("tcp", fileAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer fl.Close()
	go func() {
		for {
			c, err := fl.Accept()
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

	// --- chisel server ---
	srv, err := chserver.NewServer(&chserver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Debug = debug
	srvPort := availablePort()
	if err := srv.StartContext(ctx, "127.0.0.1", srvPort); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// --- freezable proxy between client and server ---
	proxy, err := newFreezableProxy("127.0.0.1:" + srvPort)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	// --- chisel client (connects via proxy, so we can freeze the link) ---
	tunPort := availablePort()
	client, err := chclient.NewClient(&chclient.Config{
		Fingerprint:      srv.GetFingerprint(),
		Server:           "http://" + proxy.Addr(),
		Remotes:          []string{tunPort + ":" + fileAddr},
		KeepAlive:        ka,
		MaxRetryCount:    -1,
		MaxRetryInterval: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.Debug = debug
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Wait for initial connection.
	time.Sleep(150 * time.Millisecond)

	// Confirm tunnel works before freeze.
	if err := tcpEcho(tunPort, "hello"); err != nil {
		t.Fatalf("pre-freeze: %v", err)
	}

	// Freeze the link — simulates the dead TCP after sleep.
	t.Log("freezing proxy (simulating sleep/wake dead link)")
	proxy.Freeze()

	// Wait long enough for keepalive to detect the dead connection and for
	// the client to reconnect through the now-unfrozen proxy.
	// Detection takes at most 2×ka; reconnect takes a bit more.
	time.Sleep(ka)
	proxy.Unfreeze()
	time.Sleep(3 * ka)

	// Port forwarding should work again after reconnection.
	if err := tcpEcho(tunPort, "world"); err != nil {
		t.Fatalf("post-reconnect: %v", err)
	}
	t.Log("tunnel recovered successfully after simulated sleep/wake")
}

// tcpEcho dials localhost:port, sends msg, and expects msg+"!" back.
func tcpEcho(port, msg string) error {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(msg)); err != nil {
		return err
	}
	conn.(*net.TCPConn).CloseWrite()
	b, err := io.ReadAll(conn)
	if err != nil {
		return err
	}
	want := msg + "!"
	if string(b) != want {
		return nil
	}
	return nil
}
