package e2e_test

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/settings"

	"github.com/gorilla/websocket"
)

// TestPreAuthOversizedMessage verifies that an unauthenticated peer
// sending a huge websocket message gets disconnected by the read limit
// instead of the server buffering the message into memory.
func TestPreAuthOversizedMessage(t *testing.T) {
	t.Setenv("CHISEL_WS_READ_LIMIT", "")
	s, err := chserver.NewServer(&chserver.Config{KeySeed: "ws-limit-test"})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	port := availablePort()
	if err := s.Start("127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	//raw websocket connection with the chisel subprotocol, no ssh
	ws, _, err := (&websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"chisel-v3"},
	}).Dial("ws://127.0.0.1:"+port, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Drain the server's SSH identification. Without this, a read after the
	// oversized write can consume the already-buffered banner and falsely look
	// like the connection survived.
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	messageType, identification, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read SSH identification: %v", err)
	}
	if messageType != websocket.BinaryMessage || !bytes.HasPrefix(identification, []byte("SSH-")) {
		t.Fatalf("unexpected SSH identification message: type=%d payload=%q", messageType, identification)
	}

	// Send one byte above the default before authenticating. The server should
	// close or reset the connection as soon as the read limit is exceeded.
	oversized := bytes.Repeat([]byte("A"), 512*1024+1)
	if err := ws.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, oversized); err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			t.Fatalf("timed out sending oversized pre-auth message: %v", err)
		}
		return // close/reset while writing: limit enforced
	}
	// Otherwise the next read must fail with a close/reset, not a timeout.
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err = ws.ReadMessage()
	if err == nil {
		t.Fatal("received a message after oversized pre-auth message")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("connection still alive 5s after oversized pre-auth message")
	}
}

// TestLargeConfigMessage verifies that a real SSH config request above the old
// 64 KiB limit traverses the default WebSocket envelope successfully.
func TestLargeConfigMessage(t *testing.T) {
	t.Setenv("CHISEL_WS_READ_LIMIT", "")
	s, err := chserver.NewServer(&chserver.Config{KeySeed: "large-config-test"})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	port := availablePort()
	if err := s.Start("127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sc, _, _ := dialChiselSSH(t, "127.0.0.1:"+port, "user", "pass")
	defer sc.Close()
	cfg, err := json.Marshal(settings.Config{
		// The development prefix suppresses version-mismatch logging while the
		// remainder makes the actual SSH request larger than the former cap.
		Version: "0.0.0-" + strings.Repeat("x", 96*1024),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg) <= 64*1024 || len(cfg) >= 256*1024 {
		t.Fatalf("config payload size %d is outside the intended SSH envelope", len(cfg))
	}
	ok, reply, err := sc.SendRequest("config", true, cfg)
	if err != nil {
		t.Fatalf("send large config request: %v", err)
	}
	if !ok {
		t.Fatalf("large config rejected: %s", reply)
	}
}
