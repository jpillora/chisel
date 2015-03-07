package chisel

import (
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

//all tests perform the follwing
// local proxy   http server   file server
//  [2001]   ->    [2002]   ->    [2003]

func TestSimple1(t *testing.T) {

	go fileserver(t)
	go server(t)
	time.Sleep(time.Second)

	go client(t)
	time.Sleep(time.Second)

	resp, err := http.Get("http://localhost:2001/5000")
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(b) != 5000 {
		t.Fatal("5000 bytes expected")
	}
}

//============================
// test helpers

func server(t *testing.T) {
	s, err := server.NewServer("foobar", "")
	if err != nil {
		t.Fatal(err)
	}

	s.Info = true
	s.Debug = true

	err = s.Start("0.0.0.0", "2002")
	if err != nil {
		t.Fatal(err)
	}
}

func client(t *testing.T) {
	c, err := client.NewClient("foobar", "localhost:2002", "2001:2003")
	if err != nil {
		t.Fatal(err)
	}

	c.Info = true
	c.Debug = true

	err = c.Start()
	if err != nil {
		t.Fatal(err)
	}
}

func fileserver(t *testing.T) {
	http.ListenAndServe("0.0.0.0:2003", nil)
}
