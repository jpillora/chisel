package socks5

import (
	"github.com/jpillora/chisel/share/socks5/scope"
	"net"
	"sync"
)

type ipAssoc struct {
	*oneClientUDPRemotes

	tcpCount int32 // guarded by SingleUDPPortAssociate::mtx
}

type SingleUDPPortAssociate struct {
	udpAddr     *AddrSpec
	connFactory RemoteUDPConnFactory
	log         ErrorLogger

	mtx      sync.Mutex
	ipAssocs map[string]*ipAssoc

	ctxServer ContextGo
}

// Creates new SingleUDPPortAssociate.
// Either ListenAndServeUDPPort or ServeUDPPort MUST be called after creation to start serving UDP port. It is
// convenient to call ListenAndServeUDPPort from Handler.OnStartServe.
//
// udpAddr MUST contain IP to listen UDP port on. It also MAY contain FQDN, if it should be sent to clients in
// associate responses. udpAddr MUST have valid Port, if is started with ServeUDPPort, otherwise it MAY have Port == 0
// (then Port will be chosen automatically in ListenAndServeUDPPort)
func MakeSingleUDPPortAssociate(udpAddr *AddrSpec, connFactory RemoteUDPConnFactory, log ErrorLogger) *SingleUDPPortAssociate {
	return &SingleUDPPortAssociate{
		udpAddr:     udpAddr,
		connFactory: connFactory,
		log:         log,
		ipAssocs:    make(map[string]*ipAssoc),
	}
}

// ListenAndServeUDPPort is used to create incoming UDP port and serve on it
func (s *SingleUDPPortAssociate) ListenAndServeUDPPort(ctxServer ContextGo, udpNet string) error {
	addr := net.UDPAddr{
		Port: s.udpAddr.Port,
		IP:   s.udpAddr.IP,
	}
	c, err := net.ListenUDP(udpNet, &addr)
	if err != nil {
		return err
	}
	if s.udpAddr.Port == 0 {
		s.udpAddr.Port = c.LocalAddr().(*net.UDPAddr).Port
	}

	ctxServer.Go(func() error { return s.ServeUDPPort(ctxServer, c) })
	return nil
}

func (s *SingleUDPPortAssociate) ServeUDPPort(ctxServer ContextGo, udpConn *net.UDPConn) error {
	defer scope.Closer(ctxServer.Ctx(), udpConn).Close()

	s.ctxServer = ctxServer

	sendBack := MakeSendBackTo(udpConn)

	buffer := make([]byte, s.connFactory.MaxUDPPacketSize())
	for {
		n, src, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		// s.log.Printf("client %s:%d udp packet: %d bytes", src.IP.String(), src.Port, n)

		fromIP := src.IP.String()
		assoc := s.getAssoc(fromIP)
		if assoc == nil {
			s.log.Printf("udp packet from unauthorized IP: %s", fromIP)
			continue
		}

		if err := assoc.forwardClientPktToRemote(ctxServer, src, buffer[:n], s.connFactory, sendBack); err != nil {
			s.log.Printf("%v", err)
		}
	}
}

func (s *SingleUDPPortAssociate) addAssoc(fromIP string) ContextGo {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	assoc, found := s.ipAssocs[fromIP]
	if found {
		assoc.tcpCount++
		return assoc.ctxClient
	} else {
		g, _ := scope.Group(s.ctxServer.Ctx())
		s.ipAssocs[fromIP] = &ipAssoc{oneClientUDPRemotes: makeOneClientUDPRemotes(g), tcpCount: 1}
		return g
	}
}

func (s *SingleUDPPortAssociate) delAssoc(fromIP string) {
	s.mtx.Lock()
	locked := true
	defer func() {
		if locked {
			s.mtx.Unlock()
		}
	}()

	assoc, found := s.ipAssocs[fromIP]
	if !found {
		return
	}
	assoc.tcpCount--
	if assoc.tcpCount <= 0 {
		delete(s.ipAssocs, fromIP)

		s.mtx.Unlock()
		locked = false

		assoc.stopAll()
		assoc.ctxClient.Cancel()
		assoc.ctxClient.WaitQuietly()
	}
}

func (s *SingleUDPPortAssociate) getAssoc(from string) *ipAssoc {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.ipAssocs[from]
}

func (s *SingleUDPPortAssociate) OnAssociate(conn net.Conn) error {
	from, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return onAssociateSendError(conn, err)
	}

	ctxClient := s.addAssoc(from)
	defer s.delAssoc(from)
	defer scope.Closer(ctxClient.Ctx(), conn).Close()

	return onAssociateReplyUdpAddrAndWaitForClose(conn, s.udpAddr, s.log)
}

//func (s *SingleUDPPortAssociate) onUDPPkt(ctx context.Context, client *net.UDPAddr, pkt []byte, sendBack UDPSendBackTo) {
//	destAddr, data, err := ParseUdpPacket(pkt)
//	if err != nil {
//		return
//	}
//
//	assoc := s.getAssoc(client.IP.String())
//	if assoc == nil {
//		return
//	}
//
//	w, err := assoc.getOrAddRemote(ctx, client, s.connFactory, sendBack)
//	if err != nil {
//		return
//	}
//	w.refreshLastTS()
//	if err = w.conn.Send(ctx, data, destAddr); err != nil {
//		assoc.delRemote(uint16(client.Port), w)
//		_ = w.conn.Close()
//	}
//}
