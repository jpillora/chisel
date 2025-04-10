package chclient

import (
	"crypto/elliptic"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jpillora/chisel/share/ccrypto"
	"golang.org/x/crypto/ssh"
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

func TestFallbackLegacyFingerprint(t *testing.T) {
	config := Config{
		Fingerprint: "a5:32:92:c6:56:7a:9e:61:26:74:1b:81:a6:f5:1b:44",
		Server:      "http://localhost",
	}
	c, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}
	r := ccrypto.NewDetermRand([]byte("test123"))
	priv, err := ccrypto.GenerateKeyGo119(elliptic.P256(), r)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	err = c.verifyServer("", nil, pub)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVerifyLegacyFingerprint(t *testing.T) {
	config := Config{
		Fingerprint: "a5:32:92:c6:56:7a:9e:61:26:74:1b:81:a6:f5:1b:44",
		Server:      "http://localhost",
	}
	c, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}
	r := ccrypto.NewDetermRand([]byte("test123"))
	priv, err := ccrypto.GenerateKeyGo119(elliptic.P256(), r)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	err = c.verifyLegacyFingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVerifyFingerprint(t *testing.T) {
	config := Config{
		Fingerprint: "qmrRoo8MIqePv3jC8+wv49gU6uaFgD3FASQx9V8KdmY=",
		Server:      "http://localhost",
	}
	c, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}
	r := ccrypto.NewDetermRand([]byte("test123"))
	priv, err := ccrypto.GenerateKeyGo119(elliptic.P256(), r)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	err = c.verifyServer("", nil, pub)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPermittedDomain(t *testing.T) {
	permittedDomains := []string{
		"http://something.slb.com",
		"http://cluster.local",
		"http://something-service",
		"http://localhost",
		"http://127.0.0.1",
		"https://slb-ds.com",
		"https://something.slb.com",
		"https://cluster.local",
		"https://something-service",
		"https://localhost",
		"https://thing.slb.com:3456",
		"https://thing.slb.com:80",
		"https://thing.slb.com:443",
		"https://thing.slb.com:8080",
	}

	for _, domain := range permittedDomains {
		t.Run(domain, func(t *testing.T) {
			config := Config{
				Server: domain,
			}
			_, err := NewClient(&config)
			if err != nil {
				t.Fatalf("Failed for allowed domain %s: %v", domain, err)
			}
		})
	}
}
func TestBannedDomain(t *testing.T) {
	bannedDomains := []string{
		"http://banned.com",
		"http://something.banned.com",
		"https://localheist",
		"https://nop.nope.com:6788",
	}

	for _, domain := range bannedDomains {
		t.Run(domain, func(t *testing.T) {
			config := Config{
				Server: domain,
			}
			_, err := NewClient(&config)
			if err == nil {
				t.Fatalf("Failed for banned domain %s: %v", domain, err)
			}
		})
	}
}
