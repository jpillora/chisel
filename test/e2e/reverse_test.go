package e2e_test

// func TestReverse(t *testing.T) {
// 	tmpPort := availablePort()
// 	//setup server, client, fileserver
// 	teardown := setup(t,
// 		&chserver.Config{
// 			Reverse: true,
// 		},
// 		&chclient.Config{
// 			Remotes: []string{"R:$FILEPORT:" + tmpPort},
// 		})
// 	defer teardown()
// 	//test remote
// 	result, err := post("http://localhost:"+tmpPort, "foo")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if result != "foo!" {
// 		t.Fatalf("expected exclamation mark added")
// 	}
// }
