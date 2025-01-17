package e2e_test

import (
	"path"
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
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
			TLS: *tlsConfig.serverTLS,
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

func TestMTLS(t *testing.T) {
	tlsConfig, err := newTestTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer tlsConfig.Close()
	//provide no client cert, server should reject the client request
	tlsConfig.serverTLS.CA = path.Dir(tlsConfig.serverTLS.CA)

	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: *tlsConfig.serverTLS,
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

func TestTLSMissingClientCert(t *testing.T) {
	tlsConfig, err := newTestTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer tlsConfig.Close()
	//provide no client cert, server should reject the client request
	tlsConfig.clientTLS.Cert = ""
	tlsConfig.clientTLS.Key = ""

	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: *tlsConfig.serverTLS,
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS:     *tlsConfig.clientTLS,
			Server:  "https://localhost:" + tmpPort,
		})
	defer teardown()
	//test remote
	_, err = post("http://localhost:"+tmpPort, "foo")
	if err == nil {
		t.Fatal(err)
	}
}

func TestTLSMissingClientCA(t *testing.T) {
	tlsConfig, err := newTestTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer tlsConfig.Close()
	//specify a CA which does not match the client cert
	//server should reject the client request
	//provide no client cert, server should reject the client request
	tlsConfig.serverTLS.CA = tlsConfig.clientTLS.CA

	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			TLS: *tlsConfig.serverTLS,
		},
		&chclient.Config{
			Remotes: []string{tmpPort + ":$FILEPORT"},
			TLS:     *tlsConfig.clientTLS,
			Server:  "https://localhost:" + tmpPort,
		})
	defer teardown()
	//test remote
	_, err = post("http://localhost:"+tmpPort, "foo")
	if err == nil {
		t.Fatal(err)
	}
}
