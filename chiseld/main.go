package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel/chiseld/server"
)

var VERSION string = "0.0.0-src" //set via ldflags

var help = `
	Usage: chiseld [options]

	Options:

	--host, Defines the HTTP listening host – the network interface
	(defaults to 0.0.0.0). You may also set the HOST environment
	variable.

	--port, Defines the HTTP listening port (defaults to 8080). You
	may also set the PORT environment variable.

	--auth, Specifies the exact authentication string the client must
	provide to attain access. You may also set the AUTH environment
	variable.

	--proxy, Specifies the default proxy target to use when chiseld
	receives a normal HTTP request.

	-v, Enable verbose logging

	--version, Display version (` + VERSION + `)

	Read more:
	https://github.com/jpillora/chisel
`

func main() {

	hostf := flag.String("host", "", "")
	portf := flag.String("port", "", "")
	authf := flag.String("auth", "", "")
	proxyf := flag.String("proxy", "", "")
	verbose := flag.Bool("v", false, "")
	version := flag.Bool("version", false, "")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, help)
		os.Exit(1)
	}
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		os.Exit(1)
	}

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

	s, err := chiselserver.NewServer(auth, *proxyf)
	if err != nil {
		log.Fatal(err)
	}

	s.Info = true
	s.Debug = *verbose

	if err = s.Run(host, port); err != nil {
		log.Fatal(err)
	}
}
