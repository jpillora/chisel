package chserver

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/jpillora/chisel/share/cio"
)

type testConnMeta struct {
	user       string
	remoteAddr net.Addr
}

func (m testConnMeta) User() string          { return m.user }
func (m testConnMeta) SessionID() []byte     { return []byte("test-session") }
func (m testConnMeta) ClientVersion() []byte { return nil }
func (m testConnMeta) ServerVersion() []byte { return nil }
func (m testConnMeta) RemoteAddr() net.Addr {
	if m.remoteAddr != nil {
		return m.remoteAddr
	}
	return &net.TCPAddr{}
}
func (m testConnMeta) LocalAddr() net.Addr { return &net.TCPAddr{} }

func TestAuthUser(t *testing.T) {
	s, err := NewServer(&Config{KeySeed: "auth-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddUser("alice", "secret", ""); err != nil {
		t.Fatal(err)
	}
	//valid credentials carry the username through Permissions
	perms, err := s.authUser(testConnMeta{user: "alice"}, []byte("secret"))
	if err != nil {
		t.Fatalf("valid login failed: %v", err)
	}
	if perms == nil || perms.Extensions["user"] != "alice" {
		t.Fatalf("expected user extension, got %+v", perms)
	}
	//wrong password
	if _, err := s.authUser(testConnMeta{user: "alice"}, []byte("wrong")); err == nil {
		t.Fatal("wrong password accepted")
	} else if strings.Contains(err.Error(), "%s") || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("malformed error message: %q", err)
	}
	//unknown user
	if _, err := s.authUser(testConnMeta{user: "bob"}, []byte("secret")); err == nil {
		t.Fatal("unknown user accepted")
	}
}

func TestAuthUserFailedLoginQuotesUsername(t *testing.T) {
	s, err := NewServer(&Config{KeySeed: "auth-log-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddUser("alice", "secret", ""); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	s.Logger = cio.NewLoggerFlag("server", 0)
	s.Logger.Info = true
	s.Logger.SetOutput(&logs)

	username := "mallory\nserver: forged\r\t\x00"
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 2222}
	_, err = s.authUser(testConnMeta{
		user:       username,
		remoteAddr: remoteAddr,
	}, []byte("wrong"))
	if err == nil {
		t.Fatal("malicious username was accepted")
	}
	if got, want := err.Error(), "invalid authentication for username: "+username; got != want {
		t.Fatalf("authentication error changed: got %q, want %q", got, want)
	}

	const wantLog = "server: Login failed for user \"mallory\\nserver: forged\\r\\t\\x00\" (192.0.2.10:2222)\n"
	if got := logs.String(); got != wantLog {
		t.Fatalf("unexpected failed-login log:\n got: %q\nwant: %q", got, wantLog)
	}
	if got := strings.Count(logs.String(), "\n"); got != 1 {
		t.Fatalf("attacker input created extra log lines: got %d newlines in %q", got, logs.String())
	}
}

func TestAuthUserAllowAll(t *testing.T) {
	//no users configured: authentication is disabled
	s, err := NewServer(&Config{KeySeed: "auth-allow-all"})
	if err != nil {
		t.Fatal(err)
	}
	perms, err := s.authUser(testConnMeta{user: "anyone"}, []byte("anything"))
	if err != nil {
		t.Fatalf("allow-all rejected: %v", err)
	}
	if perms != nil {
		t.Fatalf("expected nil permissions for allow-all, got %+v", perms)
	}
}

func TestInvalidAuthString(t *testing.T) {
	//auth strings without a colon used to silently disable auth
	if _, err := NewServer(&Config{KeySeed: "x", Auth: "nocolon"}); err == nil {
		t.Fatal("server accepted --auth without a colon")
	}
}
