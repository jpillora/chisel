package link

import (
	"fmt"
	"github.com/valkyrie-io/connector-tunnel/shared"
	"io"
	"net"
	"strings"

	"github.com/jpillora/sizestr"
	"github.com/valkyrie-io/connector-tunnel/shared/settings"
	"golang.org/x/crypto/ssh"
)

func (t *Link) processSSHRequests(reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "hello-request":
			r.Reply(true, []byte("hello-reply"))
		default:
			t.Debugf("Unknown request: %s", r.Type)
		}
	}
}

func (t *Link) processSSHChannels(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		go t.processSSHChannel(ch)
	}
}

func (t *Link) processSSHChannel(ch ssh.NewChannel) {
	if !t.Options.AllowOutbound {
		t.Debugf("Rejected outbound connection")
		ch.Reject(ssh.Prohibited, "Rejected outbound connection")
		return
	}
	remote := string(ch.ExtraData())
	//extract protocol
	hostPort, _ := settings.L4Proto(remote)
	sshChan, reqs, err := ch.Accept()
	if err != nil {
		t.Debugf("Stream not accepted: %s", err)
		return
	}
	stream := io.ReadWriteCloser(sshChan)
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
}

func (t *Link) handleTCP(l *shared.Logger, src io.ReadWriteCloser, hostPort string) error {
	dst, err := net.Dial("tcp", hostPort)
	if err != nil {
		return err
	}
	s, r := Pipe(src, dst)
	l.Debugf("sent %s received %s", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}
