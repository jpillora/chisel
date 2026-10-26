package chserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	chshare "github.com/jpillora/chisel/share"
	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

func TestLogHandshakeFailureQuotesAuthenticationError(t *testing.T) {
	var logs bytes.Buffer
	logger := cio.NewLoggerFlag("server", 0)
	logger.Debug = true
	logger.SetOutput(&logs)
	s := &Server{Logger: logger}

	username := "mallory\nserver: forged\r\t\x00"
	authErr := fmt.Errorf("invalid authentication for username: %s", username)
	handshakeErr := &ssh.ServerAuthError{Errors: []error{authErr}}
	s.logHandshakeFailure(handshakeErr)

	const wantLog = "server: Failed to handshake (\"[invalid authentication for username: mallory\\nserver: forged\\r\\t\\x00]\")\n"
	if got := logs.String(); got != wantLog {
		t.Fatalf("unexpected handshake failure log:\n got: %q\nwant: %q", got, wantLog)
	}
	if got := strings.Count(logs.String(), "\n"); got != 1 {
		t.Fatalf("authentication error created extra log lines: got %d newlines in %q", got, logs.String())
	}
}

func dialTestServer(t *testing.T, s *Server) ssh.Conn {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(s.handleClientHandler))
	t.Cleanup(ts.Close)
	ws, _, err := (&websocket.Dialer{Subprotocols: []string{chshare.ProtocolVersion}}).
		Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	sc, _, reqs, err := ssh.NewClientConn(cnet.NewWebSocketConn(ws), "", &ssh.ClientConfig{
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sc.Close() })
	go ssh.DiscardRequests(reqs)
	return sc
}

func TestOutboundChannelFollowsDisableForward(t *testing.T) {
	for _, disableForward := range []bool{false, true} {
		t.Run(fmt.Sprintf("disableForward=%v", disableForward), func(t *testing.T) {
			s, err := NewServer(&Config{KeySeed: "unit", Reverse: true, NoForward: disableForward})
			if err != nil {
				t.Fatal(err)
			}
			target, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			go func() {
				for {
					c, err := target.Accept()
					if err != nil {
						return
					}
					c.Close()
				}
			}()
			sc := dialTestServer(t, s)
			//a config with a forward remote is accepted either way
			remote, err := settings.DecodeRemote("127.0.0.1:0:" + target.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			cfg, _ := json.Marshal(settings.Config{Version: "0", Remotes: settings.Remotes{remote}})
			if ok, reply, err := sc.SendRequest("config", true, cfg); err != nil || !ok {
				t.Fatalf("config rejected: %v %q", err, reply)
			}
			ch, _, err := sc.OpenChannel("chisel", []byte(target.Addr().String()))
			if err == nil {
				ch.Close()
			}
			if !disableForward && err != nil {
				t.Fatalf("channel rejected without --disable-forward: %v", err)
			}
			if disableForward && (err == nil || !strings.Contains(err.Error(), "Denied outbound connection")) {
				t.Fatalf("channel not denied with --disable-forward: %v", err)
			}
		})
	}
}
