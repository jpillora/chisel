package chshare

import (
	"io/ioutil"
	"log"
	"os"
	"strconv"
)

func GeneratePidFile(flag *bool) {
	pid := []byte(strconv.Itoa(os.Getpid()))
	if err := ioutil.WriteFile("chisel.pid", pid, 0644); err != nil {
		log.Fatal(err)
	}
}
