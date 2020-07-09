package chshare

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/armon/go-socks5"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

type TunnelConfig struct {
	*Logger
	Inbound   bool
	Outbound  bool
	Socks     bool
	KeepAlive time.Duration
}

//Tunnel represents an SSH tunnel with proxy capabilities.
//Both chisel client and server are Tunnels.
//chisel client has a single set of remotes, whereas
//chisel server has multiple sets of remotes.
//Each remote has a 1:1 mapping to a proxy.
//Proxies listen, send data over ssh, and the other end
//communicates with the endpoint and returns the response.
type Tunnel struct {
	TunnelConfig
	//ssh connection
	activeConnMut sync.RWMutex
	activeConn    ssh.Conn
	wgConn        sync.WaitGroup
	//proxies
	proxyCount int
	//internals
	connStats   ConnStats
	socksServer *socks5.Server
}

//NewTunnel from the given TunnelConfig
func NewTunnel(c TunnelConfig) *Tunnel {
	t := &Tunnel{TunnelConfig: c}
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
	//setup ssh keepalive loop
	if c.KeepAlive > 0 {
		panic("TODO")
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
func (t *Tunnel) BindRemotes(ctx context.Context, remotes []*Remote) error {
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

// //optional keepalive loop
// if c.config.KeepAlive > 0 {
// 	go c.keepAliveLoop(ctx)
// }

// func (c *Client) keepAliveLoop(ctx context.Context) {
// 	for c.running {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		default:
// 			//still open
// 		}
// 		time.Sleep(c.config.KeepAlive)
// 		if c.sshConn != nil {
// 			c.sshConn.SendRequest("ping", true, nil)
// 		}
// 	}
// }
