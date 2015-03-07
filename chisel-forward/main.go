package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel/chisel-forward/client"
)

const help = `

	Usage: chisel-forward [options] server remote [remote] [remote] ...

	server is the URL to the chiseld server.

	remotes are remote connections tunneled through the server, each 
	of which come in the form:
		<local-host>:<local-port>:<remote-host>:<remote-port>

		* Only remote-port is required.
		* local-port defaults to remote-port.
		* local-host defaults to 0.0.0.0 (all interfaces).
		* remote-host defaults to 0.0.0.0 (server localhost).

		example remotes

			3000
			example.com:3000
			3000:google.com:80
			192.168.0.5:3000:google.com:80

	Options:

	--auth AUTH - auth specifies the optional authentication string
	used by the server.

	-v enable verbose logging

	Read more:
	https://github.com/jpillora/chisel

`

func main() {
	auth := flag.String("auth", "", "")
	verbose := flag.Bool("v", false, "")
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

	c.Info = true
	c.Debug = *verbose

	c.Start()

	if err = c.Wait(); err != nil {
		log.Fatal(err)
	}
}
