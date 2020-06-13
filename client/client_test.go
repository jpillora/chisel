package chclient

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Foo") != "Bar" {
			t.Fatal("expected header Foo to be 'Bar'")
		}
	}))
	// Close the server when test finishes
	defer server.Close()
	headers := http.Header{}
	headers.Set("Foo", "Bar")
	config := Config{
		Fingerprint:      "",
		Auth:             "",
		KeepAlive:        time.Second,
		MaxRetryCount:    0,
		MaxRetryInterval: time.Second,
		Server:           server.URL,
		Remotes:          []string{"socks"},
		Headers:          headers,
	}
	c, err := NewClient(&config)
	if err != nil {
		log.Fatal(err)
	}
	if err = c.Run(); err != nil {
		log.Fatal(err)
	}
}
