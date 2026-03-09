package cio

import (
	"io"
	"log"
	"sync"
)

type ReadWriteWriterCloser interface {
	io.ReadWriteCloser
	CloseWrite() error
}

func Pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {
	var sent, received int64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		received, _ = io.Copy(dst, src)
		if dst2, ok := dst.(ReadWriteWriterCloser); ok {
			dst2.CloseWrite()
		} else {
			dst.Close()
		}
		wg.Done()
	}()
	go func() {
		sent, _ = io.Copy(src, dst)
		if src2, ok := src.(ReadWriteWriterCloser); ok {
			src2.CloseWrite()
		} else {
			src.Close()
		}
		wg.Done()
	}()
	wg.Wait()
	return sent, received
}

const vis = false

type pipeVisPrinter struct {
	name string
}

func (p pipeVisPrinter) Write(b []byte) (int, error) {
	log.Printf(">>> %s: %x", p.name, b)
	return len(b), nil
}

func pipeVis(name string, r io.Reader) io.Reader {
	if vis {
		return io.TeeReader(r, pipeVisPrinter{name})
	}
	return r
}
