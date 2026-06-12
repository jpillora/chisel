package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/jpillora/chisel/share/cio"
)

func writeUser(t *testing.T, path, name string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`{"`+name+`:pass": ["*"]}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func loadIndex(t *testing.T, cfg, name string) *UserIndex {
	t.Helper()
	index := NewUserIndex(cio.NewLogger("test"))
	if err := index.LoadUsers(cfg); err != nil {
		t.Fatal(err)
	}
	if _, found := index.Get(name); !found {
		t.Fatalf("expected initial user %q", name)
	}
	return index
}

func waitUser(index *UserIndex, name string) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, found := index.Get(name); found {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestWatchWriteInPlace covers plain writes to the watched file.
func TestWatchWriteInPlace(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "users.json")
	writeUser(t, cfg, "alice")
	index := loadIndex(t, cfg, "alice")
	writeUser(t, cfg, "bob")
	if !waitUser(index, "bob") {
		t.Fatal("write-in-place update not detected")
	}
}

// TestWatchRenameOver covers editors which write a temp file and rename
// it over the original (vim), replacing the inode.
func TestWatchRenameOver(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "users.json")
	writeUser(t, cfg, "alice")
	index := loadIndex(t, cfg, "alice")
	tmp := filepath.Join(dir, "users.json.tmp")
	writeUser(t, tmp, "bob")
	if err := os.Rename(tmp, cfg); err != nil {
		t.Fatal(err)
	}
	if !waitUser(index, "bob") {
		t.Fatal("rename-over update not detected")
	}
}

// TestWatchInvalidJSONKeepsUsers covers half-written or corrupt files:
// the existing users must remain active, and a later valid write must
// still be picked up.
func TestWatchInvalidJSONKeepsUsers(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "users.json")
	writeUser(t, cfg, "alice")
	index := loadIndex(t, cfg, "alice")
	if err := os.WriteFile(cfg, []byte(`{"truncated`), 0600); err != nil {
		t.Fatal(err)
	}
	//wait for the debounced reload attempt to fail
	time.Sleep(500 * time.Millisecond)
	if _, found := index.Get("alice"); !found {
		t.Fatal("users lost after invalid JSON")
	}
	writeUser(t, cfg, "bob")
	if !waitUser(index, "bob") {
		t.Fatal("update after invalid JSON not detected")
	}
}

// TestWatchSymlinkSwap covers kubernetes configmap updates: the file is a
// symlink into a versioned directory, and updates atomically replace the
// directory symlink — no event ever fires for the file path itself.
func TestWatchSymlinkSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not supported")
	}
	base := t.TempDir()
	v1 := filepath.Join(base, "v1")
	v2 := filepath.Join(base, "v2")
	for _, dir := range []string{v1, v2} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeUser(t, filepath.Join(v1, "users.json"), "alice")
	writeUser(t, filepath.Join(v2, "users.json"), "bob")
	data := filepath.Join(base, "..data")
	if err := os.Symlink("v1", data); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(base, "users.json")
	if err := os.Symlink(filepath.Join("..data", "users.json"), cfg); err != nil {
		t.Fatal(err)
	}
	index := loadIndex(t, cfg, "alice")
	//atomic swap, kubelet-style: new symlink renamed over the old one
	tmp := filepath.Join(base, "..data_tmp")
	if err := os.Symlink("v2", tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, data); err != nil {
		t.Fatal(err)
	}
	if !waitUser(index, "bob") {
		t.Fatal("symlink swap update not detected")
	}
}

// TestWatchPinnedUserSurvivesReload verifies that pinned users (--auth)
// are not dropped when the authfile reloads; previously the first
// reload Reset() deleted them.
func TestWatchPinnedUserSurvivesReload(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "users.json")
	writeUser(t, cfg, "alice")
	index := NewUserIndex(cio.NewLogger("test"))
	index.PinUser(&User{
		Name:  "pinned",
		Pass:  "pw",
		Addrs: []*regexp.Regexp{UserAllowAll},
	})
	if err := index.LoadUsers(cfg); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"alice", "pinned"} {
		if _, found := index.Get(n); !found {
			t.Fatalf("expected user %q after load", n)
		}
	}
	//reload: replace alice with bob
	writeUser(t, cfg, "bob")
	if !waitUser(index, "bob") {
		t.Fatal("reload not detected")
	}
	if _, found := index.Get("pinned"); !found {
		t.Fatal("pinned user dropped by authfile reload")
	}
	if _, found := index.Get("alice"); found {
		t.Fatal("alice should have been replaced by the reload")
	}
}
