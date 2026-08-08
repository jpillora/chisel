package cnet

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestGoServeGracefulDrain verifies that Wait observes the graceful drain,
// while the server refuses new connections.
func TestGoServeGracefulDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := NewHTTPServer()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("done"))
	})
	if err := h.GoServe(ctx, l, handler); err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	//fire an in-flight request, then cancel the server mid-request
	bodyc := make(chan string, 1)
	errc := make(chan error, 1)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			errc <- err
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyc <- string(b)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for request to reach handler")
	}
	cancel()
	waitc := make(chan error, 1)
	go func() {
		waitc <- h.Wait()
	}()

	// Shutdown closes the listener before waiting for active handlers. Retry
	// until that close is observable, because cancellation and Shutdown run in
	// separate goroutines.
	addr := l.Addr().String()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			break
		}
		conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("new requests continued to succeed during shutdown")
		}
	}
	select {
	case err := <-waitc:
		t.Fatalf("Wait returned before in-flight handler finished: %v", err)
	default:
	}
	close(release)
	select {
	case b := <-bodyc:
		if b != "done" {
			t.Fatalf("in-flight response corrupted: %q", b)
		}
	case err := <-errc:
		t.Fatalf("in-flight request failed during shutdown: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for drained response")
	}
	// New connections remain refused after the drain finishes.
	if _, err := http.Get(url); err == nil {
		t.Fatal("new request succeeded after shutdown")
	}
	if err := <-waitc; err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestGoServeShutdownGraceTimeout(t *testing.T) {
	const grace = 100 * time.Millisecond
	t.Setenv("CHISEL_SHUTDOWN_GRACE", grace.String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := NewHTTPServer()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})
	if err := h.GoServe(ctx, l, handler); err != nil {
		t.Fatal(err)
	}

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, err := http.Get("http://" + l.Addr().String())
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for request to reach handler")
	}

	start := time.Now()
	cancel()
	if err := h.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < grace {
		t.Fatalf("Wait returned before shutdown grace expired: %v", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Wait exceeded shutdown grace by too much: %v", elapsed)
	}
	select {
	case <-requestDone:
	case <-time.After(3 * time.Second):
		t.Fatal("force-close did not unblock the active request")
	}
}
