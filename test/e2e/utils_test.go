package e2e_test

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

const (
	testHost = "127.0.0.1"
	testPort = "3000"
	testAddr = testHost + ":" + testPort
	filePort = "4000"
	fileAddr = testHost + ":" + filePort
)

func test(t *testing.T, s *chserver.Config, c *chclient.Config) {
	//setup server, client, fileserver
	teardown := setup(t, s, c)
	defer teardown()
	//test first remote
	result, err := post("http://localhost:3500", "foo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "foo!" {
		t.Fatalf("expected exclamation mark added")
	}
	//test second remote
	result, err = post("http://localhost:3501", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if result != "bar!" {
		t.Fatalf("expected exclamation mark added again")
	}
}

func setup(t *testing.T, s *chserver.Config, c *chclient.Config) context.CancelFunc {
	ctx, teardown := context.WithCancel(context.Background())
	//server
	server, err := chserver.NewServer(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.StartContext(ctx, testHost, testPort); err != nil {
		t.Fatal(err)
	}
	go func() {
		server.Wait()
		server.Infof("Closed")
		teardown()
	}()
	//client (with defaults)
	c.Fingerprint = server.GetFingerprint()
	c.Server = "http://" + testAddr
	client, err := chclient.NewClient(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	go func() {
		client.Wait()
		client.Infof("Closed")
		teardown()
	}()
	//fileserver
	f := http.Server{
		Addr: fileAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := ioutil.ReadAll(r.Body)
			w.Write(append(b, '!'))
		}),
	}
	go func() {
		log.Printf("fileserver: listening on %s", fileAddr)
		f.ListenAndServe()
		teardown()
	}()
	go func() {
		<-ctx.Done()
		f.Close()
	}()
	//ready
	return teardown
}

func post(url, body string) (string, error) {
	resp, err := http.Post(url, "text/plain", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
