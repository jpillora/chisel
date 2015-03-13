//chisel tests
//===================
//                    (direct)
//         .--------------->----------------.
//        /    chisel         chisel         \
// request--->client:2001--->server:2002---->fileserver:3000
//        \                                  /
//         '--> crowbar:4001--->crowbar:4002'
//              client           server
//
// benchmarks don't use testing.B, instead use
//		go test -test.run=Bench
//
// tests use
//		go test -test.run=Request
//
// crowbar and chisel binaries should be in your PATH

package test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"

	"testing"
	"time"
)

const (
	B  = 1
	KB = 1024 * B
	MB = 1024 * KB
	GB = 1024 * MB
)

//test
func TestRequestChisel(t *testing.T) {
	testTunnel("2001", 500, t)
	testTunnel("2001", 50000, t)
}

//benchmark
func TestBenchDirect(t *testing.T) {
	benchSizes("3000", t)
}
func TestBenchChisel(t *testing.T) {
	benchSizes("2001", t)
}

// func TestBenchrowbar(t *testing.T) {
// 	benchSizes("4001", t)
// }

func benchSizes(port string, t *testing.T) {
	for size := 1; size < 100*MB; size *= 10 {
		testTunnel(port, size, t)
	}
}

func testTunnel(port string, size int, t *testing.T) {
	t0 := time.Now()
	resp, err := requestFile(port, size)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.Now()
	fmt.Printf(":%s => %d bytes in %s\n", port, size, t1.Sub(t0))
	if len(b) != size {
		t.Fatalf("%d bytes expected, got %d", size, len(b))
	}
}

//============================

//global setup
func TestMain(m *testing.M) {

	fs := makeFileServer()
	go func() {
		log.Fatal(fs.Wait())
	}()

	dir, _ := os.Getwd()
	cd := exec.Command("crowbard",
		`-listen`, "0.0.0.0:4002",
		`-userfile`, path.Join(dir, "userfile"))
	if err := cd.Start(); err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Fatalf("crowbard: %s", cd.Wait())
	}()

	time.Sleep(100 * time.Millisecond)

	cf := exec.Command("crowbar-forward",
		"-local=0.0.0.0:4001",
		"-server=http://127.0.0.1:4002",
		"-remote=127.0.0.1:3000",
		"-username", "foo",
		"-password", "bar")
	if err := cf.Start(); err != nil {
		log.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	hd := exec.Command("chiseld", "--port", "2002", "--auth", "foobar")
	// hd.Stdout = os.Stdout
	if err := hd.Start(); err != nil {
		log.Fatal(err)
	}
	hf := exec.Command("chisel-forward",
		"--auth", "foobar",
		"127.0.0.1:2002",
		"2001:3000")
	if err := hf.Start(); err != nil {
		log.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	fmt.Println("Running!")
	code := m.Run()
	fmt.Println("Done")

	cd.Process.Kill()
	cf.Process.Kill()
	hd.Process.Kill()
	hf.Process.Kill()
	fs.Close()

	os.Exit(code)
}
