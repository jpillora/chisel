package chisel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"io"
	"net"

	"golang.org/x/crypto/pbkdf2"
)

var salt = []byte("chisel-some-salt")

func NewCryptoConn(password string, conn net.Conn) net.Conn {

	key := pbkdf2.Key([]byte(password), salt, 4096, 32, sha1.New)

	block, _ := aes.NewCipher([]byte(key))

	var riv [aes.BlockSize]byte
	rstream := cipher.NewOFB(block, riv[:])
	reader := &cipher.StreamReader{S: rstream, R: conn}

	var wiv [aes.BlockSize]byte
	wstream := cipher.NewOFB(block, wiv[:])
	writer := &cipher.StreamWriter{S: wstream, W: conn}

	return &cryptoConn{
		Conn:   conn,
		reader: reader,
		writer: writer,
	}
}

type cryptoConn struct {
	net.Conn
	reader io.Reader
	writer io.Writer
}

func (c *cryptoConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *cryptoConn) Write(p []byte) (int, error) {
	return c.writer.Write(p)
}
