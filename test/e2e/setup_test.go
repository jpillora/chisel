package e2e_test

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

const debug = true

// test layout configuration
type testLayout struct {
	server     *chserver.Config
	client     *chclient.Config
	fileServer bool
	udpEcho    bool
	udpServer  bool
}

func (tl *testLayout) setup(t *testing.T) (server *chserver.Server, client *chclient.Client, teardown func()) {
	//start of the world
	// goroutines := runtime.NumGoroutine()
	//root cancel
	ctx, cancel := context.WithCancel(context.Background())
	//fileserver (fake endpoint)
	filePort := availablePort()
	if tl.fileServer {
		fileAddr := "127.0.0.1:" + filePort
		f := http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				w.Write(append(b, '!'))
			}),
		}
		fl, err := net.Listen("tcp", fileAddr)
		if err != nil {
			t.Fatal(err)
		}
		log.Printf("fileserver: listening on %s", fileAddr)
		go func() {
			f.Serve(fl)
			cancel()
		}()
		go func() {
			<-ctx.Done()
			f.Close()
		}()
	}
	//server
	server, err := chserver.NewServer(tl.server)
	if err != nil {
		t.Fatal(err)
	}
	server.Debug = debug
	port := availablePort()
	if err := server.StartContext(ctx, "127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	go func() {
		server.Wait()
		server.Infof("Closed")
		cancel()
	}()
	//client (with defaults)
	tl.client.Fingerprint = server.GetFingerprint()
	if tl.server.TLS.Key != "" {
		//the domain name has to be localhost to match the ssl cert
		tl.client.Server = "https://localhost:" + port
	} else {
		tl.client.Server = "http://127.0.0.1:" + port
	}
	for i, r := range tl.client.Remotes {
		//convert $FILEPORT into the allocated port for this test case
		if tl.fileServer {
			tl.client.Remotes[i] = strings.Replace(r, "$FILEPORT", filePort, 1)
		}
	}
	client, err = chclient.NewClient(tl.client)
	if err != nil {
		t.Fatal(err)
	}
	client.Debug = debug
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	go func() {
		client.Wait()
		client.Infof("Closed")
		cancel()
	}()
	//cancel context tree, and wait for both client and server to stop
	teardown = func() {
		cancel()
		server.Wait()
		client.Wait()
		//confirm goroutines have been cleaned up
		// time.Sleep(500 * time.Millisecond)
		// TODO remove sleep
		// d := runtime.NumGoroutine() - goroutines
		// if d != 0 {
		// 	pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		// 	t.Fatalf("goroutines left %d", d)
		// }
	}
	//wait a bit...
	//TODO: client signal API, similar to os.Notify(signal)
	//      wait for client setup
	time.Sleep(50 * time.Millisecond)
	//ready
	return server, client, teardown
}

func simpleSetup(t *testing.T, s *chserver.Config, c *chclient.Config) context.CancelFunc {
	conf := testLayout{
		server:     s,
		client:     c,
		fileServer: true,
	}
	_, _, teardown := conf.setup(t)
	return teardown
}

func post(url, body string) (string, error) {
	resp, err := http.Post(url, "text/plain", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func availablePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Panic(err)
	}
	l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		log.Panic(err)
	}
	return port
}
