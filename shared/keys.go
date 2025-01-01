package shared

import (
	"crypto/sha256"
	"encoding/base64"
	"golang.org/x/crypto/ssh"
)

// CalculateFP4Key calculates the SHA256 hash of an SSH public key
func CalculateFP4Key(k ssh.PublicKey) string {
	bytes := sha256.Sum256(k.Marshal())
	return base64.StdEncoding.EncodeToString(bytes[:])
}
