package chclient

import (
	"net/url"
	"testing"

	"github.com/gorilla/websocket"
)

func TestSetProxySchemes(t *testing.T) {
	//all socks variants use the same SOCKS5 dialer
	for _, scheme := range []string{"socks", "socks5", "socks5h"} {
		u, err := url.Parse(scheme + "://127.0.0.1:1080")
		if err != nil {
			t.Fatal(err)
		}
		d := &websocket.Dialer{}
		if err := (&Client{}).setProxy(u, d); err != nil {
			t.Fatalf("%s:// rejected: %v", scheme, err)
		}
		if d.NetDial == nil {
			t.Fatalf("%s:// did not install the socks dialer", scheme)
		}
	}
	//http CONNECT proxies pass through
	u, _ := url.Parse("http://127.0.0.1:8080")
	d := &websocket.Dialer{}
	if err := (&Client{}).setProxy(u, d); err != nil {
		t.Fatal(err)
	}
	if d.Proxy == nil {
		t.Fatal("http:// did not set the CONNECT proxy")
	}
	//unknown socks variants are rejected
	u, _ = url.Parse("socks4://127.0.0.1:1080")
	if err := (&Client{}).setProxy(u, &websocket.Dialer{}); err == nil {
		t.Fatal("socks4:// unexpectedly accepted")
	}
}
