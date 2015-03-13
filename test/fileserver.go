package test

import (
	"crypto/tls"
	"log"
	"net/http"
	"strconv"

	"github.com/jpillora/chisel"
)

var transport = &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}
var client = &http.Client{Transport: transport}

func requestFile(port string, size int) (*http.Response, error) {
	url := "https://127.0.0.1:" + port + "/" + strconv.Itoa(size)
	// fmt.Println(url)
	return client.Get(url)
}

func makeFileServer() *chisel.HTTPServer {
	bsize := 3 * MB
	bytes := make([]byte, bsize)
	//filling huge buffer
	for i := 0; i < len(bytes); i++ {
		bytes[i] = byte(i)
	}

	s := chisel.NewHTTPServer()
	s.SetKeepAlivesEnabled(false)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rsize, _ := strconv.Atoi(r.URL.Path[1:])
		for rsize >= bsize {
			w.Write(bytes)
			rsize -= bsize
		}
		w.Write(bytes[:rsize])
	})
	s.GoListenAndServe("0.0.0.0:3000", handler, chisel.TLSConfig())
	log.Println("listening on 3000...")
	return s
}
