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
	  -v, Enable verbose logging

	  --help, This help text

	Read more:
	  https://github.com/jpillora/chisel

`

var serverHelp = `
	Usage: chisel server [options]

	Options:

	  --host, Defines the HTTP listening host – the network interface
	  (defaults to 0.0.0.0).

	  --port, Defines the HTTP listening port (defaults to 8080).

	  --key, An optional string to seed the generation of a ECDSA public
	  and private key pair. All commications will be secured using this
	  key pair. Share this fingerprint with clients to enable detection
	  of man-in-the-middle attacks.

	  --authfile, An optional path to a users.json file. This file should
	  be an object with users defined like:
	    "<user:pass>": ["<addr-regex>","<addr-regex>"]
	    when <user> connects, their <pass> will be verified and then
	    each of the remote addresses will be compared against the list
	    of address regular expressions for a match. Addresses will
	    always come in the form "<host/ip>:<port>".

	  --proxy, Specifies the default proxy target to use when chisel
	  receives a normal HTTP request.
` + commonHelp

func server(args []string) {

	flags := flag.NewFlagSet("server", flag.ContinueOnError)

	host := flags.String("host", "", "")
	port := flags.String("port", "", "")
	key := flags.String("key", "", "")
	authfile := flags.String("authfile", "", "")
	proxy := flags.String("proxy", "", "")
	verbose := flags.Bool("v", false, "")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, serverHelp)
		os.Exit(1)
	}
	flags.Parse(args)

	if *host == "" {
		*host = os.Getenv("HOST")
	}
	if *host == "" {
		*host = "0.0.0.0"
	}

	if *port == "" {
		*port = os.Getenv("PORT")
	}
	if *port == "" {
		*port = "8080"
	}

	s, err := chserver.NewServer(&chserver.Config{
		KeySeed:  *key,
		AuthFile: *authfile,
		Proxy:    *proxy,
	})
	if err != nil {
		log.Fatal(err)
	}

	s.Info = true
	s.Debug = *verbose

	if err = s.Run(*host, *port); err != nil {
		log.Fatal(err)
	}
}

var clientHelp = `
	Usage: chisel client [options] <server> <remote> [remote] [remote] ...

	server is the URL to the chisel server.

	remotes are remote connections tunnelled through the server, each of
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

	  --fingerprint, An optional fingerprint (server authentication)
	  string to compare against the server's public key. You may provide
	  just a prefix of the key or the entire string. Fingerprint
	  mismatches will close the connection.

	  --auth, An optional username and password (client authentication)
	  in the form: "<user>:<pass>". These credentials are compared to
	  the credentials inside the server's --authfile.

	  --keepalive, An optional keepalive interval. Since the underlying
	  transport is HTTP, in many instances we'll be traversing through
	  proxies, often these proxies will close idle connections. You must
	  specify a time with a unit, for example '30s' or '2m'. Defaults
	  to '0s' (disabled).

	  --proxy, An optional URL for a HTTP proxy.
` + commonHelp

func client(args []string) {

	flags := flag.NewFlagSet("client", flag.ContinueOnError)

	fingerprint := flags.String("fingerprint", "", "")
	auth := flags.String("auth", "", "")
	keepalive := flags.Duration("keepalive", 0, "")
	verbose := flags.Bool("v", false, "")
	proxy := flags.String("proxy", "", "")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, clientHelp)
		os.Exit(1)
	}
	flags.Parse(args)
	//pull out options, put back remaining args
	args = flags.Args()
	if len(args) < 2 {
		log.Fatalf("A server and least one remote is required")
	}

	c, err := chclient.NewClient(&chclient.Config{
		Fingerprint: *fingerprint,
		Auth:        *auth,
		KeepAlive:   *keepalive,
		Proxy:       *proxy,
		Server:      args[0],
		Remotes:     args[1:],
	})
	if err != nil {
		log.Fatal(err)
	}

	c.Info = true
	c.Debug = *verbose

	if err = c.Run(); err != nil {
		log.Fatal(err)
	}
}
