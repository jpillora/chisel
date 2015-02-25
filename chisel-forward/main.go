package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel/chisel-forward/client"
)

const help = `
	Usage: chisel-forward [--auth str] server remote [remote] [remote] ...

	where server is the URL to the chiseld server

	where a remote is remote connection via the server, in the form
		example.com:3000 (which means http://0.0.0.0:3000 => http://example.com:3000)
		3000:google.com:80 (which means http://0.0.0.0:3000 => http://google.com:80)

`

func main() {
	auth := flag.String("auth", "", "Optional authentication")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, help)
		os.Exit(1)
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		log.Fatalf("A server and least one remote is required")
	}

	server := args[0]
	args = args[1:]

	client.NewClient(*auth, server, args).Start()
}
