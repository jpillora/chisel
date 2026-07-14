package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

//TODO tests for:
// - failed auth
// - dynamic auth (server add/remove user)
// - watch auth file

func TestAuth(t *testing.T) {
	tmpPort1 := availablePort()
	tmpPort2 := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			KeySeed: "foobar",
			Auth:    "../bench/userfile",
		},
		&chclient.Config{
			Remotes: []string{
				"0.0.0.0:" + tmpPort1 + ":127.0.0.1:$FILEPORT",
				"0.0.0.0:" + tmpPort2 + ":localhost:$FILEPORT",
			},
			Auth: "foo:bar",
		})
	defer teardown()
	//test first remote
	result, err := post("http://localhost:"+tmpPort1, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
	//test second remote
	result, err = post("http://localhost:"+tmpPort2, "bar")
	if err != nil {
		t.Fatal(err)
	}
	if result != "bar!" {
		t.Fatalf("expected exclamation mark added again")
	}
}

// TestAuthURL verifies that a chisel server configured with --authurl
// delegates authentication to an external HTTP service.
func TestAuthURL(t *testing.T) {
	// mock auth backend: accepts alice/secret with full access
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if creds.Username == "alice" && creds.Password == "secret" {
			json.NewEncoder(w).Encode([]string{""}) // allow all
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer authSrv.Close()

	tmpPort := availablePort()
	teardown := simpleSetup(t,
		&chserver.Config{
			KeySeed: "authurl-test",
			AuthURL: authSrv.URL,
		},
		&chclient.Config{
			Remotes: []string{"0.0.0.0:" + tmpPort + ":127.0.0.1:$FILEPORT"},
			Auth:    "alice:secret",
		})
	defer teardown()

	result, err := post("http://localhost:"+tmpPort, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello!" {
		t.Fatalf("expected 'hello!', got %q", result)
	}
}

// TestAuthURLDenied verifies that a client with wrong credentials is rejected
// when the server uses --authurl.
func TestAuthURLDenied(t *testing.T) {
	// mock auth backend that always denies
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer authSrv.Close()

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "authurl-deny-test",
		AuthURL: authSrv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	port := availablePort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.StartContext(ctx, "127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	// dial directly at the SSH level; authentication must be rejected
	ws, _, err := (&websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"chisel-v3"},
	}).Dial("ws://127.0.0.1:"+port, http.Header{})
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	conn := cnet.NewWebSocketConn(ws)
	_, _, _, sshErr := ssh.NewClientConn(conn, "", &ssh.ClientConfig{
		User:            "baduser",
		Auth:            []ssh.AuthMethod{ssh.Password("wrongpass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if sshErr == nil {
		t.Fatal("expected SSH auth to fail with bad credentials, but it succeeded")
	}
}

// When the auth URL returns a restrictive address list, the server must enforce it.
func TestAuthURLAddressRestriction(t *testing.T) {
	// Auth server returns a regex that will never match any real remote address.
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if creds.Username == "alice" && creds.Password == "secret" {
			// Return a regex that matches nothing a real remote would look like.
			json.NewEncoder(w).Encode([]string{"^NOMATCH$"})
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer authSrv.Close()

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: "addr-restriction-test",
		AuthURL: authSrv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	port := availablePort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.StartContext(ctx, "127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	// Dial at the SSH level with valid credentials; the config request for a
	// real remote should be rejected because "^NOMATCH$" does not match it.
	ws, _, wsErr := (&websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"chisel-v3"},
	}).Dial("ws://127.0.0.1:"+port, http.Header{})
	if wsErr != nil {
		t.Fatalf("websocket dial: %v", wsErr)
	}
	conn := cnet.NewWebSocketConn(ws)
	sc, _, reqs, sshErr := ssh.NewClientConn(conn, "", &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("secret")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if sshErr != nil {
		t.Fatalf("SSH auth should succeed (valid credentials): %v", sshErr)
	}
	go ssh.DiscardRequests(reqs)

	// Send a config requesting a tunnel to an address that won't match "^NOMATCH$".
	targetPort := availablePort()
	remotes := []*settings.Remote{{
		RemoteHost: "127.0.0.1",
		RemotePort: targetPort,
	}}
	cfg, _ := json.Marshal(settings.Config{Version: "0", Remotes: remotes})
	ok, reply, err := sc.SendRequest("config", true, cfg)
	if err != nil {
		t.Fatalf("config request error: %v", err)
	}
	if ok {
		t.Fatalf("expected config to be rejected due to address restriction, but it was accepted (reply: %s)", reply)
	}
}
