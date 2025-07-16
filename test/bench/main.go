//chisel end-to-end test
//======================
//
//                    (direct)
//         .--------------->----------------.
//        /    chisel         chisel         \
// request--->client:2001--->server:2002---->fileserver:3000
//        \                                  /
//         '--> crowbar:4001--->crowbar:4002'
//              client           server
//
// crowbar and chisel binaries should be in your PATH

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"

	"github.com/jpillora/chisel/share/cnet"

	"time"
)

const ENABLE_CROWBAR = false

const (
	B  = 1
	KB = 1000 * B
	MB = 1000 * KB
	GB = 1000 * MB
)

func run() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fatal("go run main.go [test] or [bench]")
	}
	for _, a := range args {
		switch a {
		case "test":
			test()
		case "bench":
			bench()
		}
	}
}

//test
func test() {
	testTunnel("2001", 500)
	testTunnel("2001", 50000)
}

//benchmark
func bench() {
	benchSizes("3000")
	benchSizes("2001")
	if ENABLE_CROWBAR {
		benchSizes("4001")
	}
}

func benchSizes(port string) {
	for size := 1; size <= 100*MB; size *= 10 {
		testTunnel(port, size)
	}
}

func testTunnel(port string, size int) {
	t0 := time.Now()
	resp, err := requestFile(port, size)
	if err != nil {
		fatal(err)
	}
	if resp.StatusCode != 200 {
		fatal(err)
	}

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		fatal(err)
	}
	t1 := time.Now()
	fmt.Printf(":%s => %d bytes in %s\n", port, size, t1.Sub(t0))
	if int(n) != size {
		fatalf("%d bytes expected, got %d", size, n)
	}
}

//============================

func requestFile(port string, size int) (*http.Response, error) {
	url := "http://127.0.0.1:" + port + "/" + strconv.Itoa(size)
	// fmt.Println(url)
	return http.Get(url)
}

func makeFileServer() *cnet.HTTPServer {
	bsize := 3 * MB
	bytes := make([]byte, bsize)
	//filling huge buffer
	for i := 0; i < len(bytes); i++ {
		bytes[i] = byte(i)
	}

	s := cnet.NewHTTPServer()
	s.Server.SetKeepAlivesEnabled(false)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rsize, _ := strconv.Atoi(r.URL.Path[1:])
		for rsize >= bsize {
			w.Write(bytes)
			rsize -= bsize
		}
		w.Write(bytes[:rsize])
	})
	s.GoListenAndServe("0.0.0.0:3000", handler)
	return s
}

//============================

func fatal(args ...interface{}) {
	panic(fmt.Sprint(args...))
}
func fatalf(f string, args ...interface{}) {
	panic(fmt.Sprintf(f, args...))
}

//global setup
func main() {

	fs := makeFileServer()
	go func() {
		err := fs.Wait()
		if err != nil {
			fmt.Printf("fs server closed (%s)\n", err)
		}
	}()

	if ENABLE_CROWBAR {
		dir, _ := os.Getwd()
		cd := exec.Command("crowbard",
			`-listen`, "0.0.0.0:4002",
			`-userfile`, path.Join(dir, "userfile"))
		if err := cd.Start(); err != nil {
			fatal(err)
		}
		go func() {
			fatalf("crowbard: %v", cd.Wait())
		}()
		defer cd.Process.Kill()

		time.Sleep(100 * time.Millisecond)

		cf := exec.Command("crowbar-forward",
			"-local=0.0.0.0:4001",
			"-server=http://127.0.0.1:4002",
			"-remote=127.0.0.1:3000",
			"-username", "foo",
			"-password", "bar")
		if err := cf.Start(); err != nil {
			fatal(err)
		}
		defer cf.Process.Kill()
	}

	time.Sleep(100 * time.Millisecond)

	hd := exec.Command("chisel", "server",
		// "-v",
		"--key", "foobar",
		"--port", "2002")
	hd.Stdout = os.Stdout
	if err := hd.Start(); err != nil {
		fatal(err)
	}
	defer hd.Process.Kill()

	time.Sleep(100 * time.Millisecond)

	hf := exec.Command("chisel", "client",
		// "-v",
		"--fingerprint", "mOz4rg9zlQ409XAhhj6+fDDVwQMY42CL3Zg2W2oTYxA=",
		"127.0.0.1:2002",
		"2001:3000")
	hf.Stdout = os.Stdout
	if err := hf.Start(); err != nil {
		fatal(err)
	}
	defer hf.Process.Kill()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		if r := recover(); r != nil {
			log.Print(r)
		}
	}()
	run()

	fs.Close()
}
