package chshare

import (
	"io"
	"sync"
)

func Pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {

	var sent, received int64
	var c = make(chan bool)
	var o sync.Once

	close := func() {
		src.Close()
		dst.Close()
		close(c)
	}

	go func() {
		received, _ = io.Copy(src, dst)
		o.Do(close)
	}()

	go func() {
		sent, _ = io.Copy(dst, src)
		o.Do(close)
	}()

	<-c
	return sent, received
}
