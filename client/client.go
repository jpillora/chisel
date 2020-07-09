package chclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jpillora/backoff"
	chshare "github.com/jpillora/chisel/share"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
	"golang.org/x/sync/errgroup"
)

//Config represents a client configuration
type Config struct {
	shared           *chshare.Config
	Fingerprint      string
	Auth             string
	KeepAlive        time.Duration
	MaxRetryCount    int
	MaxRetryInterval time.Duration
	Server           string
	Proxy            string
	Remotes          []string
	Headers          http.Header
	DialContext      func(ctx context.Context, network, addr string) (net.Conn, error)
	Parent           context.Context
}

//Client represents a client instance
type Client struct {
	*chshare.Logger
	config    *Config
	sshConfig *ssh.ClientConfig
	proxyURL  *url.URL
	server    string
	connStats chshare.ConnStats
	stop      func()
	eg        *errgroup.Group
	tunnel    *chshare.Tunnel
}

//NewClient creates a new client instance
func NewClient(config *Config) (*Client, error) {
	//apply default scheme
	if !strings.HasPrefix(config.Server, "http") {
		config.Server = "http://" + config.Server
	}
	if config.MaxRetryInterval < time.Second {
		config.MaxRetryInterval = 5 * time.Minute
	}
	u, err := url.Parse(config.Server)
	if err != nil {
		return nil, err
	}
	//apply default port
	if !regexp.MustCompile(`:\d+$`).MatchString(u.Host) {
		if u.Scheme == "https" || u.Scheme == "wss" {
			u.Host = u.Host + ":443"
		} else {
			u.Host = u.Host + ":80"
		}
	}
	//swap to websockets scheme
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	shared := &chshare.Config{}
	hasReverse := false
	hasSocks := false
	hasStdio := false
	for _, s := range config.Remotes {
		r, err := chshare.DecodeRemote(s)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode remote '%s': %s", s, err)
		}
		if r.Socks {
			hasSocks = true
		}
		if r.Reverse {
			hasReverse = true
		}
		if r.Stdio {
			if hasStdio {
				return nil, errors.New("Only one stdio is allowed")
			}
			hasStdio = true
		}
		shared.Remotes = append(shared.Remotes, r)
	}
	config.shared = shared
	client := &Client{
		Logger: chshare.NewLogger("client"),
		config: config,
		server: u.String(),
	}
	//set default log level
	client.Logger.Info = true
	//outbound proxy
	if p := config.Proxy; p != "" {
		client.proxyURL, err = url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("Invalid proxy URL (%s)", err)
		}
	}
	//ssh auth and config
	user, pass := chshare.ParseAuth(config.Auth)
	client.sshConfig = &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		ClientVersion:   "SSH-" + chshare.ProtocolVersion + "-client",
		HostKeyCallback: client.verifyServer,
		Timeout:         30 * time.Second,
	}
	//prepare client tunnel
	client.tunnel = chshare.NewTunnel(chshare.TunnelConfig{
		Logger:   client.Logger,
		Inbound:  true, //client always accepts inbound
		Outbound: hasReverse,
		Socks:    hasReverse && hasSocks,
	})
	return client, nil
}

//Run starts client and blocks while connected
func (c *Client) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		return err
	}
	return c.Wait()
}

func (c *Client) verifyServer(hostname string, remote net.Addr, key ssh.PublicKey) error {
	expect := c.config.Fingerprint
	got := chshare.FingerprintKey(key)
	if expect != "" && !strings.HasPrefix(got, expect) {
		return fmt.Errorf("Invalid fingerprint (%s)", got)
	}
	//overwrite with complete fingerprint
	c.Infof("Fingerprint %s", got)
	return nil
}

//Start client and does not block
func (c *Client) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.stop = cancel
	eg, ctx := errgroup.WithContext(ctx)
	c.eg = eg
	via := ""
	if c.proxyURL != nil {
		via = " via " + c.proxyURL.String()
	}
	c.Infof("Connecting to %s%s\n", c.server, via)
	//connect chisel server
	eg.Go(func() error {
		return c.connectionLoop(ctx)
	})
	//listen sockets
	eg.Go(func() error {
		clientInbound := c.config.shared.Remotes.Reversed(false)
		return c.tunnel.BindRemotes(ctx, clientInbound)
	})
	return nil
}

func (c *Client) connectionLoop(ctx context.Context) error {
	//connection loop!
	b := &backoff.Backoff{Max: c.config.MaxRetryInterval}
	for {
		retry, err := c.connectionOnce(ctx)
		//connection error
		attempt := int(b.Attempt())
		maxAttempt := c.config.MaxRetryCount
		if err != nil {
			//show error and attempt counts
			msg := fmt.Sprintf("Connection error: %s", err)
			if attempt > 0 {
				msg += fmt.Sprintf(" (Attempt: %d", attempt)
				if maxAttempt > 0 {
					msg += fmt.Sprintf("/%d", maxAttempt)
				}
				msg += ")"
			}
			c.Debugf(msg)
		}
		//give up?
		if !retry || (maxAttempt >= 0 && attempt >= maxAttempt) {
			break
		}
		d := b.Duration()
		c.Infof("Retrying in %s...", d)
		chshare.SleepSignal(d)
	}
	c.Close()
	return nil
}

//connectionOnce returning nil error means retry
func (c *Client) connectionOnce(ctx context.Context) (retry bool, err error) {
	select {
	case <-ctx.Done():
		return false, io.EOF
	default:
		//still open
	}
	//prepare dialer
	d := websocket.Dialer{
		HandshakeTimeout: 45 * time.Second,
		Subprotocols:     []string{chshare.ProtocolVersion},
	}
	//optional proxy
	if p := c.proxyURL; p != nil {
		if err := c.setProxy(p, &d); err != nil {
			return false, err
		}
	}
	wsConn, _, err := d.DialContext(ctx, c.server, c.config.Headers)
	if err != nil {
		return true, err
	}
	conn := chshare.NewWebSocketConn(wsConn)
	// perform SSH handshake on net.Conn
	c.Debugf("Handshaking...")
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", c.sshConfig)
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			c.Infof("Authentication failed")
			c.Debugf(err.Error())
			retry = false
		} else {
			c.Infof(err.Error())
			retry = true
		}
		return retry, err
	}
	defer sshConn.Close()
	// chisel client handshake (reverse of server handshake)
	// send configuration
	c.config.shared.Version = chshare.BuildVersion
	conf, _ := chshare.EncodeConfig(c.config.shared)
	c.Debugf("Sending config")
	t0 := time.Now()
	_, configerr, err := sshConn.SendRequest("config", true, conf)
	if err != nil {
		c.Infof("Config verification failed")
		return false, err
	}
	if len(configerr) > 0 {
		return false, errors.New(string(configerr))
	}
	c.Infof("Connected (Latency %s)", time.Since(t0))
	defer c.Infof("Disconnected")
	//connected, handover ssh connection for tunnel to use, and block
	return true, c.tunnel.BindSSH(sshConn, reqs, chans)
}

func (c *Client) setProxy(u *url.URL, d *websocket.Dialer) error {
	if !strings.HasPrefix(u.Scheme, "socks") {
		// CONNECT proxy
		d.Proxy = func(*http.Request) (*url.URL, error) {
			return u, nil
		}
		return nil
	}
	// SOCKS5 proxy
	if u.Scheme != "socks" && u.Scheme != "socks5h" {
		return fmt.Errorf(
			"unsupported socks proxy type: %s:// (only socks5h:// or socks:// is supported)",
			u.Scheme,
		)
	}
	var auth *proxy.Auth
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = &proxy.Auth{
			User:     u.User.Username(),
			Password: pass,
		}
	}
	socksDialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return err
	}
	d.NetDial = socksDialer.Dial
	return nil
}

//Wait blocks while the client is running.
//Can only be called once.
func (c *Client) Wait() error {
	return c.eg.Wait()
}

//Close manually stops the client
func (c *Client) Close() error {
	if c.stop != nil {
		c.stop()
	}
	return nil
}
