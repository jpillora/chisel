package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel"
)

const help = `
	Usage: chisel-forward [--auth str] remote [remote] [remote]

	where a remote is in the form:
		example.com:3000 (http://0.0.0.0:3000 => http://example.com:3000)
		3000:google.com:80 (http://0.0.0.0:3000 => http://google.com:80)

`

func main() {
	auth := flag.String("auth", "", "Optional authentication")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, help)
		os.Exit(1)
	}
	flag.Parse()
	args := flag.Args()

	c := &chisel.Config{
		Version: chisel.Version,
		Auth:    *auth,
	}

	for _, a := range args {
		r, err := chisel.DecodeRemote(a)
		if err != nil {
			log.Fatalf("Remote decode failed: %s", err)
		}
		c.Remotes = append(c.Remotes, r)
	}

	if len(c.Remotes) == 0 {
		log.Fatalf("At least one remote is required")
	}

	fmt.Printf("Forwarding:\n")
	for i, r := range c.Remotes {
		fmt.Printf(" [#%d] %s:%s -> %s:%s\n", i+1, r.LocalHost, r.LocalPort, r.RemoteHost, r.RemotePort)
	}

	b, _ := json.Marshal(c)

	fmt.Printf("%s", b)
}
