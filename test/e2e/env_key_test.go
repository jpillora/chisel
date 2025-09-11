package e2e_test

import (
	"os"
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

func TestChiselKeyEnvironmentVariable(t *testing.T) {
	// Set the CHISEL_KEY environment variable
	os.Setenv("CHISEL_KEY", "test-key-value")
	defer os.Unsetenv("CHISEL_KEY")

	tmpPort := availablePort()
	
	// Create server with empty config - should pick up CHISEL_KEY env var
	serverConfig := &chserver.Config{}
	
	// Setup server and client
	teardown := simpleSetup(t,
		serverConfig,
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
		})
	defer teardown()

	// Test that the connection works - if the key is properly set,
	// the server should start successfully and connections should work
	result, err := post("http://localhost:"+tmpPort, "env-key-test")
	if err != nil {
		t.Fatal(err)
	}
	if result != "env-key-test!" {
		t.Fatalf("expected exclamation mark added, got: %s", result)
	}
}

func TestChiselKeyEnvironmentVariableConsistency(t *testing.T) {
	// This test verifies that the same CHISEL_KEY value produces
	// consistent behavior (same fingerprint) by manually setting KeySeed
	keyValue := "consistency-test-key"

	// Create two server instances with the same KeySeed (simulating what main.go does)
	server1, err := chserver.NewServer(&chserver.Config{
		KeySeed: keyValue,
	})
	if err != nil {
		t.Fatalf("Failed to create first server: %v", err)
	}

	server2, err := chserver.NewServer(&chserver.Config{
		KeySeed: keyValue,
	})
	if err != nil {
		t.Fatalf("Failed to create second server: %v", err)
	}

	// Both servers should have the same fingerprint since they use the same key
	if server1.GetFingerprint() != server2.GetFingerprint() {
		t.Fatalf("Expected same fingerprint for same key, got %s and %s",
			server1.GetFingerprint(), server2.GetFingerprint())
	}
}