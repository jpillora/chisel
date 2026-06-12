package chclient

import "testing"

func TestInvalidAuthString(t *testing.T) {
	//auth strings without a colon used to silently send empty creds
	if _, err := NewClient(&Config{
		Server: "http://localhost:0",
		Auth:   "nocolon",
	}); err == nil {
		t.Fatal("client accepted --auth without a colon")
	}
}
