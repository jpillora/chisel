package chclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha512"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/valkyrie-io/connector-tunnel/common/crypto"
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

const DetermRandIter = 2048

func NewDetermRand(seed []byte) io.Reader {
	var out []byte
	//strengthen seed
	var next = seed
	for i := 0; i < DetermRandIter; i++ {
		next, out = hash(next)
	}
	return &determRand{
		next: next,
		out:  out,
	}
}

type determRand struct {
	next, out []byte
}

func (d *determRand) Read(b []byte) (int, error) {
	n := 0
	l := len(b)
	for n < l {
		next, out := hash(d.next)
		n += copy(b[n:], out)
		d.next = next
	}
	return n, nil
}

func hash(input []byte) (next []byte, output []byte) {
	nextout := sha512.Sum512(input)
	return nextout[:sha512.Size/2], nextout[sha512.Size/2:]
}

func TestVerifyFingerprint(t *testing.T) {
	rand := NewDetermRand([]byte("test123"))
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand)
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	fp := crypto.FPKey(pub)

	config := Config{
		Fingerprint: fp,
	}

	c, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	err = c.verifyServer("", nil, pub)
	if err != nil {
		t.Fatal(err)
	}
}
