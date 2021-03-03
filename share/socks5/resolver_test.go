package socks5

import (
	"github.com/stretchr/testify/require"
	"testing"

	"context"
)

func TestDNSResolver(t *testing.T) {
	d := DNSResolver{}
	ctx := context.Background()

	_, addr, err := d.Resolve(ctx, "localhost")
	require.Nilf(t, err, "err: %v", err)

	require.Truef(t, addr.IsLoopback(), "expected loopback")
}
