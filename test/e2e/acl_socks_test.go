package e2e_test

import (
	"encoding/json"
	"io"
	"testing"

	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

// startSocksACLServer starts a socks5-enabled chisel server with a
// single user limited to the given address whitelist.
func startSocksACLServer(t *testing.T, seed string, addrs ...string) string {
	t.Helper()
	s, err := chserver.NewServer(&chserver.Config{
		KeySeed: seed,
		Socks5:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = debug
	if err := s.AddUser("user", "pass", addrs...); err != nil {
		t.Fatal(err)
	}
	port := availablePort()
	if err := s.Start("127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return "127.0.0.1:" + port
}

// TestSocksChannelDenied verifies that a modified client cannot open a
// "socks" channel directly (without declaring a socks remote) when the
// user's ACL has no entry matching "socks".
func TestSocksChannelDenied(t *testing.T) {
	addr := startSocksACLServer(t, "socks-acl-denied", `^127\.0\.0\.1:80$`)
	sc, _, _ := dialChiselSSH(t, addr, "user", "pass")
	defer sc.Close()
	sendConfig(t, sc, nil) //declare no remotes
	ch, _, err := sc.OpenChannel("chisel", []byte("socks"))
	if err == nil {
		ch.Close()
		t.Fatal("socks channel accepted despite ACL without a socks entry")
	}
	t.Logf("socks channel correctly rejected: %v", err)
}

// TestSocksChannelAllowed verifies that a user with a "socks" ACL entry
// can use the SOCKS5 proxy, via a real SOCKS5 handshake.
func TestSocksChannelAllowed(t *testing.T) {
	addr := startSocksACLServer(t, "socks-acl-allowed", `^socks$`)
	sc, _, _ := dialChiselSSH(t, addr, "user", "pass")
	defer sc.Close()
	sendConfig(t, sc, nil)
	ch, reqs, err := sc.OpenChannel("chisel", []byte("socks"))
	if err != nil {
		t.Fatalf("socks channel rejected for authorized user: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()
	//SOCKS5 greeting: version 5, 1 method, no-auth
	if _, err := ch.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(ch, reply); err != nil {
		t.Fatal(err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		t.Fatalf("unexpected SOCKS5 reply: %v", reply)
	}
	t.Log("SOCKS5 handshake succeeded for authorized user")
}

// TestSocksRemoteConfigACL verifies that declared socks remotes are
// checked against the "socks" token at config time (Remote.UserAddr).
func TestSocksRemoteConfigACL(t *testing.T) {
	remote, err := settings.DecodeRemote("5000:socks")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := json.Marshal(settings.Config{Version: "0", Remotes: []*settings.Remote{remote}})
	if err != nil {
		t.Fatal(err)
	}
	//denied without a socks entry
	addr := startSocksACLServer(t, "socks-acl-config-denied", `^127\.0\.0\.1:80$`)
	sc, _, _ := dialChiselSSH(t, addr, "user", "pass")
	defer sc.Close()
	ok, reply, err := sc.SendRequest("config", true, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("socks remote accepted at config time without a socks ACL entry")
	}
	t.Logf("socks remote correctly rejected at config time: %s", reply)
	//allowed with a socks entry
	addr2 := startSocksACLServer(t, "socks-acl-config-allowed", `^socks$`)
	sc2, _, _ := dialChiselSSH(t, addr2, "user", "pass")
	defer sc2.Close()
	sendConfig(t, sc2, []*settings.Remote{remote})
}
