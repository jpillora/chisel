package cnet

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

//wsPair returns the server side of a websocket connection wrapped in
//NewWebSocketConn, plus the raw client side for sending test frames.
func wsPair(t *testing.T) (serverSide chan interface{}, client *websocket.Conn) {
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
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 1024)
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

//TestWSConnReadLimit verifies that an oversized inbound message errors
//the read instead of being buffered into memory.
func TestWSConnReadLimit(t *testing.T) {
	out, client := wsPair(t)
	//bigger than the 64KB default limit
	big := bytes.Repeat([]byte("x"), 128*1024)
	if err := client.WriteMessage(websocket.BinaryMessage, big); err != nil {
		t.Fatal(err)
	}
	switch v := (<-out).(type) {
	case error:
		//read correctly failed
	case []byte:
		t.Fatalf("oversized message was read (%d bytes leaked through)", len(v))
	default:
		_ = v
	}
}

//TestWSConnReadNormal verifies normal-sized messages still flow.
func TestWSConnReadNormal(t *testing.T) {
	out, client := wsPair(t)
	if err := client.WriteMessage(websocket.BinaryMessage, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	switch v := (<-out).(type) {
	case []byte:
		if string(v) != "hello" {
			t.Fatalf("got %q", v)
		}
	case error:
		t.Fatalf("read failed: %v", v)
	}
}
