package tunnel

import (
	"sync"
	"sync/atomic"
)

type waitGroup struct {
	inner sync.WaitGroup
	n     int32
}

func (w *waitGroup) Add(n int) {
	atomic.AddInt32(&w.n, int32(n))
	w.inner.Add(n)
}

func (w *waitGroup) Done() {
	n := atomic.AddInt32(&w.n, int32(-1))
	if n >= 0 {
		w.inner.Done()
	}
}

func (w *waitGroup) Wait() {
	w.inner.Wait()
}

func (w *waitGroup) DoneAll() {
	for atomic.LoadInt32(&w.n) > 0 {
		w.Done()
	}
}
