package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel"
	"github.com/jpillora/chisel/chisel-forward/client"
)

const help = `

	Usage: chisel-forward [--auth AUTH] server remote [remote] [remote] ...

	auth specifies the optional authenication string
	used by the server.

	server is the URL to the chiseld server.

	remote is a remote connection via the server, which
	comes in the form:
		<local-host>:<local-port>:<remote-host>:<remote-port>

		* Only remote-port is required.
		* local-port defaults to remote-port.
		* local-host defaults to 0.0.0.0 (all interfaces).
		* remote-host defaults to localhost.

		example remotes

			3000
			example.com:3000
			3000:google.com:80
			192.168.0.5:3000:google.com:80

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

	chisel.Debug = true

	c, err := client.NewClient(*auth, server, remotes)
	if err != nil {
		log.Fatal(err)
	}
	err = c.Start()
	if err != nil {
		log.Fatal(err)
	}
}
