package tunnel

import (
	"fmt"
	"github.com/meteorite/socks5"
	"io"
	"net"
	"strings"

	"github.com/jpillora/chisel/share/cio"
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

func (t *Tunnel) handleSSHChannels(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		go t.handleSSHChannel(ch)
	}
}

func (t *Tunnel) handleSSHChannel(ch ssh.NewChannel) {
	if !t.Config.Outbound {
		t.Debugf("Denied outbound connection")
		ch.Reject(ssh.Prohibited, "Denied outbound connection")
		return
	}
	remote := string(ch.ExtraData())
	//extract protocol
	t.Debugf("remote: %s", remote)
	hostPort, proto := settings.L4Proto(remote)
	udp := proto == "udp"
	socksTCP := proto == "sot"
	socksUDP := proto == "sou"  // remote host:port is transmitted with each UDP packet both inbound and outbound
	//if socksUDP {
	//	hostPort = "socks" // destination is transmitted with each packet
	//}
	t.Debugf("remote: %s/%s", hostPort, proto)
	if (socksTCP || socksUDP)  &&  !t.socksAllowed {
		t.Debugf("Denied socks request, please enable socks")
		ch.Reject(ssh.Prohibited, "SOCKS5 is not enabled")
		return
	}
	sshChan, reqs, err := ch.Accept()
	if err != nil {
		t.Debugf("Failed to accept stream: %s", err)
		return
	}
	stream := io.ReadWriteCloser(sshChan)
	//cnet.MeterRWC(t.Logger.Fork("sshchan"), sshChan)
	defer stream.Close()
	go ssh.DiscardRequests(reqs)
	l := t.Logger.Fork("conn#%d", t.connStats.New())
	//ready to handle
	t.connStats.Open()
	l.Debugf("Open %s", t.connStats.String())
	if socksTCP {
		err = t.handleSocksTCP(l, stream, hostPort)
	} else if socksUDP {
		err = t.handleSocksUDP(l, stream)
	} else if udp {
		err = t.handleUDP(l, stream, hostPort)
	} else {
		err = t.handleTCP(l, stream, hostPort)
	}
	t.connStats.Close()
	errmsg := ""
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		errmsg = fmt.Sprintf(" (error %s)", err)
	}
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
}

func (t *Tunnel) handleSocksTCP(l *cio.Logger, src io.ReadWriteCloser, hostPort string) error {
	dst, err := net.Dial("tcp", hostPort)
	code := []byte{socks5.DialErrorToSocksCode(err)}
	_, errReply := src.Write(code)
	if errReply != nil {
		return errReply
	}
	if err != nil {
		return err
	}
	s, r := cio.Pipe(src, dst)
	l.Debugf("sent %s received %s", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}

func (t *Tunnel) handleTCP(l *cio.Logger, src io.ReadWriteCloser, hostPort string) error {
	dst, err := net.Dial("tcp", hostPort)
	if err != nil {
		return err
	}
	s, r := cio.Pipe(src, dst)
	l.Debugf("sent %s received %s", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}
