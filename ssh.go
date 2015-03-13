package chisel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net"

	"golang.org/x/crypto/ssh"
)

func GenerateKey() ([]byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
	f := md5.Sum(k.Marshal())
	return hex.EncodeToString(f[:])
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
		stream, reqs, err := ch.Accept()
		if err != nil {
			l.Debugf("Failed to accept stream: %s", err)
			continue
		}

		streamCount++
		id := streamCount

		go ssh.DiscardRequests(reqs)
		go handleStream(l.Fork("stream#%d", id), stream, string(ch.ExtraData()))
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
