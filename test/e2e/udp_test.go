package e2e_test

import (
	"net"
	"sync"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

func TestUDP(t *testing.T) {
	teardown := setup(t,
		&chserver.Config{},
		&chclient.Config{
			Remotes: []string{
				"2888:2999/udp",
			},
		},
	)
	defer teardown()
	//listen on 2999 udp
	a, _ := net.ResolveUDPAddr("udp", ":2999")
	l, err := net.ListenUDP("udp", a)
	if err != nil {
		t.Fatal(err)
	}
	//fake udp server, read and echo back twice
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer l.Close()
		b := make([]byte, 128)
		n, a, err := l.ReadFrom(b)
		if _, err = l.WriteTo(append(b[:n], b[:n]...), a); err != nil {
			return
		}
	}()
	//fake udp client
	conn, err := net.Dial("udp", "localhost:2888")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("bazz")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(b)
	if err != nil {
		t.Fatal(err)
		return
	}
	s := string(b[:n])
	if s != "bazzbazz" {
		t.Fatalf("expected double bazz")
	}
}
