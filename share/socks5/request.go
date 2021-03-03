package socks5

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

/******************************************************
    Requests of client:

    +----+-----+-------+------+----------+----------+
    |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
    +----+-----+-------+------+----------+----------+
    | 1  |  1  | X'00' |  1   | Variable |    2     |
    +----+-----+-------+------+----------+----------+
*******************************************************/

// CMD declaration
const (
	// CommandConnect CMD CONNECT X'01'
	CommandConnect = uint8(1)
	// CommandBind CMD BIND X'02'. The BIND request is used in protocols
	// which require the client to accept connections from the server.
	CommandBind = uint8(2)
	// CommandAssociate CMD UDP ASSOCIATE X'03'.  The UDP ASSOCIATE request
	// is used to establish an association within the UDP relay process to
	// handle UDP datagrams.
	CommandAssociate = uint8(3)
)

// ATYP address type of following address declaration
const (
	// AddressIPv4 IP V4 address: X'01'
	AddressIPv4 = uint8(1)
	// AddressDomainName DOMAINNAME: X'03'
	AddressDomainName = uint8(3)
	// AddressIPv6 IP V6 address: X'04'
	AddressIPv6 = uint8(4)
)

/******************************************************
    Response of server:

    +----+-----+-------+------+----------+----------+
    |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
    +----+-----+-------+------+----------+----------+
    | 1  |  1  | X'00' |  1   | Variable |    2     |
    +----+-----+-------+------+----------+----------+
*******************************************************/

// REP field declaration
//goland:noinspection GoUnusedConst
const (
	// ReplySucceeded X'00' succeeded
	ReplySucceeded uint8 = iota
	// ReplyServerFailure X'01' general SOCKS server failure
	ReplyServerFailure
	// ReplyRuleFailure X'02' connection not allowed by ruleset
	ReplyRuleFailure
	// ReplyNetworkUnreachable X'03' Network unreachable
	ReplyNetworkUnreachable
	// ReplyHostUnreachable X'04' Host unreachable
	ReplyHostUnreachable
	// ReplyConnectionRefused X'05' Connection refused
	ReplyConnectionRefused
	// ReplyTTLExpired X'06' TTL expired
	ReplyTTLExpired
	// ReplyCommandNotSupported X'07' Command not supported
	ReplyCommandNotSupported
	// ReplyAddrTypeNotSupported X'08' Address type not supported
	ReplyAddrTypeNotSupported
)

var errUnrecognizedAddrType = fmt.Errorf("unrecognized address type")

// AddressRewriter is used to rewrite a destination transparently
type AddressRewriter interface {
	Rewrite(ctx context.Context, request *Request) (context.Context, *AddrSpec)
}

// AddrSpec is used to return the target AddrSpec
// which may be specified as IPv4, IPv6, or a FQDN
type AddrSpec struct {
	FQDN string
	IP   net.IP
	Port int
}

func (a *AddrSpec) String() string {
	if a.FQDN != "" {
		return fmt.Sprintf("%s (%s):%d", a.FQDN, a.IP, a.Port)
	}
	return fmt.Sprintf("%s:%d", a.IP, a.Port)
}

// Address returns a string suitable to dial; prefer returning FQDN, fallback to IP-based address
func (a *AddrSpec) Address() string {
	return net.JoinHostPort(a.Host(), strconv.Itoa(a.Port))
}

func (a* AddrSpec) Host() string {
	if a.FQDN != "" {
		return a.FQDN
	} else {
		return a.IP.String()
	}
}

func isIpV4(ip net.IP) bool {
	return ip.To4() != nil
}

func (a *AddrSpec) SerializedSize() int {
	var sz int
	switch {
	case a == nil:
		sz = 4
	case a.FQDN != "":
		sz = 1 + len(a.FQDN)
	case isIpV4(a.IP):
		sz = 4
	default:
		sz = 16
	}
	return 3 + sz
}

func (a *AddrSpec) SerializeTo(buf []byte) (int, error) {
	var addrType uint8
	var addrBody []byte
	var addrPort uint16
	switch {
	case a == nil:
		addrType = AddressIPv4
		addrBody = []byte{0, 0, 0, 0}
		addrPort = 0

	case a.FQDN != "":
		addrType = AddressDomainName
		addrBody = append([]byte{byte(len(a.FQDN))}, a.FQDN...)
		addrPort = uint16(a.Port)

	case isIpV4(a.IP):
		addrType = AddressIPv4
		addrBody = a.IP.To4()
		addrPort = uint16(a.Port)

	case len(a.IP) == net.IPv6len:
		addrType = AddressIPv6
		addrBody = a.IP
		addrPort = uint16(a.Port)

	default:
		return 0, fmt.Errorf("failed to format address: %v", a)
	}

	// Format the message
	buf[0] = addrType
	copy(buf[1:], addrBody)
	buf[1+len(addrBody)] = byte(addrPort >> 8)
	buf[1+len(addrBody)+1] = byte(addrPort & 0xff)

	return 3 + len(addrBody), nil
}

// A Request represents request received by a server
type Request struct {
	// Protocol version
	Version uint8
	// Requested command
	Command uint8
	// AuthContext provided during negotiation
	AuthContext *AuthContext
	// AddrSpec of the the network that sent the request
	RemoteAddr *AddrSpec
	// AddrSpec of the desired destination
	DestAddr *AddrSpec
	// AddrSpec of the actual destination (might be affected by rewrite)
	realDestAddr *AddrSpec
	bufConn      io.Reader
}

// NewRequest creates a new Request from the tcp connection
func NewRequest(bufConn io.Reader) (*Request, error) {
	// Read the version byte
	header := []byte{0, 0, 0}
	if _, err := io.ReadAtLeast(bufConn, header, 3); err != nil {
		return nil, fmt.Errorf("failed to get command version: %v", err)
	}

	// Ensure we are compatible
	if header[0] != socks5Version {
		return nil, fmt.Errorf("unsupported command version: %v", header[0])
	}

	// Read in the destination address
	dest, err := readAddrSpec(bufConn)
	if err != nil {
		return nil, err
	}

	request := &Request{
		Version:  socks5Version,
		Command:  header[1],
		DestAddr: dest,
		bufConn:  bufConn,
	}

	return request, nil
}

// used for request processing after authentication
func (s *Server) handleRequest(ctx context.Context, req *Request, conn net.Conn) error {
	// Resolve the address if we have a FQDN
	dest := req.DestAddr
	if dest.FQDN != "" {
		_ctx, addr, err := s.config.Resolver.Resolve(ctx, dest.FQDN)
		if err != nil {
			if err := sendReply(conn, ReplyHostUnreachable, nil); err != nil {
				return fmt.Errorf("failed to send reply: %v", err)
			}
			return fmt.Errorf("failed to resolve destination '%v': %v", dest.FQDN, err)
		}
		ctx = _ctx
		dest.IP = addr
	}

	// Apply any address rewrites
	req.realDestAddr = req.DestAddr
	if s.config.Rewriter != nil {
		ctx, req.realDestAddr = s.config.Rewriter.Rewrite(ctx, req)
	}

	// Switch on the command
	switch req.Command {
	case CommandConnect:
		return s.handleConnect(ctx, conn, req)
	case CommandBind:
		return s.handleBind(ctx, conn, req)
	case CommandAssociate:
		return s.handleAssociate(ctx, conn, req)
	default:
		if err := sendReply(conn, ReplyCommandNotSupported, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("unsupported command: %v", req.Command)
	}
}

func DialErrorToSocksCode(err error) byte {
	if err == nil {
		return ReplySucceeded
	}

	msg := err.Error()
	resp := ReplyHostUnreachable
	if strings.Contains(msg, "refused") {
		resp = ReplyConnectionRefused
	} else if strings.Contains(msg, "network is unreachable") {
		resp = ReplyNetworkUnreachable
	}
	return resp
}

// handleConnect is used to handle a connect command
func (s *Server) handleConnect(ctx context.Context, conn net.Conn, req *Request) error {
	// Check if this is allowed
	_ctx, ok := s.config.Rules.Allow(ctx, req)
	if !ok {
		if err := sendReply(conn, ReplyRuleFailure, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v blocked by rules", req.DestAddr)
	}
	ctx = _ctx

	return s.config.Handler.OnConnect(ctx, conn, req)
}

// handleBind is used to handle a connect command
func (s *Server) handleBind(ctx context.Context, conn net.Conn, req *Request) error {
	// Check if this is allowed
	_, ok := s.config.Rules.Allow(ctx, req)
	if !ok {
		if err := sendReply(conn, ReplyRuleFailure, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("bind to %v blocked by rules", req.DestAddr)
	}

	// TODO: Support bind
	if err := sendReply(conn, ReplyCommandNotSupported, nil); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}
	return nil
}

// handleAssociate is used to handle a connect command
func (s *Server) handleAssociate(ctx context.Context, conn net.Conn, req *Request) error {
	// Check if this is allowed
	_ctx, ok := s.config.Rules.Allow(ctx, req)
	if !ok {
		if err := sendReply(conn, ReplyRuleFailure, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("associate to %v blocked by rules", req.DestAddr)
	}
	ctx = _ctx

	return s.config.Handler.OnAssociate(ctx, conn, req)
}


/***********************************
    Requests of client:

    +------+----------+----------+
    | ATYP | DST.ADDR | DST.PORT |
    +------+----------+----------+
    |  1   | Variable |    2     |
    +------+----------+----------+
************************************/

// readAddrSpec is used to read AddrSpec.
// Expects an address type byte, followed by the address and port
func readAddrSpec(r io.Reader) (*AddrSpec, error) {
	d := &AddrSpec{}

	// Get the address type
	addrType := []byte{0}
	if _, err := r.Read(addrType); err != nil {
		return nil, err
	}

	// Handle on a per type basis
	switch addrType[0] {
	case AddressIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadAtLeast(r, addr, len(addr)); err != nil {
			return nil, err
		}
		d.IP = addr

	case AddressIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadAtLeast(r, addr, len(addr)); err != nil {
			return nil, err
		}
		d.IP = addr

	case AddressDomainName:
		if _, err := r.Read(addrType); err != nil {
			return nil, err
		}
		addrLen := int(addrType[0])
		fqdn := make([]byte, addrLen)
		if _, err := io.ReadAtLeast(r, fqdn, addrLen); err != nil {
			return nil, err
		}
		d.FQDN = string(fqdn)

	default:
		return nil, errUnrecognizedAddrType
	}

	// Read the port
	port := []byte{0, 0}
	if _, err := io.ReadAtLeast(r, port, 2); err != nil {
		return nil, err
	}
	d.Port = (int(port[0]) << 8) | int(port[1])

	return d, nil
}

func ParseHostPort(hostPort string) (*AddrSpec, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	fromAddr := &AddrSpec{ IP: net.ParseIP(host) }
	if fromAddr.IP == nil {
		fromAddr.FQDN = host
	}
	fromAddr.Port, err = strconv.Atoi(port)
	return fromAddr, err
}

// sendReply is used to send a reply message
func sendReply(w io.Writer, resp uint8, addr *AddrSpec) error {
	msg := make([]byte, 3+addr.SerializedSize())
	msg[0] = socks5Version
	msg[1] = resp
	msg[2] = 0 // Reserved
	if _, err := addr.SerializeTo(msg[3:]); err != nil {
		return err
	}

	// Send the message
	_, err := w.Write(msg)
	return err
}

func (r *Request) SendError(w io.Writer, errCode uint8) error {
	return sendReply(w, errCode, nil)
}

func (r *Request) SendConnectSuccess(w io.Writer) error {
	return sendReply(w, ReplySucceeded, nil)
}

func (r *Request) SendAssociateSuccess(w io.Writer, addr *AddrSpec) error {
	return sendReply(w, ReplySucceeded, addr)
}
