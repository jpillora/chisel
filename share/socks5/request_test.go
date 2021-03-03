package socks5

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type MockConn struct {
	buf bytes.Buffer
}

func (m *MockConn) Write(b []byte) (int, error) {
	return m.buf.Write(b)
}

func (m *MockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: []byte{127, 0, 0, 1}, Port: 65432}
}

func (m *MockConn) Read(_ []byte) (n int, err error)   { return 0, nil }
func (m *MockConn) Close() error                       { return nil }
func (m *MockConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4zero, Port: 0} }
func (m *MockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *MockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *MockConn) SetWriteDeadline(_ time.Time) error { return nil }

func startOneShotPingPongServer(t *testing.T) *net.TCPAddr {
	// Create a local listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nilf(t, err, "err: %v", err)

	go func() {
		conn, err := l.Accept()
		if !assert.Nilf(t, err, "err: %v", err) {
			return
		}

		defer func() { _ = conn.Close() }()

		buf := make([]byte, 4)
		_, err = io.ReadAtLeast(conn, buf, 4)
		if !assert.Nilf(t, err, "err: %v", err) {
			return
		}

		if !assert.Equalf(t, []byte("ping"), buf, "bad: %v", buf) {
			return
		}
		_, err = conn.Write([]byte("pong"))
		assert.Nilf(t, err, "err: %v", err)
	}()
	return l.Addr().(*net.TCPAddr)
}

func TestRequest_Connect(t *testing.T) {
	lAddr := startOneShotPingPongServer(t)

	// Make server
	s, err := New(&Config{
		Rules:    PermitAll(),
		Resolver: DNSResolver{},
		Handler:  &NoUDPHandler{},
	})
	require.Nilf(t, err, "err: %v", err)

	err = s.config.Handler.OnStartServe(nil, nil)
	require.Nilf(t, err, "err: %v", err)

	// Create the connect request
	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{5, 1, 0, 1, 127, 0, 0, 1})

	port := []byte{0, 0}
	binary.BigEndian.PutUint16(port, uint16(lAddr.Port))
	buf.Write(port)

	// Send a ping
	buf.Write([]byte("ping"))

	// Handle the request
	resp := &MockConn{}
	req, err := NewRequest(buf)
	require.Nilf(t, err, "err: %v", err)

	err = s.handleRequest(context.Background(), req, resp)
	require.Nilf(t, err, "err: %v", err)

	// Verify response
	out := resp.buf.Bytes()
	expected := []byte{
		5,
		0,
		0,
		1,
		0, 0, 0, 0,
		0, 0,
		'p', 'o', 'n', 'g',
	}

	// Ignore the port for both
	out[8] = 0
	out[9] = 0

	require.Equalf(t, expected, out, "bad: %v %v", out, expected)
}

func TestRequest_Connect_RuleFail(t *testing.T) {
	lAddr := startOneShotPingPongServer(t)

	// Make server
	s := &Server{config: &Config{
		Rules:    PermitNone(),
		Resolver: DNSResolver{},
	}}

	// Create the connect request
	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{5, 1, 0, 1, 127, 0, 0, 1})

	port := []byte{0, 0}
	binary.BigEndian.PutUint16(port, uint16(lAddr.Port))
	buf.Write(port)

	// Send a ping
	buf.Write([]byte("ping"))

	// Handle the request
	resp := &MockConn{}
	req, err := NewRequest(buf)
	require.Nilf(t, err, "err: %v", err)

	err = s.handleRequest(context.Background(), req, resp)
	require.Truef(t, err == nil  ||  strings.Contains(err.Error(), "blocked by rules"), "err: %v", err)

	// Verify response
	out := resp.buf.Bytes()
	expected := []byte{
		5,
		2,
		0,
		1,
		0, 0, 0, 0,
		0, 0,
	}

	require.Equalf(t, expected, out, "bad: %v %v", out, expected)
}

func checkAddrSpecParsingAndSerializationOfIPv4(t *testing.T, strAddr string) {
	addr, err := ParseHostPort(strAddr)
	require.Nilf(t, err, "ParseHostPort error: %v", err)

	require.True(t, isIpV4(addr.IP), "ParseHostPort parsed IPv4 to IPv6!")
	require.Equalf(t, strAddr, addr.Address(), "Incorrect parsing of %s to %s!", strAddr, addr.Address())

	sz := addr.SerializedSize()
	require.Equal(t, 3+4, sz, "Invalid serialized size for IPv4")

	buffer := make([]byte, sz)
	n, err := addr.SerializeTo(buffer)
	require.Nilf(t, err, "Error serializing IPv4: %v", err)
	require.Equal(t, sz, n, "Invalid bytes count written to serialized size for IPv4")

	addr2, err := readAddrSpec(bytes.NewReader(buffer))
	require.Nilf(t, err, "Error parsing serialized IPv4: %v", err)
	require.Equalf(t, strAddr, addr2.Address(), "Incorrect serialization of %s to %s!", strAddr, addr2.Address())
}

func TestAddrSpecParsingAndSerializationOfIPv4(t *testing.T) {
	checkAddrSpecParsingAndSerializationOfIPv4(t, "127.0.0.1:1080")
	checkAddrSpecParsingAndSerializationOfIPv4(t, "0.0.0.0:0")
	checkAddrSpecParsingAndSerializationOfIPv4(t, "255.255.255.255:65535")
	checkAddrSpecParsingAndSerializationOfIPv4(t, "10.10.10.10:1000")
	checkAddrSpecParsingAndSerializationOfIPv4(t, "8.8.8.8:53")
}
