package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	chshare "github.com/valkyrie-io/connector-tunnel/common"
	"github.com/valkyrie-io/connector-tunnel/common/netext"
	"github.com/valkyrie-io/connector-tunnel/common/settings"
	"golang.org/x/crypto/ssh"
)

func (c *Client) connLoop(ctx context.Context) error {
	//connection loop!
	b := &Backoff{Max: c.config.MaxRetryInterval}
	for {
		connected, err := c.singularConnect(ctx)
		//reset backoff after successful connections
		if connected {
			b.Reset()
		}
		//connection error
		attempt := int(b.Attempt())
		maxAttempt := c.config.MaxRetryCount
		//dont print closed-connection errors
		if strings.HasSuffix(err.Error(), "use of closed network connection") {
			err = io.EOF
		}
		//show error message and attempt counts (excluding disconnects)
		if err != nil && err != io.EOF {
			msg := fmt.Sprintf("Connection error: %s", err)
			if attempt > 0 {
				maxAttemptVal := fmt.Sprint(maxAttempt)
				if maxAttempt < 0 {
					maxAttemptVal = "unlimited"
				}
				msg += fmt.Sprintf(" (Attempt: %d/%s)", attempt, maxAttemptVal)
			}
			c.Infof(msg)
		}
		//give up?
		if maxAttempt >= 0 && attempt >= maxAttempt {
			c.Infof("Give up")
			break
		}
		d := b.Duration()
		c.Infof("Retrying in %s...", d)
		select {
		case <-ctx.Done():
			c.Infof("Cancelled")
			return nil
		}
	}
	c.Close()
	return nil
}

// singularConnect connects to the chisel server and blocks
func (c *Client) singularConnect(ctx context.Context) (connected bool, err error) {
	//already closed?
	select {
	case <-ctx.Done():
		return false, errors.New("Cancelled")
	default:
		//still open
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	//prepare dialer
	d := websocket.Dialer{
		HandshakeTimeout: settings.EnvDuration("WS_TIMEOUT", 45*time.Second),
		Subprotocols:     []string{chshare.ProtocolVersion},
		TLSClientConfig:  c.tlsConfig,
		ReadBufferSize:   settings.EnvInt("WS_BUFF_SIZE", 0),
		WriteBufferSize:  settings.EnvInt("WS_BUFF_SIZE", 0),
		NetDialContext:   c.config.DialContext,
	}
	wsConn, _, err := d.DialContext(ctx, c.server, c.config.Headers)
	if err != nil {
		return false, err
	}
	conn := netext.NewWebSocketConn(wsConn)
	// perform SSH handshake on netext.Conn
	sshConn, chans, reqs, b, err := c.handshake(err, conn)
	if err != nil {
		return b, err
	}
	defer sshConn.Close()
	timer, err2 := c.sendConfiguration(sshConn)
	if err2 != nil {
		return false, err2
	}
	err, connected = c.handleConnected(ctx, timer, err, sshConn, reqs, chans, connected)
	return connected, err
}

func (c *Client) handleConnected(ctx context.Context, timer time.Time, err error, sshConn ssh.Conn, reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel, connected bool) (error, bool) {
	c.Infof("Connected (Latency %s)", time.Since(timer))
	//connected, handover ssh connection for sshconnection to use, and block
	err = c.tunnel.Bind(ctx, sshConn, reqs, chans)
	c.Infof("Disconnected")
	connected = time.Since(timer) > 5*time.Second
	return err, connected
}

func (c *Client) sendConfiguration(sshConn ssh.Conn) (time.Time, error) {
	c.Debugf("Sending config")
	timer := time.Now()
	_, configerr, err := sshConn.SendRequest(
		"config",
		true,
		settings.EncodeConfig(c.computed),
	)
	if err != nil {
		c.Infof("Config verification failed")
		return time.Time{}, err
	}
	if len(configerr) > 0 {
		return time.Time{}, errors.New(string(configerr))
	}
	return timer, nil
}

func (c *Client) handshake(err error, conn net.Conn) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, bool, error) {
	c.Debugf("Handshaking...")
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", c.sshConfig)
	if err != nil {
		e := err.Error()
		if strings.Contains(e, "unable to authenticate") {
			c.Infof("Authentication failed")
			c.Debugf(e)
		} else {
			c.Infof(e)
		}
		return nil, nil, nil, false, err
	}
	return sshConn, chans, reqs, false, nil
}
