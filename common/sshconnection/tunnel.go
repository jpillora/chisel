package sshconnection

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/valkyrie-io/connector-tunnel/common/logging"
	"github.com/valkyrie-io/connector-tunnel/common/netext"
	"github.com/valkyrie-io/connector-tunnel/common/settings"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// Config a SSHConnection
type Config struct {
	*logging.Logger
	Inbound   bool
	Outbound  bool
	KeepAlive time.Duration
}

// Both client and server are Tunnels.
// client has a single set of remotes, whereas
// server has multiple sets of remotes (one set per client).
// Each remote has a 1:1 mapping to a proxy.
// Proxies listen, send data over ssh, and the other end of the ssh connection
// communicates with the endpoint and returns the response.
type SSHConnection struct {
	Config
	activeConnMut  sync.RWMutex
	activatingConn waitGroup
	activeConn     ssh.Conn
	proxyCount     int
	connStats      netext.ConnCount
}

func New(c Config) *SSHConnection {
	c.Logger = c.Logger.Fork("SSHConnection")
	t := &SSHConnection{
		Config: c,
	}
	t.activatingConn.Add(1)
	return t
}

// Bind provides an active SSH for use for tunnelling
func (t *SSHConnection) Bind(ctx context.Context, c ssh.Conn, reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel) error {
	//link ctx to ssh-conn
	go t.linkCtxToSSHConnection(ctx, c)
	t.markActiveAndUnblock(c)
	t.keepAliveIfNeeded(c)
	err := t.blockSSHUntilClosed(reqs, chans, c)
	t.markSSHInactiveAndBlock()
	return err
}

func (t *SSHConnection) linkCtxToSSHConnection(ctx context.Context, c ssh.Conn) {
	<-ctx.Done()
	if c.Close() == nil {
		t.Debugf("SSH cancelled")
	}
	t.activatingConn.DoneAll()
}

func (t *SSHConnection) markActiveAndUnblock(c ssh.Conn) {
	t.activeConnMut.Lock()
	if t.activeConn != nil {
		panic("double bind ssh")
	}
	t.activeConn = c
	t.activeConnMut.Unlock()
	t.activatingConn.Done()
}

func (t *SSHConnection) keepAliveIfNeeded(c ssh.Conn) {
	if t.Config.KeepAlive > 0 {
		go t.keepAliveLoop(c)
	}
}

func (t *SSHConnection) blockSSHUntilClosed(reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel, c ssh.Conn) error {
	go t.handleSSHRequests(reqs)
	go t.handleSSHChannels(chans)
	t.Debugf("SSH connected")
	err := c.Wait()
	t.Debugf("SSH disconnected")
	return err
}

func (t *SSHConnection) markSSHInactiveAndBlock() {
	t.activatingConn.Add(1)
	t.activeConnMut.Lock()
	t.activeConn = nil
	t.activeConnMut.Unlock()
}

// getSSH blocks while connecting
func (t *SSHConnection) getSSH(ctx context.Context) ssh.Conn {
	conn, done := t.validateConn(ctx)
	if done {
		return conn
	}
	return t.connectSSH(ctx)
}

func (t *SSHConnection) validateConn(ctx context.Context) (ssh.Conn, bool) {
	//cancelled already?
	if isDone(ctx) {
		return nil, true
	}
	t.activeConnMut.RLock()
	c := t.activeConn
	t.activeConnMut.RUnlock()
	//connected already?
	if c != nil {
		return c, true
	}
	return nil, false
}

func (t *SSHConnection) connectSSH(ctx context.Context) ssh.Conn {
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

func (t *SSHConnection) activatingConnWait() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		t.activatingConn.Wait()
		close(ch)
	}()
	return ch
}

// ConnectRemotes converts the given remotes into proxies, and blocks
// until the caller cancels the context or there is a proxy error.
func (t *SSHConnection) ConnectRemotes(ctx context.Context, remotes []*settings.Remote) error {
	if len(remotes) == 0 {
		return errors.New("no remotes")
	}
	if !t.Inbound {
		return errors.New("inbound connections blocked")
	}
	proxies := make([]*SSHInbound, len(remotes))
	err2 := t.createRemoteProxies(remotes, proxies)
	if err2 != nil {
		return err2
	}
	//TODO: handle sshconnection close
	eg, ctx := errgroup.WithContext(ctx)
	return t.runTCPRemotes(ctx, proxies, eg)
}

func (t *SSHConnection) runTCPRemotes(ctx context.Context, proxies []*SSHInbound, eg *errgroup.Group) error {
	for _, proxy := range proxies {
		p := proxy
		eg.Go(func() error {
			return p.runTCP(ctx)
		})
	}
	t.Debugf("Bound proxies")
	err := eg.Wait()
	t.Debugf("Unbound proxies")
	return err
}

func (t *SSHConnection) createRemoteProxies(remotes []*settings.Remote, proxies []*SSHInbound) error {
	for i, remote := range remotes {
		err2 := t.createRemoteProxy(remote, proxies, i)
		if err2 != nil {
			return err2
		}
	}
	return nil
}

func (t *SSHConnection) createRemoteProxy(remote *settings.Remote, proxies []*SSHInbound, i int) error {
	p, err := newProxy(t.Logger, t, t.proxyCount, remote)
	if err != nil {
		return err
	}
	proxies[i] = p
	t.proxyCount++
	return nil
}

func (t *SSHConnection) keepAliveLoop(sshConn ssh.Conn) {
	//ping forever
	for {
		time.Sleep(t.Config.KeepAlive)
		_, b, err := sshConn.SendRequest("valkyrie-syn", true, nil)
		if err != nil {
			break
		}
		if len(b) > 0 && !bytes.Equal(b, []byte("valkyrie-ack")) {
			t.Debugf("strange valkyrie-ack response")
			break
		}
	}
	//close ssh connection on abnormal ping
	sshConn.Close()
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
