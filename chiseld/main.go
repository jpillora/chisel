package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jpillora/chisel/chiseld/server"
)

const help = `
	Usage: chiseld [--host 0.0.0.0] [--port 8080] [--auth str]

	host defines the HTTP listening host – the
	network interface (defaults to 0.0.0.0). You
	may also set the HOST environment variable.

	port defines the HTTP listening port (defaults
	to 8080). You may also set the PORT environment
	variable.

	auth specifies the exact authentication string
	the client must provide to attain access. You
	may also set the AUTH environment variable.

	Read more:
	https://github.com/jpillora/chisel

`

func main() {

	hostf := flag.String("host", "", "")
	portf := flag.String("port", "", "")
	authf := flag.String("auth", "", "")
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

	server.NewServer(auth).Start(host, port)
}
