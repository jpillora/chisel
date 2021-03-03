package socks5

import (
	"github.com/stretchr/testify/require"
	"testing"

	"context"
)

func TestPermitCommand(t *testing.T) {
	ctx := context.Background()
	r := &PermitCommand{true, false, false}

	_, ok := r.Allow(ctx, &Request{Command: CommandConnect})
	require.True(t, ok, "expect connect")

	_, ok = r.Allow(ctx, &Request{Command: CommandBind})
	require.False(t, ok, "do not expect bind")

	_, ok = r.Allow(ctx, &Request{Command: CommandAssociate})
	require.False(t, ok, "do not expect associate")
}
