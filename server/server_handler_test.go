package chserver

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/jpillora/chisel/share/cio"
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
