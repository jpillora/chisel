package e2e_test

import (
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

func TestTLS(t *testing.T) {
	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: chserver.TLSConfig{
				Cert: "tls/server-crt/server.crt",
				Key:  "tls/server-crt/server.key",
				CA:   "tls/server-ca/client.crt",
			},
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS: chclient.TLSConfig{
				//for self signed cert, it needs the server cert, for real cert, this need to be the trusted CA cert
				CA:   "tls/client-ca/server.crt",
				Cert: "tls/client-crt/client.crt",
				Key:  "tls/client-crt/client.key",
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

func TestMTLS(t *testing.T) {
	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: chserver.TLSConfig{
				CA:   "tls/server-ca",
				Cert: "tls/server-crt/server.crt",
				Key:  "tls/server-crt/server.key",
			},
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS: chclient.TLSConfig{
				//for self signed cert, it needs the server cert, for real cert, this need to be the trusted CA cert
				CA:   "tls/client-ca/server.crt",
				Cert: "tls/client-crt/client.crt",
				Key:  "tls/client-crt/client.key",
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

func TestTLSMissingClientCert(t *testing.T) {
	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: chserver.TLSConfig{
				CA:   "tls/server-ca/client.crt",
				Cert: "tls/server-crt/server.crt",
				Key:  "tls/server-crt/server.key",
			},
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS: chclient.TLSConfig{
				CA: "tls/client-ca/server.crt",
				//provide no client cert, server should reject the client request
				//Cert: "tls/client-crt/client.crt",
				//Key:  "tls/client-crt/client.key",
			},
			Server: "https://localhost:" + tmpPort,
		})
	defer teardown()
	//test remote
	_, err := post("http://localhost:"+tmpPort, "foo")
	if err == nil {
		t.Fatal(err)
	}
}

func TestTLSMissingClientCA(t *testing.T) {
	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: chserver.TLSConfig{
				//specify a CA which does not match the client cert
				//server should reject the client request
				CA:   "tls/server-crt/server.crt",
				Cert: "tls/server-crt/server.crt",
				Key:  "tls/server-crt/server.key",
			},
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS: chclient.TLSConfig{
				//for self signed cert, it needs the server cert, for real cert, this need to be the trusted CA cert
				CA:   "tls/client-ca/server.crt",
				Cert: "tls/client-crt/client.crt",
				Key:  "tls/client-crt/client.key",
			},
			Server: "https://localhost:" + tmpPort,
		})
	defer teardown()
	//test remote
	_, err := post("http://localhost:"+tmpPort, "foo")
	if err == nil {
		t.Fatal(err)
	}
}
