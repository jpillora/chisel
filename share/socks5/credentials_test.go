package socks5

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStaticCredentials(t *testing.T) {
	creds := StaticCredentials{
		"foo": "bar",
		"baz": "",
	}

	require.True(t, creds.Valid("foo", "bar"), "expect valid")
	require.True(t, creds.Valid("baz", ""), "expect valid")
	require.False(t, creds.Valid("foo", ""), "expect invalid")
}
