package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jpillora/chisel/chiseld/server"
)

const help = `
	Usage: chiseld [--host 0.0.0.0] [--port 8080] [--auth AUTH] [--proxy PROXY]

	host defines the HTTP listening host – the
	network interface (defaults to 0.0.0.0). You
	may also set the HOST environment variable.

	port defines the HTTP listening port (defaults
	to 8080). You may also set the PORT environment
	variable.

	auth specifies the exact authentication string
	the client must provide to attain access. You
	may also set the AUTH environment variable.

	proxy specifies the default proxy target to use
	when chiseld receives a normal HTTP request. You
	may also set the PROXY environment variable.

	Read more:
	https://github.com/jpillora/chisel

`

func main() {

	hostf := flag.String("host", "", "")
	portf := flag.String("port", "", "")
	authf := flag.String("auth", "", "")
	proxyf := flag.String("proxy", "", "")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, help)
		os.Exit(1)
	}
	flag.Parse()

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

	proxy := *proxyf
	if proxy == "" {
		proxy = os.Getenv("PROXY")
	}

	s, err := server.NewServer(auth, proxy)
	if err != nil {
		log.Fatal(err)
	}

	s.Info = true
	s.Debug = true

	err = s.Start(host, port)
	if err != nil {
		log.Fatal(err)
	}
}
