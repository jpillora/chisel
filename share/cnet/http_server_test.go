package cnet

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestGoServeGracefulDrain verifies that cancelling the GoServe context
// drains in-flight requests instead of resetting them, while refusing
// new connections.
func TestGoServeGracefulDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := NewHTTPServer()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Write([]byte("done"))
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
	time.Sleep(100 * time.Millisecond) //let the request reach the handler
	cancel()
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
	//new connections must be refused after shutdown
	if _, err := http.Get(url); err == nil {
		t.Fatal("new request succeeded after shutdown")
	}
	if err := h.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}
