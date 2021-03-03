package socks5

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"testing"
	"time"
)

func TestSOCKS5_Connect(t *testing.T) {
	lAddr := startOneShotPingPongServer(t)

	// Create a socks server
	creds := StaticCredentials{
		"foo": "bar",
	}
	cator := UserPassAuthenticator{Credentials: creds}
	conf := &Config{
		AuthMethods: []Authenticator{cator},
	}
	serv, err := New(conf)
	require.Nilf(t, err, "err: %v", err)

	// Start listening
	go func() {
		err := serv.ListenAndServe(context.Background(), "tcp", "127.0.0.1:12365")
		assert.Nilf(t, err, "err: %v", err)
	}()
	time.Sleep(10 * time.Millisecond)

	// Get a local conn
	conn, err := net.Dial("tcp", "127.0.0.1:12365")
	require.Nilf(t, err, "err: %v", err)

	// Connect, auth and connec to local
	req := bytes.NewBuffer(nil)
	req.Write([]byte{5})
	req.Write([]byte{2, AuthMethodNoAuth, AuthMethodUserPass})
	req.Write([]byte{1, 3, 'f', 'o', 'o', 3, 'b', 'a', 'r'})
	req.Write([]byte{5, 1, 0, 1, 127, 0, 0, 1})

	port := []byte{0, 0}
	binary.BigEndian.PutUint16(port, uint16(lAddr.Port))
	req.Write(port)

	// Send a ping
	req.Write([]byte("ping"))

	// Send all the bytes
	_, err = conn.Write(req.Bytes())
	require.Nilf(t, err, "err: %v", err)

	// Verify response
	expected := []byte{
		socks5Version, AuthMethodUserPass,
		1, AuthUserPassStatusSuccess,
		5,
		0,
		0,
		1,
		0, 0, 0, 0,
		0, 0,
		'p', 'o', 'n', 'g',
	}
	out := make([]byte, len(expected))

	err = conn.SetDeadline(time.Now().Add(time.Second))
	require.Nilf(t, err, "err: %v", err)

	_, err = io.ReadAtLeast(conn, out, len(out))
	require.Nilf(t, err, "err: %v", err)

	// Ignore the port
	out[12] = 0
	out[13] = 0

	require.Equalf(t, expected, out, "bad: %v", out)
}
