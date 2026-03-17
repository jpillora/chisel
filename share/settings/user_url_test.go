package settings

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jpillora/chisel/share/cio"
)

func newTestURLUserIndex(t *testing.T, handler http.HandlerFunc) *URLUserIndex {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewURLUserIndex(srv.URL, cio.NewLogger("test"))
}

// assertPostJSON verifies the request is a JSON POST.
func assertPostJSON(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", r.Method)
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestURLUserIndex_200WithAddresses(t *testing.T) {
	idx := newTestURLUserIndex(t, func(w http.ResponseWriter, r *http.Request) {
		assertPostJSON(t, r)
		json.NewEncoder(w).Encode([]string{`^127\.0\.0\.1:\d+$`, `^10\.`})
	})
	user, err := idx.GetUser("alice", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "alice" {
		t.Fatalf("expected name alice, got %s", user.Name)
	}
	if len(user.Addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(user.Addrs))
	}
	if !user.HasAccess("127.0.0.1:8080") {
		t.Fatal("expected access to 127.0.0.1:8080")
	}
	if user.HasAccess("1.2.3.4:8080") {
		t.Fatal("expected no access to 1.2.3.4:8080")
	}
}

func TestURLUserIndex_200AllowAll(t *testing.T) {
	idx := newTestURLUserIndex(t, func(w http.ResponseWriter, r *http.Request) {
		assertPostJSON(t, r)
		json.NewEncoder(w).Encode([]string{""})
	})
	user, err := idx.GetUser("bob", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if !user.HasAccess("anything:1234") {
		t.Fatal("expected allow-all access")
	}
}

func TestURLUserIndex_200EmptyAddrs(t *testing.T) {
	idx := newTestURLUserIndex(t, func(w http.ResponseWriter, r *http.Request) {
		assertPostJSON(t, r)
		json.NewEncoder(w).Encode([]string{})
	})
	user, err := idx.GetUser("carol", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if user.HasAccess("127.0.0.1:9000") {
		t.Fatal("expected no access with empty addr list")
	}
}

func TestURLUserIndex_NonOKDenied(t *testing.T) {
	idx := newTestURLUserIndex(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	})
	user, err := idx.GetUser("eve", "wrong")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if user != nil {
		t.Fatal("expected nil user on denial")
	}
}

func TestURLUserIndex_InvalidJSON(t *testing.T) {
	idx := newTestURLUserIndex(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	})
	user, err := idx.GetUser("frank", "pass")
	if err == nil {
		t.Fatal("expected error for invalid JSON body")
	}
	if user != nil {
		t.Fatal("expected nil user on parse error")
	}
}

func TestURLUserIndex_RequestFormat(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	idx := newTestURLUserIndex(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode([]string{""})
	})
	_, err := idx.GetUser("grace", "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", gotContentType)
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("could not parse request body: %v", err)
	}
	if payload.Username != "grace" || payload.Password != "hunter2" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
