package main

import (
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
	"github.com/jpillora/opts"
)

// chiselRepo is rendered in the "Read more:" section of every help screen.
const chiselRepo = "https://github.com/jpillora/chisel"

// versionTemplate keeps the Go runtime version alongside the build version in
// the help text. The --version flag still prints the build version alone.
var versionTemplate = "\n{{bold \"Version:\"}}\n{{.Pad}}{{accent \"" +
	chshare.BuildVersion + " (" + runtime.Version() + ")\"}}\n"

type rootOptions struct{}

func main() {
	serverConfig := newServerOptions()
	clientConfig := newClientOptions()
	cli := newCLI(serverConfig, clientConfig)

	parsed, err := cli.ParseArgsError(os.Args)
	if err != nil {
		// opts deliberately models help and version as successful parse exits.
		// Keep Chisel's historical stdout and trailing-newline behaviour.
		if isHelpExit(err, os.Args[1:]) {
			fmt.Print(err)
			return
		}
		if err.Error() == chshare.BuildVersion && hasRootVersionFlag(os.Args[1:]) {
			fmt.Println(err)
			return
		}
		fmt.Fprint(os.Stderr, parsed.Selected().Help())
		os.Exit(1)
	}
	if !parsed.IsRunnable() {
		fmt.Print(parsed.Help())
		return
	}
	if err := parsed.Run(); err != nil {
		log.Fatal(err)
	}
}

func newCLI(serverConfig *serverOptions, clientConfig *clientOptions) opts.Opts {
	return opts.New(&rootOptions{}).
		Name("chisel").
		Version(chshare.BuildVersion).
		Repo(chiselRepo).
		DocSet("version", versionTemplate).
		AddCommand(newServerCLI(serverConfig)).
		AddCommand(newClientCLI(clientConfig))
}

func newServerCLI(serverConfig *serverOptions) opts.Opts {
	return opts.New(serverConfig).
		Name("server").
		Summary("runs chisel in server mode").
		Repo(chiselRepo).
		DocSet("version", versionTemplate).
		DocAfter("flaggroups", "signals", signalsTemplate)
}

func newClientCLI(clientConfig *clientOptions) opts.Opts {
	return opts.New(clientConfig).
		Name("client").
		Summary("runs chisel in client mode").
		Repo(chiselRepo).
		DocSet("version", versionTemplate).
		DocAfter("summary", "remotes", remotesTemplate).
		DocAfter("flaggroups", "signals", signalsTemplate)
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-help" || arg == "--h" || arg == "-h" {
			return true
		}
	}
	return false
}

func isHelpExit(err error, args []string) bool {
	message := err.Error()
	return hasHelpFlag(args) && strings.Contains(message, "Usage:") && !strings.Contains(message, "Error:")
}

func hasRootVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			return false
		}
		if arg == "--version" || arg == "-version" || arg == "--v" || arg == "-v" {
			return true
		}
	}
	return false
}

func optsTagValue(tag, key string) string {
	for _, part := range strings.Split(tag, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

// signalsTemplate documents the signals both commands handle. The layout is
// deliberate, so it is rendered verbatim rather than reflowed.
const signalsTemplate = "\n{{bold \"Signals:\"}}\n" +
	"{{.Pad}}The chisel process is listening for:\n" +
	"{{.Pad}}  a SIGINT or SIGTERM to begin a graceful shutdown\n" +
	"{{.Pad}}    (a second signal forces an immediate exit),\n" +
	"{{.Pad}}  a SIGUSR2 to print process stats, and\n" +
	"{{.Pad}}  a SIGHUP to short-circuit the client reconnect timer\n"

func generatePidFile() error {
	pid := []byte(strconv.Itoa(os.Getpid()))
	return os.WriteFile("chisel.pid", pid, 0644)
}

type sharedStringFlag struct {
	target *string
}

func (f sharedStringFlag) String() string {
	if f.target == nil {
		return ""
	}
	return *f.target
}

func (f *sharedStringFlag) Set(value string) error {
	*f.target = value
	return nil
}

// Field order determines the order flags appear in the help text.
type serverOptions struct {
	Host   string `opts:"name=host,short=-" help:"Defines the HTTP listening host – the network interface (defaults the environment variable HOST and falls back to 0.0.0.0)."`
	Port   string `opts:"name=port,short=p" help:"Defines the HTTP listening port (defaults to the environment variable PORT and falls back to port 8080)."`
	KeyGen string `opts:"name=keygen,short=-" help:"A path to write a newly generated PEM-encoded SSH private key file. If users depend on your --key fingerprint, you may also include your --key to output your existing key. Use - (dash) to output the generated key to stdout."`
	chserver.Config
	ProxyAlias sharedStringFlag `opts:"name=proxy,mode=flag,short=-" help:"Alias for --backend."`
	PID        bool             `opts:"name=pid,short=-" help:"Generate pid file in current working directory"`
	Verbose    bool             `opts:"name=verbose,short=v" help:"Enable verbose logging"`
}

func newServerOptions() *serverOptions {
	o := &serverOptions{}
	o.Config.KeepAlive = 25 * time.Second
	o.ProxyAlias.target = &o.Config.Proxy
	return o
}

func (o *serverOptions) Run() error {
	config := &o.Config

	if o.KeyGen != "" {
		return ccrypto.GenerateKeyFile(o.KeyGen, config.KeySeed)
	}

	if config.KeySeed != "" {
		log.Print("Option `--key` is deprecated and will be removed in a future version of chisel.")
		log.Print("Please use `chisel server --keygen /file/path`, followed by `chisel server --keyfile /file/path` to specify the SSH private key")
	}

	if o.Host == "" {
		o.Host = os.Getenv("HOST")
	}
	if o.Host == "" {
		o.Host = "0.0.0.0"
	}
	if o.Port == "" {
		o.Port = os.Getenv("PORT")
	}
	if o.Port == "" {
		o.Port = "8080"
	}
	if config.KeyFile == "" {
		config.KeyFile = settings.Env("KEY_FILE")
	}
	if config.KeySeed == "" {
		config.KeySeed = settings.Env("KEY")
	}
	if config.Auth == "" {
		config.Auth = os.Getenv("AUTH")
	}
	s, err := chserver.NewServer(config)
	if err != nil {
		return err
	}
	s.Debug = o.Verbose
	if o.PID {
		if err := generatePidFile(); err != nil {
			return err
		}
	}
	go cos.GoStats()
	ctx := cos.InterruptContext()
	if err := s.StartContext(ctx, o.Host, o.Port); err != nil {
		return err
	}
	return s.Wait()
}

func setHeader(headers http.Header, arg string) error {
	index := strings.Index(arg, ":")
	if index < 0 {
		return fmt.Errorf(`Invalid header (%s). Should be in the format "HeaderName: HeaderContent"`, arg)
	}
	key := arg[0:index]
	value := arg[index+1:]
	headers.Set(key, strings.TrimSpace(value))
	return nil
}

// remotesTemplate documents the <remote> syntax. Its bullet lists, examples and
// ProxyCommand snippet are laid out by hand, so it is rendered verbatim.
const remotesTemplate = `
<server> is the URL to the chisel server.

<remote>s are remote connections tunneled through the server, each of
which come in the form:

  <local-host>:<local-port>:<remote-host>:<remote-port>/<protocol>

  ■ local-host defaults to 0.0.0.0 (all interfaces).
  ■ local-port defaults to remote-port.
  ■ remote-port is required*.
  ■ remote-host defaults to 127.0.0.1 (server localhost).
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
  at the server's internal SOCKS5 proxy. When the server also
  has --authfile set, SOCKS5 access requires an entry matching
  the token "socks" in the user's address list.

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
`

// Field order determines the order flags appear in the help text.
type clientOptions struct {
	chclient.Config
	Header   []string `opts:"name=header,short=-" help:"Set a custom header in the form \"HeaderName: HeaderContent\". Can be used multiple times. (e.g --header \"Foo: Bar\" --header \"Hello: World\")"`
	Hostname string   `opts:"name=hostname,short=-" help:"Optionally set the 'Host' header (defaults to the host found in the server url)."`
	SNI      string   `opts:"name=sni,short=-" help:"Override the ServerName when using TLS (defaults to the hostname)."`
	PID      bool     `opts:"name=pid,short=-" help:"Generate pid file in current working directory"`
}

func newClientOptions() *clientOptions {
	o := &clientOptions{}
	o.Config.KeepAlive = 25 * time.Second
	o.Config.MaxRetryCount = -1
	o.Config.Headers = http.Header{}
	return o
}

func (o *clientOptions) Run() error {
	config := &o.Config
	for _, header := range o.Header {
		if err := setHeader(config.Headers, header); err != nil {
			return err
		}
	}
	//default auth
	if config.Auth == "" {
		config.Auth = os.Getenv("AUTH")
	}
	//move hostname onto headers
	if o.Hostname != "" {
		config.Headers.Set("Host", o.Hostname)
		config.TLS.ServerName = o.Hostname
	}

	if o.SNI != "" {
		config.TLS.ServerName = o.SNI
	}

	//ready
	c, err := chclient.NewClient(config)
	if err != nil {
		return err
	}
	c.Debug = config.Verbose
	if o.PID {
		if err := generatePidFile(); err != nil {
			return err
		}
	}
	go cos.GoStats()
	ctx := cos.InterruptContext()
	if err := c.Start(ctx); err != nil {
		return err
	}
	return c.Wait()
}
