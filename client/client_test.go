package chclient

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCustomHeaders(t *testing.T) {
	assert := assert.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(req.Header.Get("Foo"), "Bar")
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
		HTTPProxy:        "",
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
