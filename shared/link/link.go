package link

import (
	"bytes"
	"context"
	"errors"
	"github.com/valkyrie-io/connector-tunnel/shared"
	"sync"
	"time"

	"github.com/valkyrie-io/connector-tunnel/shared/cnet"
	"github.com/valkyrie-io/connector-tunnel/shared/settings"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// Options a Link
type Options struct {
	*shared.Logger
	AllowInbound  bool
	AllowOutbound bool
	Heartbeat     time.Duration
}

// Link represents an SSH link with proxy capabilities.
// Both chisel client and server are Tunnels.
// chisel client has a single set of remotes, whereas
// chisel server has multiple sets of remotes (one set per client).
// Each remote has a 1:1 mapping to a proxy.
// Proxies listen, send data over ssh, and the other end of the ssh connection
// communicates with the endpoint and returns the response.
type Link struct {
	Options
	//ssh connection
	activeConnMut  sync.RWMutex
	activatingConn waitGroup
	activeConn     ssh.Conn
	//proxies
	proxyCount int
	//internals
	connStats cnet.ConnCount
}

// New Link from the given Options
func New(c Options) *Link {
	c.Logger = c.Logger.Fork("link")
	t := &Link{
		Options: c,
	}
	t.activatingConn.Add(1)
	return t
}

// BindSSH provides an active SSH for use for tunnelling
func (t *Link) BindSSH(ctx context.Context, c ssh.Conn, reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel) error {
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
	if t.Options.Heartbeat > 0 {
		go t.heartBeatLoop(c)
	}
	//block until closed
	go t.processSSHRequests(reqs)
	go t.processSSHChannels(chans)
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
func (t *Link) get(ctx context.Context) ssh.Conn {
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

func (t *Link) activatingConnWait() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		t.activatingConn.Wait()
		close(ch)
	}()
	return ch
}

// BindRemotes converts the given remotes into proxies, and blocks
// until the caller cancels the context or there is a proxy error.
func (t *Link) BindRemotes(ctx context.Context, remotes []*settings.Remote) error {
	if len(remotes) == 0 {
		return errors.New("no remotes")
	}
	if !t.AllowInbound {
		return errors.New("inbound connections blocked")
	}
	proxies := make([]*Gate, len(remotes))
	for i, remote := range remotes {
		p, err := InitGate(t.Logger, t, t.proxyCount, remote)
		if err != nil {
			return err
		}
		proxies[i] = p
		t.proxyCount++
	}
	//TODO: handle link close
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

func (t *Link) heartBeatLoop(sshConn ssh.Conn) {
	//ping forever
	for {
		time.Sleep(t.Options.Heartbeat)
		_, b, err := sshConn.SendRequest("hello-request", true, nil)
		if err != nil {
			break
		}
		if len(b) > 0 && !bytes.Equal(b, []byte("hello-reply")) {
			t.Debugf("strange hello reply")
			break
		}
	}
	//close ssh connection on abnormal ping
	sshConn.Close()
}
