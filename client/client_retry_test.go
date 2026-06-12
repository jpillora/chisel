package chclient

import (
	"context"
	"strings"
	"testing"
	"time"
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
	//port 1 is reserved/unbound: connection refused immediately
	c, err := NewClient(&Config{
		Server:        "http://127.0.0.1:1",
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
