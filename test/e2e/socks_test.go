package e2e_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

//TODO test: SOCKS-client -> [server -> client SOCKS] -> endpoint (reverse socks)

// TestSocksEndToEnd exercises the full forward-SOCKS5 path for a user granted
// socks via "^socks$":
//
//	http client -> chisel client's local SOCKS5 -> tunnel ->
//	chisel server's SOCKS5 proxy -> endpoint
//
// It covers both ACL checks: the config-time check (UserAddr() == "socks")
// lets the client register its socks remote, and the per-channel check lets
// each tunnelled connection through. A "^socks$"-only user only succeeds when
// both checks agree on the "socks" token.
func TestSocksEndToEnd(t *testing.T) {
	// endpoint reached via the socks proxy: echoes the request body + '!'
	endpoint := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			w.Write(append(b, '!'))
		}),
	}
	endpointPort := availablePort()
	el, err := net.Listen("tcp", "127.0.0.1:"+endpointPort)
	if err != nil {
		t.Fatal(err)
	}
	go endpoint.Serve(el)
	defer endpoint.Close()

	// chisel server: SOCKS5 enabled, user granted ONLY socks
	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "socks-e2e",
		Socks5:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	if err := s.AddUser("user", "pass", `^socks$`); err != nil {
		t.Fatal(err)
	}
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// chisel client: local SOCKS5 listener, authed as the socks-only user
	socksPort := availablePort()
	c, err := chclient.NewClient(&chclient.Config{
		Server:      "http://127.0.0.1:" + serverPort,
		Auth:        "user:pass",
		Fingerprint: s.GetFingerprint(),
		Remotes:     []string{"127.0.0.1:" + socksPort + ":socks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Debug = debug
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// HTTP request through the local SOCKS5 proxy to the endpoint, retrying
	// until the tunnel is established (bounded ~5s).
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:"+socksPort, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		},
	}
	endpointURL := "http://" + net.JoinHostPort("127.0.0.1", endpointPort) + "/"
	var body string
	for i := 0; i < 50; i++ {
		resp, err := httpClient.Post(endpointURL, "text/plain", strings.NewReader("foo"))
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			body = string(b)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if body != "foo!" {
		t.Fatalf("expected \"foo!\" through socks, got %q", body)
	}
	t.Logf("forward socks end-to-end works for a ^socks$ user")
}
