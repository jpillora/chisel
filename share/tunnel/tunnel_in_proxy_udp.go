package tunnel

import (
	"context"
	"encoding/gob"
	"net"
	"sync"
	"sync/atomic"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/settings"
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
func listenUDP(l *cio.Logger, ssh GetSSHConn, remote *settings.Remote) (*udpListener, error) {
	l = l.Fork("udp")
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
		ssh:     ssh,
		remote:  remote,
		inbound: conn,
	}
	return u, nil
}

type udpListener struct {
	*cio.Logger
	ssh         GetSSHConn
	remote      *settings.Remote
	inbound     *net.UDPConn
	outboundMut sync.Mutex
	outbound    *udpOutbound
	sent, recv  int64
}

func (u *udpListener) run(ctx context.Context) error {
	//udp doesnt accept connections,
	//udp simply forwards all packets
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
	u.Debugf("sent %d, received %d", u.sent, u.recv)
	return nil
}

func (u *udpListener) runInbound(ctx context.Context) error {
	dstAddr := u.remote.Remote()
	const maxMTU = 9012
	buff := make([]byte, maxMTU)
	for !isDone(ctx) {
		//read from inbound udp
		n, addr, err := u.inbound.ReadFromUDP(buff)
		if err != nil {
			return u.Errorf("read error: %w", err)
		}
		//upsert ssh channel
		o, err := u.getOubound()
		if err != nil {
			return u.Errorf("ssh-chan error: %w", err)
		}
		//send over channel, including source address
		b := buff[:n]
		if err := o.encode(addr.String(), dstAddr, b); err != nil {
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
		o, err := u.getOubound()
		if err != nil {
			return u.Errorf("ssh-chan error: %w", err)
		}
		//receive from channel, including source address
		p := udpPacket{}
		if err := o.decode(&p); err != nil {
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

func (u *udpListener) getOubound() (*udpOutbound, error) {
	u.outboundMut.Lock()
	defer u.outboundMut.Unlock()
	//cached
	if u.outbound != nil {
		return u.outbound, nil
	}
	//not cached, bind
	sshConn := u.ssh()
	if sshConn == nil {
		return nil, u.Errorf("ssh-conn nil")
	}
	//ssh request for udp packets for this proxy's remote,
	//just "udp" since the remote address is sent with each packet
	rwc, reqs, err := sshConn.OpenChannel("chisel", []byte("udp"))
	if err != nil {
		return nil, u.Errorf("ssh-chan error: %s", err)
	}
	go ssh.DiscardRequests(reqs)
	//ready
	o := &udpOutbound{
		r: gob.NewDecoder(rwc),
		w: gob.NewEncoder(rwc),
		c: rwc,
	}
	u.outbound = o
	return o, nil
}
