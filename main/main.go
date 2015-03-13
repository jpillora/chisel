package main

import (
	"log"
	"net/http"

	"github.com/jpillora/chisel"
)

func main() {

	s := chisel.NewHTTPServer()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	log.Println("listening...")
	s.GoListenAndServe("0.0.0.0:3000", handler, chisel.TLSConfig())

	log.Fatal(s.Wait())
}
