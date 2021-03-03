package tunnel

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
	"github.com/jpillora/chisel/share/socks5"
	"github.com/jpillora/sizestr"
	"golang.org/x/crypto/ssh"
)

//sshTunnel exposes a subset of Tunnel to subtypes
type sshTunnel interface {
	getSSH(ctx context.Context) ssh.Conn
}

//Proxy is the inbound portion of a Tunnel
type Proxy struct {
	*cio.Logger
	sshTun      sshTunnel
	id          int
	count       int
	remote      *settings.Remote
	dialer      net.Dialer
	tcp         *net.TCPListener
	udp         *udpListener
	socksServer *socks5.Server
}

//NewProxy creates a Proxy
func NewProxy(logger *cio.Logger, sshTun sshTunnel, index int, remote *settings.Remote) (*Proxy, error) {
	id := index + 1
	p := &Proxy{
		Logger: logger.Fork("proxy#%s", remote.String()),
		sshTun: sshTun,
		id:     id,
		remote: remote,
	}
	return p, p.listen()
}

func (p *Proxy) listen() error {
	if p.remote.Stdio {
		//TODO check if pipes active?
	} else if p.remote.Socks || p.remote.LocalProto == "tcp" {
		addr, err := net.ResolveTCPAddr("tcp", p.remote.LocalHost+":"+p.remote.LocalPort)
		if err != nil {
			return p.Errorf("resolve: %s", err)
		}
		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return p.Errorf("tcp: %s", err)
		}
		if p.remote.Socks {
			udpAddr, err := net.ResolveUDPAddr("udp", p.remote.LocalHost+":0")
			if err != nil {
				_ = l.Close()
				return p.Errorf("resolve: %s", err)
			}

			sl := log.New(ioutil.Discard, "", 0)
			if p.Logger.Debug {
				sl = log.New(os.Stdout, "[socks]", log.Ldate|log.Ltime)
			}
			p.socksServer, _ = socks5.New(&socks5.Config{
				Handler: makeChiselSocksHandler(p, udpAddr, sl),
			})
		}
		p.Infof("Listening")
		p.tcp = l
	} else if p.remote.LocalProto == "udp" {
		l, err := listenUDP(p.Logger, p.sshTun, p.remote)
		if err != nil {
			return err
		}
		p.Infof("Listening")
		p.udp = l
	} else {
		return p.Errorf("unknown local proto")
	}
	return nil
}

//Run enables the proxy and blocks while its active,
//close the proxy by cancelling the context.
func (p *Proxy) Run(ctx context.Context) error {
	if p.remote.Stdio {
		return p.runStdio(ctx)
	} else if p.remote.Socks {
		return p.socksServer.Serve(ctx, p.tcp)
	} else if p.remote.LocalProto == "tcp" {
		return p.runTCP(ctx)
	} else if p.remote.LocalProto == "udp" {
		return p.udp.run(ctx)
	}
	panic("should not get here")
}

func (p *Proxy) runStdio(ctx context.Context) error {
	defer p.Infof("Closed")
	for {
		p.pipeRemote(ctx, cio.Stdio, p.remote.Remote(), nil)
		select {
		case <-ctx.Done():
			return nil
		default:
			// the connection is not ready yet, keep waiting
		}
	}
}

func (p *Proxy) runTCP(ctx context.Context) error {
	done := make(chan struct{})
	//implements missing net.ListenContext
	go func() {
		select {
		case <-ctx.Done():
			p.tcp.Close()
		case <-done:
		}
	}()
	for {
		src, err := p.tcp.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				//listener closed
				err = nil
			default:
				p.Infof("Accept error: %s", err)
			}
			close(done)
			return err
		}
		go p.pipeRemote(ctx, src, p.remote.Remote(), nil)
	}
}

func (p *Proxy) pipeRemote(ctx context.Context, src io.ReadWriteCloser, remoteHostPort string, handshake func(dst ssh.Channel) error) error {
	defer src.Close()
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	sshConn := p.sshTun.getSSH(ctx)
	if sshConn == nil {
		msg := "no remote connection"
		l.Debugf(msg)
		return errors.New(msg)
	}
	//ssh request for tcp connection for this proxy's remote
	l.Debugf("Requesting: %s", remoteHostPort)
	dst, reqs, err := sshConn.OpenChannel("chisel", []byte(remoteHostPort))
	if err != nil {
		l.Infof("Stream error: %s", err)
		return err
	}
	go ssh.DiscardRequests(reqs)
	if handshake != nil {
		if err = handshake(dst); err != nil {
			return err
		}
	}
	//then pipe
	s, r := cio.Pipe(src, dst)
	l.Debugf("Close (sent %s received %s)", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}
