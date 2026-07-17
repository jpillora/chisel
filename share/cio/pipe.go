package cio

import (
	"io"
	"sync"
)

//closeWriter is implemented by connections which support
//half-close (*net.TCPConn, ssh.Channel, cnet rwcConn)
type closeWriter interface {
	CloseWrite() error
}

//Pipe copies between src and dst in both directions, propagating
//half-closes: when one direction reaches a clean EOF, the write side
//of its destination is closed (CloseWrite) while the other direction
//keeps flowing. Copy errors tear down both ends. Both connections
//are fully closed before Pipe returns.
func Pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {
	var sent, received int64
	var wg sync.WaitGroup
	teardown := func() {
		src.Close()
		dst.Close()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		sent, err = io.Copy(dst, src)
		if err != nil {
			teardown()
			return
		}
		closeWrite(dst)
	}()
	go func() {
		defer wg.Done()
		var err error
		received, err = io.Copy(src, dst)
		if err != nil {
			teardown()
			return
		}
		closeWrite(src)
	}()
	wg.Wait()
	teardown()
	return sent, received
}

//closeWrite half-closes the connection when supported,
//falling back to a full close
func closeWrite(c io.ReadWriteCloser) {
	if cw, ok := c.(closeWriter); ok {
		cw.CloseWrite()
		return
	}
	c.Close()
}
