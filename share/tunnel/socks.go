package tunnel

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/jpillora/chisel/share/settings"
)

// A separate destination keeps legacy peers from accepting a UDP channel as
// an ordinary SOCKS stream. The existing TCP SOCKS protocol stays unchanged.
const socksUDPDestination = "socks-udp-v1"

// Older peers may accept a channel before attempting their TCP dial. Require
// an explicit marker before reporting UDP ASSOCIATE success to the client.
const socksUDPReady = "chisel-socks-udp-v1\n"

const (
	socksSuccess     = 0
	socksFailure     = 1
	socksDenied      = 2
	socksUnsupported = 7
	socksBadAddress  = 8
)

var errSocksAddress = errors.New("unsupported SOCKS address type")

type socksAddress struct {
	host string
	port uint16
	wire []byte
}

func readSocksAddress(r io.Reader) (socksAddress, error) {
	a := socksAddress{}
	head := make([]byte, 1)
	if _, err := io.ReadFull(r, head); err != nil {
		return a, err
	}
	n := 0
	switch head[0] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(r, length); err != nil {
			return a, err
		}
		if length[0] == 0 {
			return a, errors.New("empty SOCKS hostname")
		}
		head = append(head, length[0])
		n = int(length[0])
	default:
		return a, errSocksAddress
	}
	body := make([]byte, n+2)
	if _, err := io.ReadFull(r, body); err != nil {
		return a, err
	}
	if head[0] == 3 {
		a.host = string(body[:n])
	} else {
		a.host = net.IP(body[:n]).String()
	}
	a.port = binary.BigEndian.Uint16(body[n:])
	a.wire = append(head, body...)
	return a, nil
}

func (a socksAddress) destination() string {
	return net.JoinHostPort(a.host, fmt.Sprint(a.port))
}

func socksIPPort(ip net.IP, port int) []byte {
	atyp := byte(4)
	if v4 := ip.To4(); v4 != nil {
		ip, atyp = v4, 1
	} else {
		ip = ip.To16()
	}
	b := append([]byte{atyp}, ip...)
	return binary.BigEndian.AppendUint16(b, uint16(port))
}

func socksReply(w io.Writer, code byte, addr []byte) error {
	if addr == nil {
		addr = []byte{1, 0, 0, 0, 0, 0, 0}
	}
	return writeSocksBytes(w, append([]byte{5, code, 0}, addr...))
}

func parseSocksUDP(b []byte) (socksAddress, []byte, error) {
	if len(b) < 4 || b[0] != 0 || b[1] != 0 || b[2] != 0 {
		return socksAddress{}, nil, errors.New("invalid or fragmented SOCKS UDP packet")
	}
	a, err := readSocksAddress(bytes.NewReader(b[3:]))
	if err != nil {
		return a, nil, err
	}
	if a.port == 0 {
		return a, nil, errors.New("zero SOCKS UDP destination port")
	}
	return a, b[3+len(a.wire):], nil
}

func socksUDPTooLarge(n, limit int, err error) bool {
	// Unix truncates UDP reads; Windows may return WSAEMSGSIZE (10040)
	// instead. Treat both as dropped datagrams, not listener/flow failures.
	return (err == nil && n > limit) || errors.Is(err, syscall.EMSGSIZE) ||
		(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10040)))
}

func writeSocksBytes(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

type socksUDPStream struct {
	io.ReadWriteCloser
	max int
	mu  sync.Mutex // IPv4 and IPv6 readers share the reply stream
}

func (s *socksUDPStream) readPacket() ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(s, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || uint64(n) > uint64(s.max) {
		return nil, errors.New("invalid SOCKS UDP frame size")
	}
	b := make([]byte, int(n))
	_, err := io.ReadFull(s, b)
	return b, err
}

func (s *socksUDPStream) writePacket(b []byte) error {
	if len(b) == 0 || len(b) > s.max {
		return errors.New("invalid SOCKS UDP frame size")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(b)))
	if err := writeSocksBytes(s, header[:]); err != nil {
		return err
	}
	return writeSocksBytes(s, b)
}

func socksUDPLimits() (size, conns int, deadline time.Duration) {
	size = settings.EnvInt("UDP_MAX_SIZE", 9012)
	if size <= 0 {
		size = 9012
	}
	if size > 65507 {
		size = 65507
	}
	conns = settings.EnvInt("UDP_MAX_CONNS", 100)
	if conns <= 0 {
		conns = 100
	}
	deadline = settings.EnvDuration("UDP_DEADLINE", 15*time.Second)
	if deadline <= 0 {
		deadline = 15 * time.Second
	}
	return
}
