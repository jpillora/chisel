package e2e_test

import (
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

func TestTls(t *testing.T) {
	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: chserver.TLSConfig{
				Cert:      "tls/server-crt/server.crt",
				Key:       "tls/server-crt/server.key",
				MtlsCaDir: "tls/server-ca",
			},
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS: chclient.TLSConfig{
				SkipVerify: false,
				//for self signed cert, it needs the server cert, for real cert, this need to be the trusted CA cert
				CA:         "tls/client-ca/server.crt",
				MtlsCliCrt: "tls/client-crt/client.crt",
				MtlsCliKey: "tls/client-crt/client.key",
			},
			Server: "https://localhost:" + tmpPort,
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
