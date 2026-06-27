package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/settings"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// dialChiselSSH connects to the chisel server via websocket and
// performs an SSH handshake as the given user.
func dialChiselSSH(t *testing.T, serverAddr, user, pass string) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request) {
	t.Helper()
	ws, _, err := (&websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"chisel-v3"},
	}).Dial("ws://"+serverAddr, http.Header{})
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	conn := cnet.NewWebSocketConn(ws)
	sc, chans, reqs, err := ssh.NewClientConn(conn, "", &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("ssh handshake: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	go func() { for c := range chans { c.Reject(ssh.Prohibited, "") } }()
	return sc, chans, reqs
}

// sendConfig sends the chisel config request with the given remotes.
func sendConfig(t *testing.T, sc ssh.Conn, remotes []*settings.Remote) {
	t.Helper()
	cfg, err := json.Marshal(settings.Config{Version: "0", Remotes: remotes})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	ok, reply, err := sc.SendRequest("config", true, cfg)
	if err != nil {
		t.Fatalf("config request: %v", err)
	}
	if !ok {
		t.Fatalf("config rejected: %s", reply)
	}
}

// TestAuthChannelDenied verifies that a channel to an unauthorized
// destination is rejected.
func TestAuthChannelDenied(t *testing.T) {
	allowedPort := availablePort()
	blockedPort := availablePort()

	blockedListener, err := net.Listen("tcp", "127.0.0.1:"+blockedPort)
	if err != nil {
		t.Fatal(err)
	}
	defer blockedListener.Close()
	go func() {
		for {
			conn, err := blockedListener.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte("FORBIDDEN"))
			conn.Close()
		}
	}()

	// Start chisel server with ACL: user can only reach allowedPort
	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "acl-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	if err := s.AddUser("user", "pass", fmt.Sprintf(`^127\.0\.0\.1:%s$`, allowedPort)); err != nil {
		t.Fatal(err)
	}
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	serverAddr := "127.0.0.1:" + serverPort

	// Connect and send config with only the allowed remote
	sc, _, _ := dialChiselSSH(t, serverAddr, "user", "pass")
	defer sc.Close()

	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", allowedPort, allowedPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})

	// Try to open a channel to the BLOCKED port — must be rejected
	target := net.JoinHostPort("127.0.0.1", blockedPort)
	ch, _, err := sc.OpenChannel("chisel", []byte(target))
	if err == nil {
		ch.Close()
		t.Fatalf("channel to blocked port %s was accepted", blockedPort)
	}
	t.Logf("channel to blocked port correctly rejected: %v", err)
}

// TestAuthChannelAllowed verifies that a channel to an authorized
// destination is accepted.
func TestAuthChannelAllowed(t *testing.T) {
	allowedPort := availablePort()

	// Start a TCP listener on the allowed port
	allowedListener, err := net.Listen("tcp", "127.0.0.1:"+allowedPort)
	if err != nil {
		t.Fatal(err)
	}
	defer allowedListener.Close()
	go func() {
		for {
			conn, err := allowedListener.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte("ALLOWED"))
			conn.Close()
		}
	}()

	// Start chisel server with ACL: user can only reach allowedPort
	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "acl-test-allowed",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	if err := s.AddUser("user", "pass", fmt.Sprintf(`^127\.0\.0\.1:%s$`, allowedPort)); err != nil {
		t.Fatal(err)
	}
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	serverAddr := "127.0.0.1:" + serverPort

	// Connect and send config with the allowed remote
	sc, _, _ := dialChiselSSH(t, serverAddr, "user", "pass")
	defer sc.Close()

	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", allowedPort, allowedPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})

	// Open channel to the allowed port — must succeed
	target := net.JoinHostPort("127.0.0.1", allowedPort)
	ch, reqs, err := sc.OpenChannel("chisel", []byte(target))
	if err != nil {
		t.Fatalf("channel to allowed port %s was rejected: %v", allowedPort, err)
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	// Read data from the allowed target
	buf := make([]byte, 64)
	n, err := ch.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read from allowed channel: %v", err)
	}
	if string(buf[:n]) != "ALLOWED" {
		t.Fatalf("expected 'ALLOWED', got %q", buf[:n])
	}
	t.Logf("channel to allowed port works correctly, received: %s", buf[:n])
}

// TestNoAuthChannel verifies that when no auth is configured,
// all destinations are reachable.
func TestNoAuthChannel(t *testing.T) {
	targetPort := availablePort()

	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:"+targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte("OPEN"))
			conn.Close()
		}
	}()

	// Start chisel server with NO auth
	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "no-acl-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	serverAddr := "127.0.0.1:" + serverPort

	// Connect with any credentials (server accepts all when no auth configured)
	sc, _, _ := dialChiselSSH(t, serverAddr, "anyone", "anything")
	defer sc.Close()

	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", targetPort, targetPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})

	// Open channel — should be accepted since no ACL
	target := net.JoinHostPort("127.0.0.1", targetPort)
	ch, creqs, err := sc.OpenChannel("chisel", []byte(target))
	if err != nil {
		t.Fatalf("channel rejected when no ACL is configured: %v", err)
	}
	go ssh.DiscardRequests(creqs)
	defer ch.Close()

	buf := make([]byte, 64)
	n, err := ch.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "OPEN" {
		t.Fatalf("expected 'OPEN', got %q", buf[:n])
	}
	t.Logf("no-ACL mode works correctly")
}

// TestAuthWildcardChannel verifies that a user with wildcard access
// can reach any destination.
func TestAuthWildcardChannel(t *testing.T) {
	targetPort := availablePort()

	listener, err := net.Listen("tcp", "127.0.0.1:"+targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte("WILDCARD"))
			conn.Close()
		}
	}()

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "acl-wildcard-test",
		Auth:    "admin:secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+serverPort, "admin", "secret")
	defer sc.Close()

	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", targetPort, targetPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})

	target := net.JoinHostPort("127.0.0.1", targetPort)
	ch, reqs, err := sc.OpenChannel("chisel", []byte(target))
	if err != nil {
		t.Fatalf("wildcard user channel rejected: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	buf := make([]byte, 64)
	n, err := ch.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "WILDCARD" {
		t.Fatalf("expected 'WILDCARD', got %q", buf[:n])
	}
	t.Logf("wildcard user correctly allowed")
}

// TestAuthSocksChannelDenied verifies that a user who is NOT granted socks
// cannot open a socks channel, even when the server runs with --socks5.
// socks channels are authorized against the per-user ACL like any other
// destination (the channel token is the literal "socks").
func TestAuthSocksChannelDenied(t *testing.T) {
	allowedPort := availablePort()

	// SOCKS5 is enabled, so any rejection can only come from the ACL,
	// not from the "SOCKS5 is not enabled" guard.
	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "acl-socks-denied",
		Socks5:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	// user is pinned to a single TCP destination; socks is NOT granted
	if err := s.AddUser("user", "pass", fmt.Sprintf(`^127\.0\.0\.1:%s$`, allowedPort)); err != nil {
		t.Fatal(err)
	}
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// declare ONLY the allowed remote so the config handshake passes
	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+serverPort, "user", "pass")
	defer sc.Close()
	r, err := settings.DecodeRemote(fmt.Sprintf("0.0.0.0:%s:127.0.0.1:%s", allowedPort, allowedPort))
	if err != nil {
		t.Fatal(err)
	}
	sendConfig(t, sc, []*settings.Remote{r})

	// open a raw channel whose ExtraData is "socks" — must be denied by the ACL
	ch, _, err := sc.OpenChannel("chisel", []byte("socks"))
	if err == nil {
		ch.Close()
		t.Fatal("socks channel was accepted for a user without socks access")
	}
	// it must be the ACL talking, not "SOCKS5 is not enabled"
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("socks channel rejected for the wrong reason: %v", err)
	}
	t.Logf("socks channel correctly rejected by ACL: %v", err)
}

// TestAuthSocksChannelAllowed verifies the ACL does not over-block: a user
// explicitly granted socks (a regex matching the "socks" channel token) can
// open a socks channel and reach a destination through the SOCKS5 proxy.
func TestAuthSocksChannelAllowed(t *testing.T) {
	targetPort := availablePort()

	// a destination to CONNECT to through socks
	ln, err := net.Listen("tcp", "127.0.0.1:"+targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte("VIA-SOCKS"))
			conn.Close()
		}
	}()

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "acl-socks-allowed",
		Socks5:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	// grant socks at the channel level (the channel token is the literal "socks")
	if err := s.AddUser("user", "pass", `^socks$`); err != nil {
		t.Fatal(err)
	}
	serverPort := availablePort()
	if err := s.Start("127.0.0.1", serverPort); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+serverPort, "user", "pass")
	defer sc.Close()
	// no remotes to declare; the socks channel is opened directly
	sendConfig(t, sc, []*settings.Remote{})

	ch, reqs, err := sc.OpenChannel("chisel", []byte("socks"))
	if err != nil {
		t.Fatalf("socks channel rejected for a user granted socks: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	// drive a minimal SOCKS5 CONNECT to the allowed local listener
	if _, err := ch.Write([]byte{0x05, 0x01, 0x00}); err != nil { // greeting: no-auth
		t.Fatal(err)
	}
	if _, err := io.ReadFull(ch, make([]byte, 2)); err != nil { // method selection
		t.Fatal(err)
	}
	port, err := strconv.Atoi(targetPort)
	if err != nil {
		t.Fatal(err)
	}
	connect := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, byte(port >> 8), byte(port)}
	if _, err := ch.Write(connect); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(ch, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("socks CONNECT failed, rep=%d", reply[1])
	}
	buf := make([]byte, 16)
	n, err := ch.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read via socks: %v", err)
	}
	if string(buf[:n]) != "VIA-SOCKS" {
		t.Fatalf("expected 'VIA-SOCKS' via socks, got %q", buf[:n])
	}
	t.Logf("socks channel allowed and functional for a granted user")
}
