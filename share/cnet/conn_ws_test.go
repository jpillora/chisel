package cnet

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsPair returns reads from the server side of a websocket connection wrapped
// in NewWebSocketConn, plus the raw client side for sending test messages.
func wsPair(t *testing.T, readSize int) (serverSide chan interface{}, client *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	out := make(chan interface{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		conn := NewWebSocketConn(ws)
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, readSize)
		n, err := conn.Read(buf)
		if err != nil {
			out <- err
			return
		}
		out <- append([]byte(nil), buf[:n]...)
	}))
	t.Cleanup(ts.Close)
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return out, ws
}

// TestWSConnReadDefaultAllowsSSHPacketEnvelope verifies that the default has
// room for x/crypto/ssh's 256 KiB packet ceiling plus transport overhead.
func TestWSConnReadDefaultAllowsSSHPacketEnvelope(t *testing.T) {
	t.Setenv("CHISEL_WS_READ_LIMIT", "")
	const sshPacketWithHeadroom = sshMaxTransportPacket + 4*1024
	out, client := wsPair(t, sshPacketWithHeadroom)
	message := bytes.Repeat([]byte("x"), sshPacketWithHeadroom)
	if err := client.WriteMessage(websocket.BinaryMessage, message); err != nil {
		t.Fatal(err)
	}
	switch v := (<-out).(type) {
	case []byte:
		if !bytes.Equal(v, message) {
			t.Fatalf("read %d bytes, want %d", len(v), len(message))
		}
	case error:
		t.Fatalf("read failed: %v", v)
	}
}

// TestWSConnReadDefaultRejectsOversizedMessage verifies that the finite default
// still rejects messages above its pre-auth bound.
func TestWSConnReadDefaultRejectsOversizedMessage(t *testing.T) {
	t.Setenv("CHISEL_WS_READ_LIMIT", "")
	out, client := wsPair(t, defaultWebSocketReadLimit+1)
	message := bytes.Repeat([]byte("x"), defaultWebSocketReadLimit+1)
	if err := client.WriteMessage(websocket.BinaryMessage, message); err != nil {
		t.Fatal(err)
	}
	switch v := (<-out).(type) {
	case error:
		if !errors.Is(v, websocket.ErrReadLimit) {
			t.Fatalf("read failed with %v, want %v", v, websocket.ErrReadLimit)
		}
	case []byte:
		t.Fatalf("oversized message was read (%d bytes leaked through)", len(v))
	}
}

func TestWSConnReadCustomLimit(t *testing.T) {
	const limit = 32 * 1024
	t.Setenv("CHISEL_WS_READ_LIMIT", "32768")
	out, client := wsPair(t, limit+1)
	if err := client.WriteMessage(websocket.BinaryMessage, bytes.Repeat([]byte("x"), limit+1)); err != nil {
		t.Fatal(err)
	}
	v, ok := (<-out).(error)
	if !ok {
		t.Fatal("message above custom limit was read")
	}
	if !errors.Is(v, websocket.ErrReadLimit) {
		t.Fatalf("read failed with %v, want %v", v, websocket.ErrReadLimit)
	}
}

func TestWSConnReadNegativeLimitUsesDefault(t *testing.T) {
	t.Setenv("CHISEL_WS_READ_LIMIT", "-1")
	out, client := wsPair(t, defaultWebSocketReadLimit+1)
	message := bytes.Repeat([]byte("x"), defaultWebSocketReadLimit+1)
	if err := client.WriteMessage(websocket.BinaryMessage, message); err != nil {
		t.Fatal(err)
	}
	v, ok := (<-out).(error)
	if !ok {
		t.Fatal("negative limit disabled the read cap")
	}
	if !errors.Is(v, websocket.ErrReadLimit) {
		t.Fatalf("read failed with %v, want %v", v, websocket.ErrReadLimit)
	}
}

func TestWSConnReadLimitDisabled(t *testing.T) {
	t.Setenv("CHISEL_WS_READ_LIMIT", "0")
	message := bytes.Repeat([]byte("x"), defaultWebSocketReadLimit+1)
	out, client := wsPair(t, len(message))
	if err := client.WriteMessage(websocket.BinaryMessage, message); err != nil {
		t.Fatal(err)
	}
	switch v := (<-out).(type) {
	case []byte:
		if !bytes.Equal(v, message) {
			t.Fatalf("read %d bytes, want %d", len(v), len(message))
		}
	case error:
		t.Fatalf("read failed with limit disabled: %v", v)
	}
}
