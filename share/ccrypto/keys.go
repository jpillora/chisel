package ccrypto

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
)

// GenerateKey generates a PEM key
func GenerateKey(seed string) ([]byte, error) {
	return Seed2PEM(seed)
}

// GenerateKeyFile generates an ChiselKey
func GenerateKeyFile(keyFilePath, seed string) error {
	chiselKey, err := seed2ChiselKey(seed)
	if err != nil {
		return err
	}

	if keyFilePath == "-" {
		fmt.Print(string(chiselKey))
		return nil
	}
	return os.WriteFile(keyFilePath, chiselKey, 0600)
}

func GenerateKeyJson(seed string) error {
	privateKey, err := seed2PrivateKey(seed)
	if err != nil {
		return err
	}
	chiselKey, err := privateKey2ChiselKey(privateKey)
	if err != nil {
		return err
	}
	pemBytes, err := ChiselKey2PEM(chiselKey)
	if err != nil {
		return err
	}
	private, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return err
	}
	fingerprint := FingerprintKey(private.PublicKey())

	fmt.Printf("{\"key\": \"%s\", \"fingerprint\": \"%s\"}",
		chiselKey, fingerprint)
	return nil
}

// FingerprintKey calculates the SHA256 hash of an SSH public key
func FingerprintKey(k ssh.PublicKey) string {
	bytes := sha256.Sum256(k.Marshal())
	return base64.StdEncoding.EncodeToString(bytes[:])
}
