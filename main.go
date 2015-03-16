package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel/client"
	"github.com/jpillora/chisel/server"
)

var VERSION string = "0.0.0-src" //set via ldflags

var help = `
	Usage: chisel [command] [--help]

	Version: ` + VERSION + `

	Commands:
	  server - runs chisel in server mode
	  client - runs chisel in client mode

	Read more:
	  https://github.com/jpillora/chisel

`

func main() {

	version := flag.Bool("version", false, "")
	flag.Bool("help", false, "")
	flag.Bool("h", false, "")
	flag.Usage = func() {}
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		os.Exit(1)
	}

	args := flag.Args()

	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "server":
		server(args)
	case "client":
		client(args)
	default:
		fmt.Fprintf(os.Stderr, help)
		os.Exit(1)
	}
}

var commonHelp = `
	  --key, Enables AES256 encryption and specify the string to
	  use to derive the key (derivation is performed using PBKDF2
	  with 2048 iterations of SHA256).

	  -v, Enable verbose logging

	  --help, This help text
`

var serverHelp = `
	Usage: chisel server [options]

	Options:

	  --host, Defines the HTTP listening host – the network interface
	  (defaults to 0.0.0.0).

	  --port, Defines the HTTP listening port (defaults to 8080).

	  --proxy, Specifies the default proxy target to use when chiseld
	  receives a normal HTTP request.
` + commonHelp + `
	Read more:
	  https://github.com/jpillora/chisel

`

func server(args []string) {

	flags := flag.NewFlagSet("server", flag.ContinueOnError)

	hostf := flags.String("host", "", "")
	portf := flags.String("port", "", "")
	authf := flags.String("auth", "", "")
	proxyf := flags.String("proxy", "", "")
	verbose := flags.Bool("v", false, "")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, serverHelp)
		os.Exit(1)
	}
	flags.Parse(args)

	host := *hostf
	if host == "" {
		host = os.Getenv("HOST")
	}
	if host == "" {
		host = "0.0.0.0"
	}

	port := *portf
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	auth := *authf
	if auth == "" {
		auth = os.Getenv("AUTH")
	}

	s, err := chserver.NewServer(auth, *proxyf)
	if err != nil {
		log.Fatal(err)
	}

	s.Info = true
	s.Debug = *verbose

	if err = s.Run(host, port); err != nil {
		log.Fatal(err)
	}
}

var clientHelp = `
	Usage: chisel client [options] <server> <remote> [remote] [remote] ...

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
` + commonHelp + `
	Read more:
	  https://github.com/jpillora/chisel

`

func client(args []string) {

	flags := flag.NewFlagSet("client", flag.ContinueOnError)

	auth := flags.String("auth", "", "")
	verbose := flags.Bool("v", false, "")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, clientHelp)
	}
	flags.Parse(args)

	args = flags.Args()
	if len(args) < 2 {
		log.Fatalf("A server and least one remote is required")
	}

	server := args[0]
	remotes := args[1:]

	c, err := chclient.NewClient(*auth, server, remotes...)
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
