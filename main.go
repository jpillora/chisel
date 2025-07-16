package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	chshare "github.com/jpillora/chisel/share"
	"github.com/jpillora/chisel/share/ccrypto"
	"github.com/jpillora/chisel/share/cos"
	"github.com/jpillora/chisel/share/settings"
)

var help = `
  Usage: chisel [command] [--help]

  Version: ` + chshare.BuildVersion + ` (` + runtime.Version() + `)

  Commands:
    server - runs chisel in server mode
    client - runs chisel in client mode

  Read more:
    https://github.com/jpillora/chisel

`

func main() {

	version := flag.Bool("version", false, "")
	v := flag.Bool("v", false, "")
	flag.Bool("help", false, "")
	flag.Bool("h", false, "")
	flag.Usage = func() {}
	flag.Parse()

	if *version || *v {
		fmt.Println(chshare.BuildVersion)
		os.Exit(0)
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
		fmt.Print(help)
		os.Exit(0)
	}
}

var commonHelp = `
    --pid Generate pid file in current working directory

    -v, Enable verbose logging

    --help, This help text

  Signals:
    The chisel process is listening for:
      a SIGUSR2 to print process stats, and
      a SIGHUP to short-circuit the client reconnect timer

  Version:
    ` + chshare.BuildVersion + ` (` + runtime.Version() + `)

  Read more:
    https://github.com/jpillora/chisel

`

func generatePidFile() {
	pid := []byte(strconv.Itoa(os.Getpid()))
	if err := os.WriteFile("chisel.pid", pid, 0644); err != nil {
		log.Fatal(err)
	}
}

var serverHelp = `
  Usage: chisel server [options]

  Options:

    --host, Defines the HTTP listening host – the network interface
    (defaults the environment variable HOST and falls back to 0.0.0.0).

    --port, -p, Defines the HTTP listening port (defaults to the environment
    variable PORT and fallsback to port 8080).

    --key, (deprecated use --keygen and --keyfile instead)
    An optional string to seed the generation of a ECDSA public
    and private key pair. All communications will be secured using this
    key pair. Share the subsequent fingerprint with clients to enable detection
    of man-in-the-middle attacks (defaults to the CHISEL_KEY environment
    variable, otherwise a new key is generate each run).

    --keygen, A path to write a newly generated PEM-encoded SSH private key file.
    If users depend on your --key fingerprint, you may also include your --key to
    output your existing key. Use - (dash) to output the generated key to stdout.

    --keyfile, An optional path to a PEM-encoded SSH private key. When
    this flag is set, the --key option is ignored, and the provided private key
    is used to secure all communications. (defaults to the CHISEL_KEY_FILE
    environment variable). Since ECDSA keys are short, you may also set keyfile
    to an inline base64 private key (e.g. chisel server --keygen - | base64).

    --authfile, An optional path to a users.json file. This file should
    be an object with users defined like:
      {
        "<user:pass>": ["<addr-regex>","<addr-regex>"]
      }
    when <user> connects, their <pass> will be verified and then
    each of the remote addresses will be compared against the list
    of address regular expressions for a match. Addresses will
    always come in the form "<remote-host>:<remote-port>" for normal remotes
    and "R:<local-interface>:<local-port>" for reverse port forwarding
    remotes. This file will be automatically reloaded on change.

    --auth, An optional string representing a single user with full
    access, in the form of <user:pass>. It is equivalent to creating an
    authfile with {"<user:pass>": [""]}. If unset, it will use the
    environment variable AUTH.

    --keepalive, An optional keepalive interval. Since the underlying
    transport is HTTP, in many instances we'll be traversing through
    proxies, often these proxies will close idle connections. You must
    specify a time with a unit, for example '5s' or '2m'. Defaults
    to '25s' (set to 0s to disable).

    --backend, Specifies another HTTP server to proxy requests to when
    chisel receives a normal HTTP request. Useful for hiding chisel in
    plain sight.

    --socks5, Allow clients to access the internal SOCKS5 proxy. See
    chisel client --help for more information.

    --reverse, Allow clients to specify reverse port forwarding remotes
    in addition to normal remotes.

    --tls-key, Enables TLS and provides optional path to a PEM-encoded
    TLS private key. When this flag is set, you must also set --tls-cert,
    and you cannot set --tls-domain.

    --tls-cert, Enables TLS and provides optional path to a PEM-encoded
    TLS certificate. When this flag is set, you must also set --tls-key,
    and you cannot set --tls-domain.

    --tls-domain, Enables TLS and automatically acquires a TLS key and
    certificate using LetsEncrypt. Setting --tls-domain requires port 443.
    You may specify multiple --tls-domain flags to serve multiple domains.
    The resulting files are cached in the "$HOME/.cache/chisel" directory.
    You can modify this path by setting the CHISEL_LE_CACHE variable,
    or disable caching by setting this variable to "-". You can optionally
    provide a certificate notification email by setting CHISEL_LE_EMAIL.

    --tls-ca, a path to a PEM encoded CA certificate bundle or a directory
    holding multiple PEM encode CA certificate bundle files, which is used to 
    validate client connections. The provided CA certificates will be used 
    instead of the system roots. This is commonly used to implement mutual-TLS. 
` + commonHelp

func server(args []string) {

	flags := flag.NewFlagSet("server", flag.ContinueOnError)

	config := &chserver.Config{}
	flags.StringVar(&config.KeySeed, "key", "", "")
	flags.StringVar(&config.KeyFile, "keyfile", "", "")
	flags.StringVar(&config.AuthFile, "authfile", "", "")
	flags.StringVar(&config.Auth, "auth", "", "")
	flags.DurationVar(&config.KeepAlive, "keepalive", 25*time.Second, "")
	flags.StringVar(&config.Proxy, "proxy", "", "")
	flags.StringVar(&config.Proxy, "backend", "", "")
	flags.BoolVar(&config.Socks5, "socks5", false, "")
	flags.BoolVar(&config.Reverse, "reverse", false, "")
	flags.StringVar(&config.TLS.Key, "tls-key", "", "")
	flags.StringVar(&config.TLS.Cert, "tls-cert", "", "")
	flags.Var(multiFlag{&config.TLS.Domains}, "tls-domain", "")
	flags.StringVar(&config.TLS.CA, "tls-ca", "", "")

	host := flags.String("host", "", "")
	p := flags.String("p", "", "")
	port := flags.String("port", "", "")
	pid := flags.Bool("pid", false, "")
	verbose := flags.Bool("v", false, "")
	keyGen := flags.String("keygen", "", "")

	flags.Usage = func() {
		fmt.Print(serverHelp)
		os.Exit(0)
	}
	flags.Parse(args)

	if *keyGen != "" {
		if err := ccrypto.GenerateKeyFile(*keyGen, config.KeySeed); err != nil {
			log.Fatal(err)
		}
		return
	}

	if config.KeySeed != "" {
		log.Print("Option `--key` is deprecated and will be removed in a future version of chisel.")
		log.Print("Please use `chisel server --keygen /file/path`, followed by `chisel server --keyfile /file/path` to specify the SSH private key")
	}

	if *host == "" {
		*host = os.Getenv("HOST")
	}
	if *host == "" {
		*host = "0.0.0.0"
	}
	if *port == "" {
		*port = *p
	}
	if *port == "" {
		*port = os.Getenv("PORT")
	}
	if *port == "" {
		*port = "8080"
	}
	if config.KeyFile == "" {
		config.KeyFile = settings.Env("KEY_FILE")
	} else if config.KeySeed == "" {
		config.KeySeed = settings.Env("KEY")
	}
	if config.Auth == "" {
		config.Auth = os.Getenv("AUTH")
	}
	s, err := chserver.NewServer(config)
	if err != nil {
		log.Fatal(err)
	}
	s.Debug = *verbose
	if *pid {
		generatePidFile()
	}
	go cos.GoStats()
	ctx := cos.InterruptContext()
	if err := s.StartContext(ctx, *host, *port); err != nil {
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
  Usage: chisel client [options] <server> <remote> [remote] [remote] ...

  <server> is the URL to the chisel server.

  <remote>s are remote connections tunneled through the server, each of
  which come in the form:

    <local-host>:<local-port>:<remote-host>:<remote-port>/<protocol>

    ■ local-host defaults to 0.0.0.0 (all interfaces).
    ■ local-port defaults to remote-port.
    ■ remote-port is required*.
    ■ remote-host defaults to 0.0.0.0 (server localhost).
    ■ protocol defaults to tcp.

  which shares <remote-host>:<remote-port> from the server to the client
  as <local-host>:<local-port>, or:

    R:<local-interface>:<local-port>:<remote-host>:<remote-port>/<protocol>

  which does reverse port forwarding, sharing <remote-host>:<remote-port>
  from the client to the server's <local-interface>:<local-port>.

    example remotes

      3000
      example.com:3000
      3000:google.com:80
      192.168.0.5:3000:google.com:80
      socks
      5000:socks
      R:2222:localhost:22
      R:socks
      R:5000:socks
      stdio:example.com:22
      1.1.1.1:53/udp

    When the chisel server has --socks5 enabled, remotes can
    specify "socks" in place of remote-host and remote-port.
    The default local host and port for a "socks" remote is
    127.0.0.1:1080. Connections to this remote will terminate
    at the server's internal SOCKS5 proxy.

    When the chisel server has --reverse enabled, remotes can
    be prefixed with R to denote that they are reversed. That
    is, the server will listen and accept connections, and they
    will be proxied through the client which specified the remote.
    Reverse remotes specifying "R:socks" will listen on the server's
    default socks port (1080) and terminate the connection at the
    client's internal SOCKS5 proxy.

    When stdio is used as local-host, the tunnel will connect standard
    input/output of this program with the remote. This is useful when 
    combined with ssh ProxyCommand. You can use
      ssh -o ProxyCommand='chisel client chiselserver stdio:%h:%p' \
          user@example.com
    to connect to an SSH server through the tunnel.

  Options:

    --fingerprint, A *strongly recommended* fingerprint string
    to perform host-key validation against the server's public key.
	Fingerprint mismatches will close the connection.
	Fingerprints are generated by hashing the ECDSA public key using
	SHA256 and encoding the result in base64.
	Fingerprints must be 44 characters containing a trailing equals (=).

    --auth, An optional username and password (client authentication)
    in the form: "<user>:<pass>". These credentials are compared to
    the credentials inside the server's --authfile. defaults to the
    AUTH environment variable.

    --keepalive, An optional keepalive interval. Since the underlying
    transport is HTTP, in many instances we'll be traversing through
    proxies, often these proxies will close idle connections. You must
    specify a time with a unit, for example '5s' or '2m'. Defaults
    to '25s' (set to 0s to disable).

    --max-retry-count, Maximum number of times to retry before exiting.
    Defaults to unlimited.

    --max-retry-interval, Maximum wait time before retrying after a
    disconnection. Defaults to 5 minutes.

    --proxy, An optional HTTP CONNECT or SOCKS5 proxy which will be
    used to reach the chisel server. Authentication can be specified
    inside the URL.
    For example, http://admin:password@my-server.com:8081
            or: socks://admin:password@my-server.com:1080

    --header, Set a custom header in the form "HeaderName: HeaderContent".
    Can be used multiple times. (e.g --header "Foo: Bar" --header "Hello: World")

    --hostname, Optionally set the 'Host' header (defaults to the host
    found in the server url).

    --sni, Override the ServerName when using TLS (defaults to the 
    hostname).

    --tls-ca, An optional root certificate bundle used to verify the
    chisel server. Only valid when connecting to the server with
    "https" or "wss". By default, the operating system CAs will be used.

    --tls-skip-verify, Skip server TLS certificate verification of
    chain and host name (if TLS is used for transport connections to
    server). If set, client accepts any TLS certificate presented by
    the server and any host name in that certificate. This only affects
    transport https (wss) connection. Chisel server's public key
    may be still verified (see --fingerprint) after inner connection
    is established.

    --tls-key, a path to a PEM encoded private key used for client 
    authentication (mutual-TLS).

    --tls-cert, a path to a PEM encoded certificate matching the provided 
    private key. The certificate must have client authentication 
    enabled (mutual-TLS).
` + commonHelp

func client(args []string) {
	flags := flag.NewFlagSet("client", flag.ContinueOnError)
	config := chclient.Config{Headers: http.Header{}}
	flags.StringVar(&config.Fingerprint, "fingerprint", "", "")
	flags.StringVar(&config.Auth, "auth", "", "")
	flags.DurationVar(&config.KeepAlive, "keepalive", 25*time.Second, "")
	flags.IntVar(&config.MaxRetryCount, "max-retry-count", -1, "")
	flags.DurationVar(&config.MaxRetryInterval, "max-retry-interval", 0, "")
	flags.StringVar(&config.Proxy, "proxy", "", "")
	flags.StringVar(&config.TLS.CA, "tls-ca", "", "")
	flags.BoolVar(&config.TLS.SkipVerify, "tls-skip-verify", false, "")
	flags.StringVar(&config.TLS.Cert, "tls-cert", "", "")
	flags.StringVar(&config.TLS.Key, "tls-key", "", "")
	flags.Var(&headerFlags{config.Headers}, "header", "")
	hostname := flags.String("hostname", "", "")
	sni := flags.String("sni", "", "")
	pid := flags.Bool("pid", false, "")
	verbose := flags.Bool("v", false, "")
	flags.Usage = func() {
		fmt.Print(clientHelp)
		os.Exit(0)
	}
	flags.Parse(args)
	//pull out options, put back remaining args
	args = flags.Args()
	if len(args) < 2 {
		log.Fatalf("A server and least one remote is required")
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
	c, err := chclient.NewClient(&config)
	if err != nil {
		log.Fatal(err)
	}
	c.Debug = *verbose
	if *pid {
		generatePidFile()
	}
	go cos.GoStats()
	ctx := cos.InterruptContext()
	if err := c.Start(ctx); err != nil {
		log.Fatal(err)
	}
	if err := c.Wait(); err != nil {
		log.Fatal(err)
	}
}
