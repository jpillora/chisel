package e2e_test

import (
	"testing"

	chclient "github.com/valkyrie-io/connector-tunnel/client"
	chserver "github.com/valkyrie-io/connector-tunnel/server"
)

func TestTLS(t *testing.T) {
	tlsConfig, err := newTestTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer tlsConfig.Close()

	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS:     *tlsConfig.serverTLS,
			KeyFile: "./ec_private_key.pem",
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS:     *tlsConfig.clientTLS,
			Server:  "https://localhost:" + tmpPort,
		})
	defer teardown()
	//test remote
	result, err := post("http://localhost:"+tmpPort, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
}
