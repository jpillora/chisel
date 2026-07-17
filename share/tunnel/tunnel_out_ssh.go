package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/settings"
	"github.com/jpillora/sizestr"
	"golang.org/x/crypto/ssh"
)

func (t *Tunnel) handleSSHRequests(reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "ping":
			r.Reply(true, []byte("pong"))
		default:
			t.Debugf("Unknown request: %s", r.Type)
		}
	}
}

func (t *Tunnel) handleSSHChannels(ctx context.Context, chans <-chan ssh.NewChannel) {
	for ch := range chans {
		go t.handleSSHChannel(ctx, ch)
	}
}

func (t *Tunnel) handleSSHChannel(ctx context.Context, ch ssh.NewChannel) {
	if !t.Config.Outbound {
		t.Debugf("Denied outbound connection")
		ch.Reject(ssh.Prohibited, "Denied outbound connection")
		return
	}
	remote := string(ch.ExtraData())
	//extract protocol
	hostPort, proto := settings.L4Proto(remote)
	udp := proto == "udp"
	socks := hostPort == "socks"
	if socks && t.socksServer == nil {
		t.Debugf("Denied socks request, please enable socks")
		ch.Reject(ssh.Prohibited, "SOCKS5 is not enabled")
		return
	}
	//check ACL against the actual requested destination
	//(socks channels are checked against the well-known token "socks")
	if t.Config.ACL != nil && !t.Config.ACL(hostPort) {
		//info-level: post-1.12 the most likely cause is an authfile
		//missing a "socks" grant, and operators need to see that
		//without -v
		t.Infof("Denied connection to %s (ACL)", hostPort)
		ch.Reject(ssh.Prohibited, "access denied")
		return
	}
	//tcp: dial the target before accepting the channel, so dial
	//failures propagate to the inbound side instead of presenting
	//a successful connection to a dead target
	var dst net.Conn
	if !socks && !udp {
		d := net.Dialer{Timeout: settings.EnvDuration("DIAL_TIMEOUT", 30*time.Second)}
		c, err := d.DialContext(ctx, "tcp", hostPort)
		if err != nil {
			t.Debugf("Failed to dial %s: %s", hostPort, err)
			ch.Reject(ssh.ConnectionFailed, err.Error())
			return
		}
		dst = c
	}
	sshChan, reqs, err := ch.Accept()
	if err != nil {
		t.Debugf("Failed to accept stream: %s", err)
		if dst != nil {
			dst.Close()
		}
		return
	}
	stream := io.ReadWriteCloser(sshChan)
	defer stream.Close()
	go ssh.DiscardRequests(reqs)
	l := t.Logger.Fork("conn#%d", t.connStats.New())
	//ready to handle
	t.connStats.Open()
	l.Debugf("Open %s", t.connStats.String())
	if socks {
		err = t.handleSocks(stream)
	} else if udp {
		err = t.handleUDP(l, stream, hostPort)
	} else {
		err = t.handleTCP(l, stream, dst)
	}
	t.connStats.Close()
	errmsg := ""
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		errmsg = fmt.Sprintf(" (error %s)", err)
	}
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
}

func (t *Tunnel) handleSocks(src io.ReadWriteCloser) error {
	return t.socksServer.ServeConn(cnet.NewRWCConn(src))
}

func (t *Tunnel) handleTCP(l *cio.Logger, src io.ReadWriteCloser, dst net.Conn) error {
	s, r := cio.Pipe(src, dst)
	l.Debugf("sent %s received %s", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}
