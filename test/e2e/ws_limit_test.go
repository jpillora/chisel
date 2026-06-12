package e2e_test

import (
	"bytes"
	"net"
	"net/http"
	"testing"
	"time"

	chserver "github.com/jpillora/chisel/server"

	"github.com/gorilla/websocket"
)

// TestPreAuthOversizedFrame verifies that an unauthenticated peer
// sending a huge websocket frame gets disconnected by the read limit
// instead of the server buffering the frame into memory.
func TestPreAuthOversizedFrame(t *testing.T) {
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
	//send a 5MB frame before authenticating; the server should kill
	//the connection part-way through the frame
	big := bytes.Repeat([]byte("A"), 5*1024*1024)
	if err := ws.WriteMessage(websocket.BinaryMessage, big); err != nil {
		return //reset mid-write: limit enforced
	}
	//otherwise the next read must fail with a close/reset, not
	//a timeout (a timeout means the connection survived)
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = ws.ReadMessage()
	if err == nil {
		t.Fatal("received a message after oversized pre-auth frame")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("connection still alive 5s after oversized pre-auth frame")
	}
}
