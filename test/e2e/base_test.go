package e2e_test

import (
	"testing"

	chclient "github.com/valkyrie-io/connector-tunnel/client"
	chserver "github.com/valkyrie-io/connector-tunnel/server"
)

func TestReverse(t *testing.T) {
	tmpPort := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Options{
			Reverse: true,
		},
		&chclient.Config{
			Remotes: []string{"R:" + tmpPort + ":$FILEPORT"},
		})
	defer teardown()
	//test remote (this goes through the server and out the client)
	result, err := post("http://localhost:"+tmpPort, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
}
