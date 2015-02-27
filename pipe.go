package chisel

import (
	"io"
	"net"
	"sync"
)

func Pipe(src net.Conn, dst net.Conn) {

	var c = make(chan bool)
	var o sync.Once

	close := func() {
		src.Close()
		dst.Close()
		close(c)
	}

	go func() {
		io.Copy(src, dst)
		o.Do(close)
	}()

	go func() {
		io.Copy(dst, src)
		o.Do(close)
	}()

	<-c
}
