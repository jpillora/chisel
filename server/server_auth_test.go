package chserver

import (
	"net"
	"strings"
	"testing"
)

type testConnMeta struct{ user string }

func (m testConnMeta) User() string          { return m.user }
func (m testConnMeta) SessionID() []byte     { return []byte("test-session") }
func (m testConnMeta) ClientVersion() []byte { return nil }
func (m testConnMeta) ServerVersion() []byte { return nil }
func (m testConnMeta) RemoteAddr() net.Addr  { return &net.TCPAddr{} }
func (m testConnMeta) LocalAddr() net.Addr   { return &net.TCPAddr{} }

func TestAuthUser(t *testing.T) {
	s, err := NewServer(&Config{KeySeed: "auth-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddUser("alice", "secret", ""); err != nil {
		t.Fatal(err)
	}
	//valid credentials carry the username through Permissions
	perms, err := s.authUser(testConnMeta{"alice"}, []byte("secret"))
	if err != nil {
		t.Fatalf("valid login failed: %v", err)
	}
	if perms == nil || perms.Extensions["user"] != "alice" {
		t.Fatalf("expected user extension, got %+v", perms)
	}
	//wrong password
	if _, err := s.authUser(testConnMeta{"alice"}, []byte("wrong")); err == nil {
		t.Fatal("wrong password accepted")
	} else if strings.Contains(err.Error(), "%s") || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("malformed error message: %q", err)
	}
	//unknown user
	if _, err := s.authUser(testConnMeta{"bob"}, []byte("secret")); err == nil {
		t.Fatal("unknown user accepted")
	}
}

func TestAuthUserAllowAll(t *testing.T) {
	//no users configured: authentication is disabled
	s, err := NewServer(&Config{KeySeed: "auth-allow-all"})
	if err != nil {
		t.Fatal(err)
	}
	perms, err := s.authUser(testConnMeta{"anyone"}, []byte("anything"))
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
