package scope

import (
	"context"
	"golang.org/x/sync/errgroup"
	"io"
)

// ContextCloser tries to loosely fill in gap between new context-aware world and old world of context-less network
// primitives with blocking operations, whose unblocking and cancellation logic is entirely different.
// Cancelling such blocking operations is often achieved by Close()-ing corresponding primitives in another goroutine.
// ContextCloser runs separate goroutine, which closes all io.Closer-s passed to ContextCloser constructor func, when
// its bind context's Done() channel is closed. This separate goroutine is finished either after context is expired
// and io.Closer-s are closed or when ContextCloser.Cancel() or ContextCloser.Close() is called, if it happens earlier.
// ContextCloser must be constructed with scope.Closer or scope.CloserWithErrorGroup func.
//
// To finish ContextCloser's goroutine before context is expired call either Close() or Cancel(), but not both!
// Close() closes all the io.Closer passed to ContextCloser constructor func regardless of context and finishes goroutine.
// Cancel() just finishes goroutine and doesn't close anything.
// Both Close() and Cancel() close inner channel without any check, so calling them both or calling any of them more
// then once leads to panic! But it is perfectly safe to call either of them once regardless of whether context was
// already expired or not.
//
// It is convenient to construct ContextCloser and call either Close() or Cancel() at once in defer statement.
// If io.Closer (e.g. a net.Conn) should be bound not only to context, but also to current func scope, then use Close():
//  defer scope.Closer(ctx, conn).Close()
// If io.Closer (e.g. a net.Conn) should be bound to context only while current goroutine is in current func (rarer
// case, that might break structured concurrency principle), then use Cancel():
//  defer scope.Closer(ctx, conn).Cancel()
type ContextCloser struct {
	ctx     context.Context
	closers []io.Closer
	cancel  chan interface{}
}

func makeCloser(ctx context.Context, closers ...io.Closer) *ContextCloser {
	return &ContextCloser{ctx, closers, make(chan interface{})}
}

func (ac *ContextCloser) closeAll() {
	for _, closer := range ac.closers {
		_ = closer.Close()
	}
}

func (ac *ContextCloser) waitForCtxDoneOrCancel() {
	select {
	case <-ac.ctx.Done():
		ac.closeAll()
	case <-ac.cancel:
	}
}

// Creates new ContextCloser, that binds all the given closers to given ctx with separate goroutine.
func Closer(ctx context.Context, closers ...io.Closer) *ContextCloser {
	ac := makeCloser(ctx, closers...)
	go ac.waitForCtxDoneOrCancel()
	return ac
}

// Creates new ContextCloser, that binds all the given closers to given ctx with new goroutine.
// New goroutine is added to given errgroup g.
// Note, that given errgroup g will be able to finish only if either given ctx will expire or some other goroutine,
// added to the errgroup will finish with error, or produced ContextCloser will be cancelled or closed explicitly,
// because ContextCloser's goroutine will be blocked forever in other cases. So, use with caution!
func CloserWithErrorGroup(ctx context.Context, g *errgroup.Group, closer ...io.Closer) *ContextCloser {
	ac := makeCloser(ctx, closer...)
	g.Go(func() error {
		ac.waitForCtxDoneOrCancel()
		return nil
	})
	return ac
}

// Cancels ContextCloser's binding and finishes it, if ctx.Done() channel was not closed yet (if it was, then does
// nothing). Panics, if called more then once, or called after Close().
func (ac *ContextCloser) Cancel() {
	close(ac.cancel)
}

// Closes all io.Closer-s of ContextCloser and finishes its goroutine, if ctx.Done() channel was not closed yet (if it
// was, then does nothing). Panics, if called more then once, or called after Cancel()
func (ac *ContextCloser) Close() {
	ac.closeAll()
	ac.Cancel()
}
