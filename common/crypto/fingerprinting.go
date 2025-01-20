package crypto

import (
	"crypto/sha256"
	"encoding/base64"
	"golang.org/x/crypto/ssh"
)

// FPKey calculates the SHA256 hash of an SSH public key
func FPKey(k ssh.PublicKey) string {
	bytes := sha256.Sum256(k.Marshal())
	return base64.StdEncoding.EncodeToString(bytes[:])
}
