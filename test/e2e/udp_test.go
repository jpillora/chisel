package e2e_test

import (
	"log"
	"net"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"golang.org/x/sync/errgroup"
)

func TestUDP(t *testing.T) {
	//listen on random udp port
	echoPort := availableUDPPort()
	a, _ := net.ResolveUDPAddr("udp", ":"+echoPort)
	l, err := net.ListenUDP("udp", a)
	if err != nil {
		t.Fatal(err)
	}
	//chisel client+server
	inboundPort := availableUDPPort()
	teardown := simpleSetup(t,
		&chserver.Config{},
		&chclient.Config{
			Remotes: []string{
				inboundPort + ":" + echoPort + "/udp",
			},
		},
	)
	defer teardown()
	//fake udp server, read and echo back duplicated, close
	eg := errgroup.Group{}
	eg.Go(func() error {
		defer l.Close()
		b := make([]byte, 128)
		n, a, err := l.ReadFrom(b)
		if err != nil {
			return err
		}
		if _, err = l.WriteTo(append(b[:n], b[:n]...), a); err != nil {
			return err
		}
		return nil
	})
	//fake udp client
	conn, err := net.Dial("udp4", "localhost:"+inboundPort)
	if err != nil {
		t.Fatal(err)
	}
	//write bazz through the tunnel
	if _, err := conn.Write([]byte("bazz")); err != nil {
		t.Fatal(err)
	}
	//receive bazzbazz back
	b := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(b)
	if err != nil {
		t.Fatal(err)
		return
	}
	//udp server should close correctly
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
		return
	}
	//ensure expected
	s := string(b[:n])
	if s != "bazzbazz" {
		t.Fatalf("expected double bazz")
	}
}

func availableUDPPort() string {
	a, _ := net.ResolveUDPAddr("udp", ":0")
	l, err := net.ListenUDP("udp", a)
	if err != nil {
		log.Panicf("availability listen: %s", err)
	}
	l.Close()
	_, port, err := net.SplitHostPort(l.LocalAddr().String())
	if err != nil {
		log.Panic(err)
	}
	return port
}
