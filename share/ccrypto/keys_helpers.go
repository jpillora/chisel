package ccrypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
)

const KeyPrefix = "tk-"

//  Relations between entities:
//
//   .............> PEM <...........
//   .               ^             .
//   .               |             .
//   .               |             .
// Seed -------> PrivateKey        .
//   .               ^             .
//   .               |             .
//   .               V             .
//   .............> Key ............

func Seed2PEM(seed string) ([]byte, error) {
	privateKey, err := seed2PrivateKey(seed)
	if err != nil {
		return nil, err
	}

	return privateKey2PEM(privateKey)
}

func seed2Key(seed string) ([]byte, error) {
	privateKey, err := seed2PrivateKey(seed)
	if err != nil {
		return nil, err
	}

	return privateKey2Key(privateKey)
}

func seed2PrivateKey(seed string) (*ecdsa.PrivateKey, error) {
	if seed == "" {
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		return GenerateKeyGo119(elliptic.P256(), NewDetermRand([]byte(seed)))
	}
}

func privateKey2Key(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	b, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	encodedPrivateKey := make([]byte, base64.RawStdEncoding.EncodedLen(len(b)))
	base64.RawStdEncoding.Encode(encodedPrivateKey, b)

	return append([]byte(KeyPrefix), encodedPrivateKey...), nil
}

func privateKey2PEM(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	b, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), nil
}

func key2PrivateKey(k []byte) (*ecdsa.PrivateKey, error) {
	rawKey := k[len(KeyPrefix):]

	decodedPrivateKey := make([]byte, base64.RawStdEncoding.DecodedLen(len(rawKey)))
	_, err := base64.RawStdEncoding.Decode(decodedPrivateKey, rawKey)
	if err != nil {
		return nil, err
	}

	return x509.ParseECPrivateKey(decodedPrivateKey)
}

func Key2PEM(k []byte) ([]byte, error) {
	privateKey, err := key2PrivateKey(k)
	if err == nil {
		return privateKey2PEM(privateKey)
	}

	return nil, err
}

func IsKey(k []byte) bool {
	return strings.HasPrefix(string(k), KeyPrefix)
}
