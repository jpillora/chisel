package chshare

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

func GenerateKey(seed string) ([]byte, error) {
	var r io.Reader
	if seed == "" {
		r = rand.Reader
	} else {
		r = NewDetermRand([]byte(seed))
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), r)
	if err != nil {
		return nil, err
	}
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("Unable to marshal ECDSA private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), nil
}

func FingerprintKey(k ssh.PublicKey) string {
	bytes := md5.Sum(k.Marshal())
	strbytes := make([]string, len(bytes))
	for i, b := range bytes {
		strbytes[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(strbytes, ":")
}

func OpenStream(conn ssh.Conn, remote string) (io.ReadWriteCloser, error) {
	stream, reqs, err := conn.OpenChannel("chisel", []byte(remote))
	if err != nil {
		return nil, err
	}
	go ssh.DiscardRequests(reqs)
	return stream, nil
}

func RejectStreams(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		ch.Reject(ssh.Prohibited, "Tunnels disallowed")
	}
}
