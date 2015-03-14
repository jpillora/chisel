package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel/chisel-forward/client"
)

var VERSION string = "0.0.0-src" //set via ldflags

var help = `
	Usage: chisel-forward [options] server remote [remote] [remote] ...

	server is the URL to the chiseld server.

	remotes are remote connections tunneled through the server, each of
	which come in the form:

		<local-host>:<local-port>:<remote-host>:<remote-port>

		* remote-port is required.
		* local-port defaults to remote-port.
		* local-host defaults to 0.0.0.0 (all interfaces).
		* remote-host defaults to 0.0.0.0 (server localhost).

		example remotes

			3000
			example.com:3000
			3000:google.com:80
			192.168.0.5:3000:google.com:80

	Options:

	--auth, Enables AES256 encryption and specifies the string to
	use to derive the key.

	-v, Enable verbose logging

	--version, Display version (` + VERSION + `)

	Read more:
	https://github.com/jpillora/chisel
`

func main() {
	auth := flag.String("auth", "", "")
	verbose := flag.Bool("v", false, "")
	version := flag.Bool("version", false, "")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, help)
	}
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) < 2 {
		log.Fatalf("A server and least one remote is required")
	}

	server := args[0]
	remotes := args[1:]

	c, err := chiselclient.NewClient(*auth, server, remotes...)
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
