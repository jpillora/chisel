package tunnel

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/jpillora/sizestr"
	"github.com/valkyrie-io/connector-tunnel/common/logging"
	"github.com/valkyrie-io/connector-tunnel/common/settings"
	"golang.org/x/crypto/ssh"
)

func (t *SSHTunnel) handleSSHRequests(reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "valkyrie-syn":
			r.Reply(true, []byte("valkyrie-ack"))
		default:
			t.Debugf("Unknown request: %s", r.Type)
		}
	}
}

func (t *SSHTunnel) handleSSHChannels(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		go t.handleSSHChannel(ch)
	}
}

func (t *SSHTunnel) handleSSHChannel(ch ssh.NewChannel) {
	if !t.validateAllowOutbound(ch) {
		return
	}
	hostPort, reqs, stream, err := t.connectRemote(ch)
	if err != nil {
		return
	}
	defer stream.Close()
	go ssh.DiscardRequests(reqs)
	l, err := t.handleOpenConnection(err, stream, hostPort)
	t.closeConnection(err, l)
}

func (t *SSHTunnel) connectRemote(ch ssh.NewChannel) (string, <-chan *ssh.Request, io.ReadWriteCloser, error) {
	remote := string(ch.ExtraData())
	//extract protocol
	hostPort := settings.L4Proto(remote)
	sshChan, reqs, err := ch.Accept()
	if err != nil {
		t.Debugf("Failed to accept stream: %s", err)
		return "", nil, nil, err
	}
	stream := io.ReadWriteCloser(sshChan)
	return hostPort, reqs, stream, err
}

func (t *SSHTunnel) validateAllowOutbound(ch ssh.NewChannel) bool {
	if !t.Config.Outbound {
		t.Debugf("Denied outbound connection")
		ch.Reject(ssh.Prohibited, "Denied outbound connection")
		return false
	}
	return true
}

func (t *SSHTunnel) handleOpenConnection(err error, stream io.ReadWriteCloser, hostPort string) (*logging.Logger, error) {
	l := t.Logger.Fork("conn#%d", t.connStats.New())
	//ready to handle
	t.connStats.Open()
	l.Debugf("Open %s", t.connStats.String())
	err = t.handleTCP(l, stream, hostPort)
	return l, err
}

func (t *SSHTunnel) closeConnection(err error, l *logging.Logger) {
	t.connStats.Close()
	errmsg := ""
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		errmsg = fmt.Sprintf(" (error %s)", err)
	}
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
}

func (t *SSHTunnel) handleTCP(l *logging.Logger, src io.ReadWriteCloser, hostPort string) error {
	dst, err := net.Dial("tcp", hostPort)
	if err != nil {
		return err
	}
	s, r := Pipe(src, dst)
	l.Debugf("sent %s received %s", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}
