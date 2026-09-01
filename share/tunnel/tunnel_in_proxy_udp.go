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

// listenUDP is a special listener which forwards packets via
// the bound ssh connection. tricky part is multiplexing lots of
// udp clients through the entry node. each will listen on its
// own source-port for a response:
//
//	                                            (random)
//	src-1 1111->...                         dst-1 6345->7777
//	src-2 2222->... <---> udp <---> udp <-> dst-1 7543->7777
//	src-3 3333->...    listener    handler  dst-1 1444->7777
//
// we must store these mappings (1111-6345, etc) in memory for a length
// of time, so that when the exit node receives a response on 6345, it
// knows to return it to 1111.
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
		Logger:       l,
		sshTun:       sshTun,
		remote:       remote,
		inbound:      conn,
		maxMTU:       settings.EnvInt("UDP_MAX_SIZE", 9012),
		peers:        map[string]udpPeer{},
		peerDeadline: settings.EnvDuration("UDP_DEADLINE", 15*time.Second),
		maxPeers:     settings.EnvInt("UDP_MAX_CONNS", 100),
	}
	u.Debugf("UDP max size: %d bytes", u.maxMTU)
	return u, nil
}

type udpListener struct {
	*cio.Logger
	sshTun       sshTunnel
	remote       *settings.Remote
	inbound      *net.UDPConn
	outboundMut  sync.Mutex
	outbound     *udpChannel
	peerMut      sync.Mutex
	peers        map[string]udpPeer
	peerDeadline time.Duration
	maxPeers     int
	sent, recv   int64
	maxMTU       int
}

// udpPeer is a listener peer that actually sent a datagram to the bound UDP
// socket. Only these addresses may receive a return packet from the client.
type udpPeer struct {
	addr     *net.UDPAddr
	lastSeen time.Time
}

func (u *udpListener) run(ctx context.Context) error {
	defer u.inbound.Close()
	//udp doesn't accept connections,
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
		if e, ok := err.(net.Error); ok && e.Timeout() {
			continue
		}
		if err != nil {
			return u.Errorf("read error: %w", err)
		}
		// Only accept return packets for peers which have actually sent to
		// this listener. The peer address is otherwise client-controlled.
		u.rememberPeer(addr)
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
		//write back to an observed UDP peer only. p.Src comes from the SSH
		//client, so it must not be resolved or used as a new destination.
		addr, ok := u.returnPeer(p.Src)
		if !ok {
			u.Debugf("Dropping UDP return packet for unobserved peer %q", p.Src)
			continue
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

// rememberPeer records an address obtained directly from ReadFromUDP. Entries
// expire after UDP_DEADLINE and the map is capped at UDP_MAX_CONNS so an
// Internet-facing listener cannot retain unbounded peer state.
func (u *udpListener) rememberPeer(addr *net.UDPAddr) {
	now := time.Now()
	key := addr.String()
	u.peerMut.Lock()
	defer u.peerMut.Unlock()
	if u.peers == nil {
		u.peers = map[string]udpPeer{}
	}
	u.expirePeers(now)
	if _, found := u.peers[key]; !found && len(u.peers) >= u.maxPeers {
		u.evictOldestPeer()
	}
	// ReadFromUDP allocates the address today, but retain our own copy so the
	// peer table cannot depend on that implementation detail.
	peerAddr := *addr
	peerAddr.IP = append(net.IP(nil), addr.IP...)
	u.peers[key] = udpPeer{addr: &peerAddr, lastSeen: now}
}

// returnPeer obtains a non-expired peer recorded by rememberPeer. Matching the
// exact string sent by runInbound also rejects hostname and alternate-address
// encodings supplied by a client.
func (u *udpListener) returnPeer(src string) (*net.UDPAddr, bool) {
	now := time.Now()
	u.peerMut.Lock()
	defer u.peerMut.Unlock()
	peer, found := u.peers[src]
	if !found {
		return nil, false
	}
	if peer.lastSeen.Before(now.Add(-u.peerDeadline)) {
		delete(u.peers, src)
		return nil, false
	}
	return peer.addr, true
}

func (u *udpListener) expirePeers(now time.Time) {
	for key, peer := range u.peers {
		if peer.lastSeen.Before(now.Add(-u.peerDeadline)) {
			delete(u.peers, key)
		}
	}
}

func (u *udpListener) evictOldestPeer() {
	var oldestKey string
	var oldest time.Time
	for key, peer := range u.peers {
		if oldestKey == "" || peer.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = peer.lastSeen
		}
	}
	if oldestKey != "" {
		delete(u.peers, oldestKey)
	}
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
	u.Debugf("acquired channel")
	return o, nil
}

func (u *udpListener) unsetUDPChan(sshConn ssh.Conn) {
	sshConn.Wait()
	u.Debugf("lost channel")
	u.outboundMut.Lock()
	u.outbound = nil
	u.outboundMut.Unlock()
}
