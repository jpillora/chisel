package client

//proxy conn        server  | clients
//                 /-> ws <-|-> ws <--> local <-
//         proxy <---> ws <-|-> ws <--> local <-
//                 \-> ws <-|-> ws <--> local <-
import (
	"encoding/binary"
	"log"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/jpillora/chisel"
)

type ProxyServer struct {
	id      int
	addr    string
	session *yamux.Session
}

func NewProxyServer(id int, addr string, session *yamux.Session) *ProxyServer {
	return &ProxyServer{
		id:      id,
		addr:    addr,
		session: session,
	}
}

func (r *ProxyServer) start() {

	a, err := net.ResolveTCPAddr("tcp4", r.addr)
	if err != nil {
		log.Fatal(err)
	}

	l, err := net.ListenTCP("tcp4", a)
	if err != nil {
		log.Fatal(err)
	}

	for {
		src, err := l.Accept()
		if err != nil {
			log.Println(err)
			return
		}

		dst, err := r.session.Open()
		if err != nil {
			log.Println(err)
			return
		}

		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(r.id))
		dst.Write(b)

		chisel.Pipe(src, dst)
	}
}

func (r *ProxyServer) Send() {

}

func (r *ProxyServer) Recieve() {

}
