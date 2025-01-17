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

const ValkyrieKeyPrefix = "ck-"

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
//   ..........> ChiselKey .........

func Seed2PEM(seed string) ([]byte, error) {
	privateKey, err := seed2PrivateKey(seed)
	if err != nil {
		return nil, err
	}

	return privateKey2PEM(privateKey)
}

func seed2ValkyrieKey(seed string) ([]byte, error) {
	privateKey, err := seed2PrivateKey(seed)
	if err != nil {
		return nil, err
	}

	return privateKey2ValkyrieKey(privateKey)
}

func seed2PrivateKey(seed string) (*ecdsa.PrivateKey, error) {
	if seed == "" {
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		return GenerateKeyGo119(elliptic.P256(), NewDetermRand([]byte(seed)))
	}
}

func privateKey2ValkyrieKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	b, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	encodedPrivateKey := make([]byte, base64.RawStdEncoding.EncodedLen(len(b)))
	base64.RawStdEncoding.Encode(encodedPrivateKey, b)

	return append([]byte(ValkyrieKeyPrefix), encodedPrivateKey...), nil
}

func privateKey2PEM(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	b, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), nil
}

func valkyrieKey2PrivateKey(valkyrieKey []byte) (*ecdsa.PrivateKey, error) {
	rawValkyrieKey := valkyrieKey[len(ValkyrieKeyPrefix):]

	decodedPrivateKey := make([]byte, base64.RawStdEncoding.DecodedLen(len(rawValkyrieKey)))
	_, err := base64.RawStdEncoding.Decode(decodedPrivateKey, rawValkyrieKey)
	if err != nil {
		return nil, err
	}

	return x509.ParseECPrivateKey(decodedPrivateKey)
}

func ValkyrieKey2PEM(valkyrieKey []byte) ([]byte, error) {
	privateKey, err := valkyrieKey2PrivateKey(valkyrieKey)
	if err == nil {
		return privateKey2PEM(privateKey)
	}

	return nil, err
}

func IsValkyrieKey(valkyrieKey []byte) bool {
	return strings.HasPrefix(string(valkyrieKey), ValkyrieKeyPrefix)
}
