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
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"

	"github.com/jpillora/chisel/share"

	"time"
)

const (
	B  = 1
	KB = 1024 * B
	MB = 1024 * KB
	GB = 1024 * MB
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
	benchSizes("4001")
}

func benchSizes(port string) {
	for size := 1; size < 100*MB; size *= 10 {
		testTunnel(port, size)
	}
}

func testTunnel(port string, size int) {
	t0 := time.Now()
	resp, err := requestFile(port, size)
	if err != nil {
		fatal(err)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fatal(err)
	}
	t1 := time.Now()
	fmt.Printf(":%s => %d bytes in %s\n", port, size, t1.Sub(t0))
	if len(b) != size {
		fatalf("%d bytes expected, got %d", size, len(b))
	}
}

//============================

func requestFile(port string, size int) (*http.Response, error) {
	url := "http://127.0.0.1:" + port + "/" + strconv.Itoa(size)
	// fmt.Println(url)
	return http.Get(url)
}

func makeFileServer() *chshare.HTTPServer {
	bsize := 3 * MB
	bytes := make([]byte, bsize)
	//filling huge buffer
	for i := 0; i < len(bytes); i++ {
		bytes[i] = byte(i)
	}

	s := chshare.NewHTTPServer()
	s.SetKeepAlivesEnabled(false)
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
		fatal(fs.Wait())
	}()

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

	time.Sleep(100 * time.Millisecond)

	hd := exec.Command("chisel", "server", "--port", "2002" /*"--key", "foobar",*/)
	// hd.Stdout = os.Stdout
	if err := hd.Start(); err != nil {
		fatal(err)
	}
	hf := exec.Command("chisel", "client", /*"--key", "foobar",*/
		"127.0.0.1:2002",
		"2001:3000")
	// hf.Stdout = os.Stdout
	if err := hf.Start(); err != nil {
		fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	defer func() {
		if r := recover(); r != nil {
			log.Print(r)
		}
	}()
	run()

	cd.Process.Kill()
	cf.Process.Kill()
	hd.Process.Kill()
	hf.Process.Kill()
	fs.Close()
}
