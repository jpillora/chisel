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
	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

//Config a Tunnel
type Config struct {
	*cio.Logger
	Inbound   bool
	Outbound  bool
	Socks     bool
	KeepAlive time.Duration
}

//Tunnel represents an SSH tunnel with proxy capabilities.
//Both chisel client and server are Tunnels.
//chisel client has a single set of remotes, whereas
//chisel server has multiple sets of remotes (one set per client).
//Each remote has a 1:1 mapping to a proxy.
//Proxies listen, send data over ssh, and the other end of the ssh connection
//communicates with the endpoint and returns the response.
type Tunnel struct {
	Config
	//ssh connection
	activeConnMut sync.RWMutex
	activeConn    ssh.Conn
	wgConn        sync.WaitGroup
	//proxies
	proxyCount int
	//internals
	connStats   cnet.ConnStats
	socksServer *socks5.Server
}

//New Tunnel from the given Config
func New(c Config) *Tunnel {
	t := &Tunnel{Config: c}
	//block getters
	t.wgConn.Add(1)
	//setup socks server (not listening on any port!)
	if c.Socks {
		sl := log.New(ioutil.Discard, "", 0)
		if t.Logger.Debug {
			sl = log.New(os.Stdout, "[socks]", log.Ldate|log.Ltime)
		}
		t.socksServer, _ = socks5.New(&socks5.Config{Logger: sl})
		t.Infof("SOCKS5 endpoint enabled")
	}
	return t
}

//BindSSH provides an active SSH for use for tunnelling
func (t *Tunnel) BindSSH(c ssh.Conn, reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel) error {
	//mark active
	t.activeConnMut.Lock()
	if t.activeConn != nil {
		panic("double bind ssh")
	}
	t.activeConn = c
	t.activeConnMut.Unlock()
	//unblock getters
	t.wgConn.Done()
	//optional keepalive loop against this connection
	if t.Config.KeepAlive > 0 {
		go t.keepAliveLoop(c)
	}
	//block until closed
	go t.handleSSHChannels(chans)
	err := c.Wait()
	//reblock getters
	t.wgConn.Add(1)
	//mark inactive
	t.activeConnMut.Lock()
	t.activeConn = nil
	t.activeConnMut.Unlock()
	return err
}

func (t *Tunnel) getSSH() ssh.Conn {
	t.wgConn.Wait() //TODO timeout?
	t.activeConnMut.RLock()
	c := t.activeConn
	t.activeConnMut.RUnlock()
	return c
}

//BindRemotes converts the given remotes into proxies, and blocks
//until the caller cancels the context or there is a proxy error.
func (t *Tunnel) BindRemotes(ctx context.Context, remotes []*config.Remote) error {
	if len(remotes) == 0 {
		return nil
	}
	if !t.Inbound {
		return errors.New("inbound connections blocked")
	}
	proxies := make([]*Proxy, len(remotes))
	for i, remote := range remotes {
		p, err := NewProxy(t.Logger, t.getSSH, t.proxyCount, remote)
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
	return eg.Wait()
}

func (t *Tunnel) keepAliveLoop(sshConn ssh.Conn) {
	for {
		time.Sleep(t.Config.KeepAlive)
		_, resp, err := sshConn.SendRequest("ping", true, nil)
		if err != nil {
			break
		}
		if !bytes.Equal(resp, []byte("pong")) {
			break
		}
	}
}
