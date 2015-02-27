package chisel

import (
	"log"
	"os"
)

var Debug = os.Getenv("DEBUG") != ""

var Printf = func(fmt string, args ...interface{}) {
	if Debug {
		log.Printf(fmt, args...)
	}
}
