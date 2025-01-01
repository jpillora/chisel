package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/valkyrie-io/connector-tunnel/client"
	"github.com/valkyrie-io/connector-tunnel/server"
	shared "github.com/valkyrie-io/connector-tunnel/shared"
	"github.com/valkyrie-io/connector-tunnel/shared/settings"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var help = `
  Usage: valkyrie [command] [--help]

  Version: ` + shared.BuildVersion + ` (` + runtime.Version() + `)

  Commands:
    initServer - runs valkyrie in server mode
    initClient - runs valkyrie in client mode
`

func main() {
	version := flag.Bool("version", false, "")
	flag.Bool("help", false, "")
	flag.Usage = func() {}
	flag.Parse()

	if *version {
		fmt.Println(shared.BuildVersion)
		os.Exit(0)
	}

	args := flag.Args()

	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "initServer":
		initServer(args)
	case "initClient":
		initClient(args)
	default:
		fmt.Print(help)
		os.Exit(0)
	}
}

var commonHelp = `
    -v, Enable verbose logging

    --help, This help text

  Version:
    ` + shared.BuildVersion + ` (` + runtime.Version() + `)
`

func generatePidFile() {
	pid := []byte(strconv.Itoa(os.Getpid()))
	if err := os.WriteFile("valkyrie.pid", pid, 0644); err != nil {
		log.Fatal(err)
	}
}

var serverHelp = `
  Usage: valkyrie initServer [options]

  Options:

    --host, Specifies the network interface for the HTTP initServer to listen on 
    (defaults to the HOST environment variable or 0.0.0.0 if not set).

    --port, -p, Sets the port for the HTTP initServer to listen on 
    (defaults to the PORT environment variable or 8080 if not specified).

    --keygen, Generates a new PEM-encoded SSH private key and writes it to the 
    specified file path. If you want to view the fingerprint of an existing key, 
    provide the path to that key using --key. Use a dash (-) to output the generated 
    key directly to stdout.

    --keyfile, Points to a PEM-encoded SSH private key file. If provided, this key 
    will be used for securing communications instead of generating a new key with --key. 
    This can also be set to an inline base64 private key for short keys. 
    Defaults to the VALKYRIE_KEY_FILE environment variable.

    --authfile, Specifies a path to a users.json file for user authentication. 
    The file should contain an object mapping user credentials to address regular 
    expressions, for example:
      {
        "<user:pass>": ["<addr-regex>", "<addr-regex>"]
      }
    Credentials will be verified, and address matching will use 
    "<remote-host>:<remote-port>" for standard remotes and 
    "R:<local-interface>:<local-port>" for reverse remotes. The file is 
    automatically reloaded upon changes.

    --keepalive, Configures an optional interval for sending keepalive messages. 
    Since HTTP transport may pass through proxies that close idle connections, 
    this setting helps maintain the connection. Specify a duration, such as '5s' 
    or '2m'. Defaults to '25s' (set to 0s to disable).

    --reverse, Enables clients to configure reverse port forwarding in addition 
    to standard remotes.

    --tls-key, Activates TLS and specifies the path to a PEM-encoded private key 
    for encryption. When using this option, you must also set --tls-cert and cannot 
    use --tls-domain.

    --tls-cert, Activates TLS and specifies the path to a PEM-encoded certificate 
    that matches the private key provided with --tls-key. This option is required 
    when --tls-key is set and cannot be used alongside --tls-domain.

` + commonHelp

func initServer(args []string) {

	flags := flag.NewFlagSet("initServer", flag.ContinueOnError)

	config := &chserver.Options{}
	flags.StringVar(&config.KeyFile, "keyfile", "", "")
	flags.StringVar(&config.AuthFile, "authfile", "", "")
	flags.DurationVar(&config.KeepAlive, "keepalive", 25*time.Second, "")
	flags.BoolVar(&config.Reverse, "reverse", true, "")
	flags.StringVar(&config.TLS.Key, "tls-key", "", "")
	flags.StringVar(&config.TLS.Cert, "tls-cert", "", "")
	flags.Var(multiFlag{&config.TLS.Domains}, "tls-domain", "")
	flags.StringVar(&config.TLS.CA, "tls-ca", "", "")

	host := flags.String("host", "", "")
	port := flags.String("port", "", "")
	verbose := flags.Bool("v", false, "")

	flags.Usage = func() {
		fmt.Print(serverHelp)
		os.Exit(0)
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
	if config.KeyFile == "" {
		config.KeyFile = settings.Env("KEY_FILE")
	}
	s, err := chserver.NewServer(config)
	if err != nil {
		log.Fatal(err)
	}
	s.Debug = *verbose

	if err := s.Start(*host, *port); err != nil {
		log.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		log.Fatal(err)
	}
}

type multiFlag struct {
	values *[]string
}

func (flag multiFlag) String() string {
	return strings.Join(*flag.values, ", ")
}

func (flag multiFlag) Set(arg string) error {
	*flag.values = append(*flag.values, arg)
	return nil
}

type headerFlags struct {
	http.Header
}

func (flag *headerFlags) String() string {
	out := ""
	for k, v := range flag.Header {
		out += fmt.Sprintf("%s: %s\n", k, v)
	}
	return out
}

func (flag *headerFlags) Set(arg string) error {
	index := strings.Index(arg, ":")
	if index < 0 {
		return fmt.Errorf(`Invalid header (%s). Should be in the format "HeaderName: HeaderContent"`, arg)
	}
	if flag.Header == nil {
		flag.Header = http.Header{}
	}
	key := arg[0:index]
	value := arg[index+1:]
	flag.Header.Set(key, strings.TrimSpace(value))
	return nil
}

var clientHelp = `
  Usage: valkyrie initClient [options] <valkyrie-URL> <remote> [remote] [remote] ...

  R:<local-interface>:<local-port>:<remote-host>:<remote-port>/<protocol>

  This sets up reverse port forwarding, making <remote-host>:<remote-port>
  accessible via the initServer’s <local-interface>:<local-port> from the initClient.

    Example remote configurations:
      R:2222:localhost:22

  Options:

    --auth, Specifies optional user credentials for initClient authentication 
    in the format "<user>:<pass>". These credentials are matched against 
    the initServer's --authfile. Defaults to the value of the AUTH environment 
    variable.

    --keepalive, Configures an optional keepalive interval to maintain 
    the connection. Since HTTP transport is often used, proxies may close 
    idle connections. Specify a duration with a unit, e.g., '5s' or '2m'. 
    Defaults to '25s' (use '0s' to disable).

    --max-retry-count, Sets the maximum number of retries before exiting. 
    The default is unlimited retries.

    --max-retry-interval, Defines the maximum time to wait before 
    attempting to reconnect after a disconnection. Defaults to 5 minutes.

    --header, Adds custom headers in the format "HeaderName: HeaderContent". 
    This option can be repeated, e.g., --header "Foo: Bar" --header "Hello: World".

    --hostname, Sets the 'Host' header explicitly (default is the host 
    from the initServer URL).

    --sni, Overrides the ServerName in TLS connections (default is 
    the hostname).

    --tls-ca, Specifies a root certificate bundle for verifying the 
    valkyrie initServer. This is only applicable when connecting via "https" 
    or "wss". By default, system CAs are used.

    --tls-key, Specifies the path to a PEM-encoded private key for initClient 
    authentication (mutual TLS).

    --tls-cert, Specifies the path to a PEM-encoded certificate that 
    matches the private key provided. The certificate must support 
    initClient authentication (mutual TLS).
` + commonHelp

func initClient(args []string) {
	flags := flag.NewFlagSet("initClient", flag.ContinueOnError)
	config := parseFlags(flags)
	hostname := flags.String("hostname", "", "")
	sni := flags.String("sni", "", "")
	verbose := flags.Bool("v", false, "")
	flags.Usage = func() {
		fmt.Print(clientHelp)
		os.Exit(0)
	}
	flags.Parse(args)
	//pull out options, put back remaining args
	args = flags.Args()
	if len(args) < 2 {
		log.Fatalf("A initServer and least one remote is required")
	}
	config.Server = args[0]
	config.Remotes = args[1:]
	//default auth
	if config.Auth == "" {
		config.Auth = os.Getenv("AUTH")
	}
	//move hostname onto headers
	if *hostname != "" {
		config.Headers.Set("Host", *hostname)
		config.TLS.ServerName = *hostname
	}

	if *sni != "" {
		config.TLS.ServerName = *sni
	}

	//ready
	c, err := client.NewClient(&config)
	if err != nil {
		log.Fatal(err)
	}
	c.Debug = *verbose

	if err := c.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	if err := c.Wait(); err != nil {
		log.Fatal(err)
	}
}

func parseFlags(flags *flag.FlagSet) client.Config {
	config := client.Config{Headers: http.Header{}}
	flags.StringVar(&config.Fingerprint, "fingerprint", "", "")
	flags.StringVar(&config.Auth, "auth", "", "")
	flags.DurationVar(&config.KeepAlive, "keepalive", 25*time.Second, "")
	flags.IntVar(&config.MaxRetryCount, "max-retry-count", -1, "")
	flags.DurationVar(&config.MaxRetryInterval, "max-retry-interval", 0, "")
	flags.StringVar(&config.TLS.CA, "tls-ca", "", "")
	flags.StringVar(&config.TLS.Cert, "tls-cert", "", "")
	flags.StringVar(&config.TLS.Key, "tls-key", "", "")
	flags.Var(&headerFlags{config.Headers}, "header", "")
	return config
}
