package chclient

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	chserver "github.com/jpillora/chisel/server"
)

func TestRetryIntervalDefaults(t *testing.T) {
	c, err := NewClient(&Config{Server: "http://localhost:0"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.config.MinRetryInterval; got != time.Second {
		t.Fatalf("default MinRetryInterval = %s, want 1s", got)
	}
	if got := c.config.MaxRetryInterval; got != 5*time.Minute {
		t.Fatalf("default MaxRetryInterval = %s, want 5m", got)
	}
}

func TestRetryIntervalExplicit(t *testing.T) {
	//explicit values are honored, including sub-second ones
	c, err := NewClient(&Config{
		Server:           "http://localhost:0",
		MinRetryInterval: 200 * time.Millisecond,
		MaxRetryInterval: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.config.MinRetryInterval; got != 200*time.Millisecond {
		t.Fatalf("MinRetryInterval = %s, want 200ms", got)
	}
	if got := c.config.MaxRetryInterval; got != 2*time.Second {
		t.Fatalf("MaxRetryInterval = %s, want 2s", got)
	}
	//max below min is raised to min
	c2, err := NewClient(&Config{
		Server:           "http://localhost:0",
		MinRetryInterval: 5 * time.Second,
		MaxRetryInterval: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c2.config.MaxRetryInterval; got != 5*time.Second {
		t.Fatalf("inverted MaxRetryInterval = %s, want raised to 5s", got)
	}
}

// TestGiveUpReturnsError verifies that exhausting --max-retry-count
// surfaces an error (non-zero process exit) instead of nil.
func TestGiveUpReturnsError(t *testing.T) {
	dialErr := errors.New("test dial failure")
	c, err := NewClient(&Config{
		Server: "http://example.invalid",
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, dialErr
		},
		MaxRetryCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	err = c.Wait()
	if err == nil {
		t.Fatal("expected an error after exhausting connection attempts")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancelledContextWinsOverRetryExhaustion(t *testing.T) {
	var dials atomic.Int32
	c, err := NewClient(&Config{
		Server: "http://example.invalid",
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("unexpected dial")
		},
		MaxRetryCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.connectionLoop(ctx); err != nil {
		t.Fatalf("cancelled connection loop returned an error: %v", err)
	}
	if got := dials.Load(); got != 0 {
		t.Fatalf("cancelled connection loop made %d dials, want 0", got)
	}
}

func TestCancellationAfterConnectionWinsOverRetryExhaustion(t *testing.T) {
	serverCtx, stopServer := context.WithCancel(context.Background())
	server, err := chserver.NewServer(&chserver.Config{KeySeed: "retry-cancel-test"})
	if err != nil {
		t.Fatal(err)
	}
	port := availableTCPPort(t)
	if err := server.StartContext(serverCtx, "127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopServer()
		if err := server.Wait(); err != nil {
			t.Errorf("server shutdown: %v", err)
		}
	}()

	clientCtx, stopClient := context.WithCancel(context.Background())
	c, err := NewClient(&Config{
		Fingerprint:   server.GetFingerprint(),
		Server:        "http://127.0.0.1:" + port,
		MaxRetryCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(clientCtx); err != nil {
		t.Fatal(err)
	}
	defer stopClient()

	readyCtx, stopReady := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopReady()
	if !c.Ready(readyCtx) {
		t.Fatal("client did not establish a connection")
	}
	stopClient()
	if err := c.Wait(); err != nil {
		t.Fatalf("cancelled connected client returned an error: %v", err)
	}
}

func availableTCPPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(port)
}
