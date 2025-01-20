package tunnel

import (
	"io"
	"sync"
)

func Pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {
	var sent, received int64
	var wg sync.WaitGroup
	var o sync.Once
	closePipeExecutor := func() { closePipe(src, dst) }
	wg.Add(2)
	go copeRecievedBytes(&received, src, dst, o, closePipeExecutor, wg)
	go copySentBytes(&sent, dst, src, o, closePipeExecutor, wg)
	wg.Wait()
	return sent, received
}

func copySentBytes(sent *int64, dst io.ReadWriteCloser, src io.ReadWriteCloser, o sync.Once, closePipeExecutor func(), wg sync.WaitGroup) {
	s, err := io.Copy(dst, src)
	if err == nil {
		return
	}
	o.Do(closePipeExecutor)
	wg.Done()
	sent = &s
}

func copeRecievedBytes(received *int64, src io.ReadWriteCloser, dst io.ReadWriteCloser, o sync.Once, closePipeExecutor func(), wg sync.WaitGroup) {
	r, err := io.Copy(src, dst)
	if err == nil {
		return
	}
	o.Do(closePipeExecutor)
	wg.Done()
	received = &r
}

func closePipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) {
	src.Close()
	dst.Close()
}
