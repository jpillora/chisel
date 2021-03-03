package scope

import (
	"context"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"sync"
	"testing"
	"time"
)

const (
	logBeforeClose = iota
	logChild1
	logChild2
	logChild3
	logClose
	logAfterClose
)

type logOnClose struct {
	log []int
	mtx sync.Mutex
}

func (p *logOnClose) print(e int) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.log = append(p.log, e)
}

func (p *logOnClose) Close() error {
	p.print(logClose)
	return nil
}

func (p *logOnClose) checkLog(expected ...int) bool {
	if len(p.log) != len(expected) {
		return false
	}
	for i := 0; i != len(expected); i++ {
		if p.log[i] != expected[i] {
			return false
		}
	}
	return true
}

func TestDeferCloserCancelDoesntCloseIfContextIsNotDone(t *testing.T) {
	p := &logOnClose{}

	g, ctx := errgroup.WithContext(context.Background())
	func(ctx context.Context) {
		defer CloserWithErrorGroup(ctx, g, p).Cancel()
		p.print(logBeforeClose)
	}(ctx)
	err := g.Wait()
	require.Nilf(t, err, "Closer goroutine reports error: %v", err)

	require.Truef(t, p.checkLog(logBeforeClose), "Closer closes, when context is not Done (%v)!", p.log)
}

func TestCloserClosesWhenContextIsDone(t *testing.T) {
	p := &logOnClose{}

	err := func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		g, _ := Group(ctx)
		g.AddCloserCancelling(p)
		p.print(logBeforeClose)
		cancel()
		time.Sleep(1 * time.Nanosecond) // shame: to let Closer goroutine choose the right way in its select ..
		err := g.Wait()
		p.print(logAfterClose)
		return err
	}(context.Background())
	require.Nilf(t, err, "Closer goroutine reports error: %v", err)

	require.Truef(t, p.checkLog(logBeforeClose, logClose, logAfterClose),
		"Closer does not close, when context is Done (%v)!", p.log)
}

func TestDeferCloserCloseCloses(t *testing.T) {
	p := &logOnClose{}

	func(ctx context.Context) {
		defer Closer(ctx, p).Close()
		p.print(logBeforeClose)
	}(context.Background())

	require.Truef(t, p.checkLog(logBeforeClose, logClose),
		"Closer does not close, when Close() is called (%v)!", p.log)
}
