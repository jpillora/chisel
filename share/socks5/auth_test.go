package socks5

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNoAuth(t *testing.T) {
	req := bytes.NewBuffer(nil)
	req.Write([]byte{1, AuthMethodNoAuth})
	var resp bytes.Buffer

	s, _ := New(&Config{})
	ctx, err := s.authenticate(&resp, req)
	require.Nilf(t, err, "err: %v", err)

	require.Equal(t, AuthMethodNoAuth, ctx.Method, "Invalid Context Method")

	out := resp.Bytes()
	require.Equalf(t, []byte{socks5Version, AuthMethodNoAuth}, out, "bad: %v", out)
}

func TestPasswordAuth_Valid(t *testing.T) {
	req := bytes.NewBuffer(nil)
	req.Write([]byte{2, AuthMethodNoAuth, AuthMethodUserPass})
	req.Write([]byte{1, 3, 'f', 'o', 'o', 3, 'b', 'a', 'r'})
	var resp bytes.Buffer

	cred := StaticCredentials{
		"foo": "bar",
	}

	cator := UserPassAuthenticator{Credentials: cred}

	s, _ := New(&Config{AuthMethods: []Authenticator{cator}})

	ctx, err := s.authenticate(&resp, req)
	require.Nilf(t, err, "err: %v", err)

	require.Equal(t, AuthMethodUserPass, ctx.Method, "Invalid Context Method")

	val, ok := ctx.Payload["Username"]
	require.True(t, ok, "Missing key Username in auth context's payload")
	require.Equal(t, "foo", val, "Invalid Username in auth context's payload")

	out := resp.Bytes()
	require.Equalf(t, []byte{socks5Version, AuthMethodUserPass, 1, AuthUserPassStatusSuccess}, out, "bad: %v", out)
}

func TestPasswordAuth_Invalid(t *testing.T) {
	req := bytes.NewBuffer(nil)
	req.Write([]byte{2, AuthMethodNoAuth, AuthMethodUserPass})
	req.Write([]byte{1, 3, 'f', 'o', 'o', 3, 'b', 'a', 'z'})
	var resp bytes.Buffer

	cred := StaticCredentials{
		"foo": "bar",
	}
	cator := UserPassAuthenticator{Credentials: cred}
	s, _ := New(&Config{AuthMethods: []Authenticator{cator}})

	ctx, err := s.authenticate(&resp, req)
	require.Equalf(t, ErrUserAuthFailed, err, "err: %v", err)
	require.Nil(t, ctx, "Invalid Context Method")

	out := resp.Bytes()
	require.Equalf(t, []byte{socks5Version, AuthMethodUserPass, 1, AuthUserPassStatusFailure}, out, "bad: %v", out)
}

func TestNoSupportedAuth(t *testing.T) {
	req := bytes.NewBuffer(nil)
	req.Write([]byte{1, AuthMethodNoAuth})
	var resp bytes.Buffer

	cred := StaticCredentials{
		"foo": "bar",
	}
	cator := UserPassAuthenticator{Credentials: cred}

	s, _ := New(&Config{AuthMethods: []Authenticator{cator}})

	ctx, err := s.authenticate(&resp, req)
	require.Equalf(t, ErrNoSupportedAuth, err, "err: %v", err)
	require.Nil(t, ctx, "Invalid Context Method")

	out := resp.Bytes()
	require.Equalf(t, []byte{socks5Version, AuthMethodNoAcceptable}, out, "bad: %v", out)
}
