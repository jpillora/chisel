// +build pprof

package cos

import (
	"log"
	"net/http"
	_ "net/http/pprof" //import http profiler api
)

func init() {
	go func() {
		log.Fatal(http.ListenAndServe("localhost:6060", nil))
	}()
	log.Printf("[pprof] listening on 6060")
}
