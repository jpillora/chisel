package e2e_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

//freezableProxy is a TCP proxy that can silently blackhole its current
//connections — data is discarded without closing them, so neither end
//receives a RST or FIN. This simulates a dead path after OS sleep/wake
//or a NAT timeout. Connections made after FreezeExisting are unaffected,
//so reconnect attempts always succeed.
type freezableProxy struct {
	listener net.Listener
	target   string
	mut      sync.Mutex
	frozen   []*atomic.Bool //one flag per active connection pair
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

func (p *freezableProxy) Close() { p.listener.Close() }

//FreezeExisting blackholes all current connections.
func (p *freezableProxy) FreezeExisting() {
	p.mut.Lock()
	defer p.mut.Unlock()
	for _, f := range p.frozen {
		f.Store(true)
	}
	p.frozen = nil
}

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
		frozen := &atomic.Bool{}
		p.mut.Lock()
		p.frozen = append(p.frozen, frozen)
		p.mut.Unlock()
		go p.pipe(src, dst, frozen)
		go p.pipe(dst, src, frozen)
	}
}

func (p *freezableProxy) pipe(dst, src net.Conn, frozen *atomic.Bool) {
	defer src.Close()
	defer dst.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if err != nil {
			return
		}
		if frozen.Load() {
			continue //discard: the connection is a black hole
		}
		if _, err := dst.Write(buf[:n]); err != nil {
			return
		}
	}
}

//TestKeepAliveReconnect verifies that when the network path goes silent
//(packets dropped without RST/FIN, as after OS sleep/wake) the keepalive
//ping timeout closes the dead connection and the client reconnects,
//restoring port forwarding. Without the ping timeout, SendRequest blocks
//until the kernel retransmit timeout (15+ minutes) and the tunnel hangs.
func TestKeepAliveReconnect(t *testing.T) {
	const keepalive = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	//fileserver (the tunnelled endpoint)
	filePort := availablePort()
	fileAddr := "127.0.0.1:" + filePort
	f := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			w.Write(append(b, '!'))
		}),
	}
	fl, err := net.Listen("tcp", fileAddr)
	if err != nil {
		t.Fatal(err)
	}
	go f.Serve(fl)
	defer f.Close()
	//chisel server
	server, err := chserver.NewServer(&chserver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	server.Debug = debug
	serverPort := availablePort()
	if err := server.StartContext(ctx, "127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		server.Wait()
	}()
	//freezable proxy between client and server
	proxy, err := newFreezableProxy("127.0.0.1:" + serverPort)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	//chisel client connects via the proxy so the link can be killed
	tunnelPort := availablePort()
	client, err := chclient.NewClient(&chclient.Config{
		Fingerprint:      server.GetFingerprint(),
		Server:           "http://" + proxy.Addr(),
		Remotes:          []string{tunnelPort + ":" + fileAddr},
		KeepAlive:        keepalive,
		MaxRetryCount:    -1,
		MaxRetryInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.Debug = debug
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		client.Wait()
	}()
	//tunnel must work before the link dies
	url := "http://localhost:" + tunnelPort
	if err := waitPost(url, "hello", "hello!", 5*time.Second); err != nil {
		t.Fatalf("pre-freeze: %v", err)
	}
	//silently kill the established connection — the in-flight ping can
	//never be answered, so only the ping timeout can detect the failure
	proxy.FreezeExisting()
	//keepalive should close the dead connection and reconnect
	if err := waitPost(url, "world", "world!", 10*time.Second); err != nil {
		t.Fatalf("tunnel did not recover after dead link: %v", err)
	}
}

//waitPost polls the tunnel until it returns the expected response.
func waitPost(url, body, want string, timeout time.Duration) error {
	c := http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for {
		var got string
		resp, err := c.Post(url, "text/plain", strings.NewReader(body))
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			got = string(b)
			if got == want {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("timed out: %w", err)
			}
			return fmt.Errorf("timed out: got %q, want %q", got, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
