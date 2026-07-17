package chserver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewServerKeyErrors verifies key-loading failures are returned as
// errors rather than calling log.Fatal, which would kill processes
// embedding chisel as a library.
func TestNewServerKeyErrors(t *testing.T) {
	//missing key file
	if _, err := NewServer(&Config{KeyFile: "/nonexistent/key"}); err == nil {
		t.Fatal("expected error for missing key file")
	}
	//unparseable key material
	bad := filepath.Join(t.TempDir(), "bad.key")
	if err := os.WriteFile(bad, []byte("not a pem key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(&Config{KeyFile: bad}); err == nil {
		t.Fatal("expected error for unparseable key file")
	}
}
