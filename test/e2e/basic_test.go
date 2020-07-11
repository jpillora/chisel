package e2e_test

import (
	"net"
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

func TestBasic(t *testing.T) {
	//setup server, client, fileserver
	teardown := setup(t,
		&chserver.Config{},
		&chclient.Config{
			Remotes: []string{
				"3500:" + filePort,
			},
		})
	defer teardown()
	//test remote
	result, err := post("http://localhost:3500", "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
}

func TestAuth(t *testing.T) {
	//setup server, client, fileserver
	teardown := setup(t,
		&chserver.Config{
			KeySeed: "foobar",
			Auth:    "../bench/userfile",
		},
		&chclient.Config{
			Remotes: []string{
				"3500:" + filePort,
				"3501:" + fileAddr,
			},
			Auth: "foo:bar",
		})
	defer teardown()
	//test first remote
	result, err := post("http://localhost:3500", "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
	//test second remote
	result, err = post("http://localhost:3501", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if result != "bar!" {
		t.Fatalf("expected exclamation mark added again")
	}
}

func TestUDP(t *testing.T) {
	teardown := setup(
		t,
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
	go func() {
		defer l.Close()
		b := make([]byte, 128)
		n, a, err := l.ReadFrom(b)
		if _, err = l.WriteTo(append(b[:n], b[:n]...), a); err != nil {
			return
		}
		if _, err = l.WriteTo(b, a); err != nil {
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
