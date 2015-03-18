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
	"net"
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

func ConnectStreams(l *Logger, chans <-chan ssh.NewChannel) {

	var streamCount int

	for ch := range chans {

		addr := string(ch.ExtraData())

		stream, reqs, err := ch.Accept()
		if err != nil {
			l.Debugf("Failed to accept stream: %s", err)
			continue
		}

		streamCount++
		id := streamCount

		go ssh.DiscardRequests(reqs)
		go handleStream(l.Fork("stream#%d", id), stream, addr)
	}
}

func handleStream(l *Logger, src io.ReadWriteCloser, remote string) {

	dst, err := net.Dial("tcp", remote)
	if err != nil {
		l.Debugf("%s", err)
		src.Close()
		return
	}

	l.Debugf("Open")
	s, r := Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}
