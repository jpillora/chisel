package link

import (
	"io"
	"sync"
)

func Pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {
	var sent, received int64
	var wg sync.WaitGroup
	var o sync.Once
	closeFn := func() {
		src.Close()
		dst.Close()
	}

	receivedFn := func() {
		received, _ = io.Copy(src, dst)
		o.Do(closeFn)
		wg.Done()
	}

	sentFn := func() {
		sent, _ = io.Copy(dst, src)
		o.Do(closeFn)
		wg.Done()
	}

	wg.Add(2)
	go receivedFn()
	go sentFn()

	wg.Wait()
	return sent, received
}
