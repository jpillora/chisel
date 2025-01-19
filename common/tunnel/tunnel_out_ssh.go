package tunnel

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/jpillora/sizestr"
	"github.com/valkyrie-io/connector-tunnel/common/cio"
	"github.com/valkyrie-io/connector-tunnel/common/settings"
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
	hostPort := settings.L4Proto(remote)
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
	err = t.handleTCP(l, stream, hostPort)
	t.connStats.Close()
	errmsg := ""
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		errmsg = fmt.Sprintf(" (error %s)", err)
	}
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
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
