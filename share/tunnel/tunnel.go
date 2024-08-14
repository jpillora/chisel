package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/armon/go-socks5"
	"github.com/OutSystems/chisel/share/cio"
	"github.com/OutSystems/chisel/share/cnet"
	"github.com/OutSystems/chisel/share/settings"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// Config a Tunnel
type Config struct {
	*cio.Logger
	Inbound   bool
	Outbound  bool
	Socks     bool
	KeepAlive time.Duration
}

// Tunnel represents an SSH tunnel with proxy capabilities.
// Both chisel client and server are Tunnels.
// chisel client has a single set of remotes, whereas
// chisel server has multiple sets of remotes (one set per client).
// Each remote has a 1:1 mapping to a proxy.
// Proxies listen, send data over ssh, and the other end of the ssh connection
// communicates with the endpoint and returns the response.
type Tunnel struct {
	Config
	//ssh connection
	activeConnMut  sync.RWMutex
	activatingConn waitGroup
	activeConn     ssh.Conn
	//proxies
	proxyCount int
	//internals
	connStats   cnet.ConnCount
	socksServer *socks5.Server
}

// New Tunnel from the given Config
func New(c Config) *Tunnel {
	c.Logger = c.Logger.Fork("tun")
	t := &Tunnel{
		Config: c,
	}
	t.activatingConn.Add(1)
	//setup socks server (not listening on any port!)
	extra := ""
	if c.Socks {
		sl := log.New(ioutil.Discard, "", 0)
		if t.Logger.Debug {
			sl = log.New(os.Stdout, "[socks]", log.Ldate|log.Ltime)
		}
		t.socksServer, _ = socks5.New(&socks5.Config{Logger: sl})
		extra += " (SOCKS enabled)"
	}
	t.Debugf("Created%s", extra)
	return t
}

// BindSSH provides an active SSH for use for tunnelling
func (t *Tunnel) BindSSH(ctx context.Context, c ssh.Conn, reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel) error {
	//link ctx to ssh-conn
	go func() {
		<-ctx.Done()
		if c.Close() == nil {
			t.Debugf("SSH cancelled")
		}
		t.activatingConn.DoneAll()
	}()
	//mark active and unblock
	t.activeConnMut.Lock()
	if t.activeConn != nil {
		panic("double bind ssh")
	}
	t.activeConn = c
	t.activeConnMut.Unlock()
	t.activatingConn.Done()
	//optional keepalive loop against this connection
	if t.Config.KeepAlive > 0 {
		go t.keepAliveLoop(c)
	}
	//block until closed
	go t.handleSSHRequests(reqs)
	go t.handleSSHChannels(chans)
	t.Debugf("SSH connected")
	err := c.Wait()
	t.Debugf("SSH disconnected")
	//mark inactive and block
	t.activatingConn.Add(1)
	t.activeConnMut.Lock()
	t.activeConn = nil
	t.activeConnMut.Unlock()
	return err
}

// getSSH blocks while connecting
func (t *Tunnel) getSSH(ctx context.Context) ssh.Conn {
	//cancelled already?
	if isDone(ctx) {
		return nil
	}
	t.activeConnMut.RLock()
	c := t.activeConn
	t.activeConnMut.RUnlock()
	//connected already?
	if c != nil {
		return c
	}
	//connecting...
	select {
	case <-ctx.Done(): //cancelled
		return nil
	case <-time.After(settings.EnvDuration("SSH_WAIT", 35*time.Second)):
		return nil //a bit longer than ssh timeout
	case <-t.activatingConnWait():
		t.activeConnMut.RLock()
		c := t.activeConn
		t.activeConnMut.RUnlock()
		return c
	}
}

func (t *Tunnel) activatingConnWait() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		t.activatingConn.Wait()
		close(ch)
	}()
	return ch
}

// BindRemotes converts the given remotes into proxies, and blocks
// until the caller cancels the context or there is a proxy error.
func (t *Tunnel) BindRemotes(ctx context.Context, remotes []*settings.Remote) error {
	if len(remotes) == 0 {
		return errors.New("no remotes")
	}
	if !t.Inbound {
		return errors.New("inbound connections blocked")
	}
	proxies := make([]*Proxy, len(remotes))
	for i, remote := range remotes {
		p, err := NewProxy(t.Logger, t, t.proxyCount, remote)
		if err != nil {
			return err
		}
		proxies[i] = p
		t.proxyCount++
	}
	//TODO: handle tunnel close
	eg, ctx := errgroup.WithContext(ctx)
	for _, proxy := range proxies {
		p := proxy
		eg.Go(func() error {
			return p.Run(ctx)
		})
	}
	t.Debugf("Bound proxies")
	err := eg.Wait()
	t.Debugf("Unbound proxies")
	return err
}

func (t *Tunnel) keepAliveLoop(sshConn ssh.Conn) {
	//ping forever

	//reply_timeout set to KeepAlive interval, if KeepAlive is less than 10s, set reply_timeout to 10s
	//maybe a new config option for reply_timeout is also fine
	reply_timeout := t.Config.KeepAlive

	if reply_timeout < 10*time.Second {
		reply_timeout = 10 * time.Second
	}

	for {
		time.Sleep(t.Config.KeepAlive)

		errChannel := make(chan error, 2)

		timeoutTimer := time.AfterFunc(reply_timeout, func() {
			errChannel <- errors.New("KEEPALIVE REPLY TIMEOUT ERROR")
		})

		go func() {
			_, b, err := sshConn.SendRequest("ping", true, nil)

			ret_err := err

			if err == nil && len(b) > 0 && !bytes.Equal(b, []byte("pong")) {
				t.Debugf("strange ping response")
				ret_err = errors.New("strange ping response")
			}

			errChannel <- ret_err
		}()

		err := <-errChannel

		// explicitly stop the timer, as it's no longer needed at this point.
		timeoutTimer.Stop()
		// errChannel should be garbage collected, as it won't be in use, even on a timeout,
		// 	as SendRequest will be unblocked on connection closure, the goroutine will send
		// 	 an error message and finish, therefore releasing the channel.

		if err != nil {
			break
		}
	}
	//close ssh connection on abnormal ping
	sshConn.Close()
}
