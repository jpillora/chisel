package cio

import (
	"io"
	"log"
	"sync"
)

func Pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {
	var sent, received int64
	var wg sync.WaitGroup
	var o sync.Once
	close := func() {
		src.Close()
		dst.Close()
	}
	wg.Add(2)
	go func() {
		received, _ = io.Copy(src, dst)
		o.Do(close)
		wg.Done()
	}()
	go func() {
		sent, _ = io.Copy(dst, src)
		o.Do(close)
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
