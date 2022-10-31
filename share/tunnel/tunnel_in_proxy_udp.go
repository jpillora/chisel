package tunnel

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
	"github.com/jpillora/sizestr"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

//listenUDP is a special listener which forwards packets via
//the bound ssh connection. tricky part is multiplexing lots of
//udp clients through the entry node. each will listen on its
//own source-port for a response:
//                                                (random)
//    src-1 1111->...                         dst-1 6345->7777
//    src-2 2222->... <---> udp <---> udp <-> dst-1 7543->7777
//    src-3 3333->...    listener    handler  dst-1 1444->7777
//
//we must store these mappings (1111-6345, etc) in memory for a length
//of time, so that when the exit node receives a response on 6345, it
//knows to return it to 1111.
func listenUDP(l *cio.Logger, sshTun sshTunnel, remote *settings.Remote) (*udpListener, error) {
	a, err := net.ResolveUDPAddr("udp", remote.Local())
	if err != nil {
		return nil, l.Errorf("resolve: %s", err)
	}
	conn, err := net.ListenUDP("udp", a)
	if err != nil {
		return nil, l.Errorf("listen: %s", err)
	}
	//ready
	u := &udpListener{
		Logger:  l,
		sshTun:  sshTun,
		remote:  remote,
		inbound: conn,
		maxMTU:  settings.EnvInt("UDP_MAX_SIZE", 9012),
	}
	u.Debugf("UDP max size: %d bytes", u.maxMTU)
	return u, nil
}

type udpListener struct {
	*cio.Logger
	sshTun      sshTunnel
	remote      *settings.Remote
	inbound     *net.UDPConn
	outboundMut sync.Mutex
	outbound    *udpChannel
	sent, recv  int64
	maxMTU      int
}

func (u *udpListener) run(ctx context.Context) error {
	defer u.inbound.Close()
	//udp doesnt accept connections,
	//udp simply forwards packets
	//and therefore only needs to listen
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return u.runInbound(ctx)
	})
	eg.Go(func() error {
		return u.runOutbound(ctx)
	})
	if err := eg.Wait(); err != nil {
		u.Debugf("listen: %s", err)
		return err
	}
	u.Debugf("Close (sent %s received %s)", sizestr.ToString(u.sent), sizestr.ToString(u.recv))
	return nil
}

func (u *udpListener) runInbound(ctx context.Context) error {
	buff := make([]byte, u.maxMTU)
	for !isDone(ctx) {
		//read from inbound udp
		u.inbound.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := u.inbound.ReadFromUDP(buff)
		if e, ok := err.(net.Error); ok && (e.Timeout() || e.Temporary()) {
			continue
		}
		if err != nil {
			return u.Errorf("read error: %w", err)
		}
		//upsert ssh channel
		uc, err := u.getUDPChan(ctx)
		if err != nil {
			if strings.HasSuffix(err.Error(), "EOF") {
				continue
			}
			return u.Errorf("inbound-udpchan: %w", err)
		}
		//send over channel, including source address
		b := buff[:n]
		if err := uc.encode(addr.String(), b); err != nil {
			if strings.HasSuffix(err.Error(), "EOF") {
				continue //dropped packet...
			}
			return u.Errorf("encode error: %w", err)
		}
		//stats
		atomic.AddInt64(&u.sent, int64(n))
	}
	return nil
}

func (u *udpListener) runOutbound(ctx context.Context) error {
	for !isDone(ctx) {
		//upsert ssh channel
		uc, err := u.getUDPChan(ctx)
		if err != nil {
			if strings.HasSuffix(err.Error(), "EOF") {
				continue
			}
			return u.Errorf("outbound-udpchan: %w", err)
		}
		//receive from channel, including source address
		p := udpPacket{}
		if err := uc.decode(&p); err == io.EOF {
			//outbound ssh disconnected, get new connection...
			continue
		} else if err != nil {
			return u.Errorf("decode error: %w", err)
		}
		//write back to inbound udp
		addr, err := net.ResolveUDPAddr("udp", p.Src)
		if err != nil {
			return u.Errorf("resolve error: %w", err)
		}
		n, err := u.inbound.WriteToUDP(p.Payload, addr)
		if err != nil {
			return u.Errorf("write error: %w", err)
		}
		//stats
		atomic.AddInt64(&u.recv, int64(n))
	}
	return nil
}

func (u *udpListener) getUDPChan(ctx context.Context) (*udpChannel, error) {
	u.outboundMut.Lock()
	defer u.outboundMut.Unlock()
	//cached
	if u.outbound != nil {
		return u.outbound, nil
	}
	//not cached, bind
	sshConn := u.sshTun.getSSH(ctx)
	if sshConn == nil {
		return nil, fmt.Errorf("ssh-conn nil")
	}
	//ssh request for udp packets for this proxy's remote,
	//just "udp" since the remote address is sent with each packet
	dstAddr := u.remote.Remote() + "/udp"
	rwc, reqs, err := sshConn.OpenChannel("chisel", []byte(dstAddr))
	if err != nil {
		return nil, fmt.Errorf("ssh-chan error: %s", err)
	}
	go ssh.DiscardRequests(reqs)
	//remove on disconnect
	go u.unsetUDPChan(sshConn)
	//ready
	o := &udpChannel{
		r: gob.NewDecoder(rwc),
		w: gob.NewEncoder(rwc),
		c: rwc,
	}
	u.outbound = o
	u.Debugf("aquired channel")
	return o, nil
}

func (u *udpListener) unsetUDPChan(sshConn ssh.Conn) {
	sshConn.Wait()
	u.Debugf("lost channel")
	u.outboundMut.Lock()
	u.outbound = nil
	u.outboundMut.Unlock()
}
