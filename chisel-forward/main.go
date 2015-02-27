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

	where 'server' is the URL to the chiseld server

	where 'remote' is a remote connection via the server, in the form
		example.com:3000 (which means http://0.0.0.0:3000 => http://example.com:3000)
		3000:google.com:80 (which means http://0.0.0.0:3000 => http://google.com:80)

	Read more:
	https://github.com/jpillora/chisel

`

func main() {
	auth := flag.String("auth", "", "Optional authentication")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, help)
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		log.Fatalf("A server and least one remote is required")
	}

	server := args[0]
	remotes := args[1:]

	c, err := client.NewClient(*auth, server, remotes)
	if err != nil {
		log.Fatal(err)
	}
	err = c.Start()
	if err != nil {
		log.Fatal(err)
	}
}
