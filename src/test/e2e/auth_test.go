package e2e_test

import (
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

//TODO tests for:
// - failed auth
// - dynamic auth (server add/remove user)
// - watch auth file

func TestAuth(t *testing.T) {
	tmpPort1 := availablePort()
	tmpPort2 := availablePort()
	//setup server, client, fileserver
	teardown := simpleSetup(t,
		&chserver.Config{
			KeySeed: "foobar",
			Auth:    "../bench/userfile",
		},
		&chclient.Config{
			Remotes: []string{
				"0.0.0.0:" + tmpPort1 + ":127.0.0.1:$FILEPORT",
				"0.0.0.0:" + tmpPort2 + ":localhost:$FILEPORT",
			},
			Auth: "foo:bar",
		})
	defer teardown()
	//test first remote
	result, err := post("http://localhost:"+tmpPort1, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
	//test second remote
	result, err = post("http://localhost:"+tmpPort2, "bar")
	if err != nil {
		t.Fatal(err)
	}
	if result != "bar!" {
		t.Fatalf("expected exclamation mark added again")
	}
}
