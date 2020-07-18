package chclient

import (
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestCustomHeaders(t *testing.T) {
	//fake server
	wg := sync.WaitGroup{}
	wg.Add(1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Foo") != "Bar" {
			t.Fatal("expected header Foo to be 'Bar'")
		}
		wg.Done()
	}))
	defer server.Close()
	//client
	headers := http.Header{}
	headers.Set("Foo", "Bar")
	config := Config{
		KeepAlive:        time.Second,
		MaxRetryInterval: time.Second,
		Server:           server.URL,
		Remotes:          []string{"9000"},
		Headers:          headers,
	}
	c, err := NewClient(&config)
	if err != nil {
		log.Fatal(err)
	}
	go c.Run()
	//wait for test to complete
	wg.Wait()
	c.Close()
}
