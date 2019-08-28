package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseClientFlag(t *testing.T) {
	assert := assert.New(t)
	args := []string{
		"-fingerprint", "FINGERPRINT",
		"-auth", "AUTH-VALUE",
		"-hostname", "HOSTNAME",
		"-keepalive", "30s",
		"-header", "Header1: Foo",
		"-header", "Header2: Bar",
		"-max-retry-count", "2",
		"-max-retry-interval", "12s",
		"SERVER",
		"REMOTE",
	}
	config, pid, verbose := parseClientFlags(args)
	assert.Equal(config.Fingerprint, "FINGERPRINT")
	assert.Equal(config.Headers.Get("Header1"), "Foo")
	assert.Equal(config.Headers.Get("Header2"), "Bar")
	assert.Equal(config.Headers.Get("Host"), "HOSTNAME")
	assert.Equal(config.KeepAlive, 30*time.Second)
	assert.Equal(config.MaxRetryInterval, 12*time.Second)
	assert.Equal(config.MaxRetryCount, 2)
	assert.Equal(config.Auth, "AUTH-VALUE")
	assert.Equal(config.Server, "SERVER")
	assert.Equal(config.Remotes, []string{"REMOTE"})
	assert.Equal(*pid, false)
	assert.Equal(*verbose, false)
}
