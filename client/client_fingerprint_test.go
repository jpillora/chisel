package chclient

import (
	"crypto/md5"
	"fmt"
	"strings"
	"testing"

	"github.com/jpillora/chisel/share/ccrypto"
	"golang.org/x/crypto/ssh"
)

// TestLegacyFingerprintRequiresFullMatch verifies truncated legacy MD5
// fingerprints are rejected; a prefix match would "verify" against
// roughly 1 in 65k keys for a two-octet prefix.
func TestLegacyFingerprintRequiresFullMatch(t *testing.T) {
	pem, err := ccrypto.Seed2PEM("legacy-fp-test")
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()
	sum := md5.Sum(pub.Marshal())
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	full := strings.Join(parts, ":")
	mk := func(fp string) *Client {
		return &Client{config: &Config{Fingerprint: fp}}
	}
	if err := mk(full).verifyLegacyFingerprint(pub); err != nil {
		t.Fatalf("full legacy fingerprint rejected: %v", err)
	}
	for _, truncated := range []string{full[:5], full[:23], "a5:32"} {
		if err := mk(truncated).verifyLegacyFingerprint(pub); err == nil {
			t.Fatalf("truncated legacy fingerprint %q accepted", truncated)
		}
	}
}
