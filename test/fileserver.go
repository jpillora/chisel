package test

import (
	"net/http"
	"strconv"

	"github.com/jpillora/chisel"
)

func requestFile(port string, size int) (*http.Response, error) {
	url := "http://127.0.0.1:" + port + "/" + strconv.Itoa(size)
	// fmt.Println(url)
	return http.Get(url)
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
	s.GoListenAndServe("0.0.0.0:3000", handler)
	return s
}
