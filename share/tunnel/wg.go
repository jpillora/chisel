package tunnel

import "sync"

type waitGroup struct {
	mu    sync.Mutex
	inner sync.WaitGroup
	n     int
}

func (w *waitGroup) Add(n int) {
	w.mu.Lock()
	w.n += n
	w.inner.Add(n)
	w.mu.Unlock()
}

func (w *waitGroup) Done() {
	w.mu.Lock()
	if w.n > 0 {
		w.n--
		w.inner.Done()
	}
	w.mu.Unlock()
}

func (w *waitGroup) DoneAll() {
	w.mu.Lock()
	for w.n > 0 {
		w.n--
		w.inner.Done()
	}
	w.mu.Unlock()
}

func (w *waitGroup) Wait() {
	w.inner.Wait()
}
