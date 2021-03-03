package scope

import (
	"context"
	"golang.org/x/sync/errgroup"
	"io"
)

// ContextGroup is basically a convenient wrapper for errgroup.Group and optional scope.ContextCloser-s.
// ContextGroup must be constructed with scope.Group. Any number of scope.ContextCloser-s can be added to it using
// Add[After]Closer[Cancelling]() functions.
type ContextGroup struct {
	ctx    context.Context
	eg     *errgroup.Group
	before []*closerWithFlag
	after  []*closerWithFlag
}

// Creates ContextGroup, wrapping new errgroup.Group
func Group(ctx context.Context) (*ContextGroup, context.Context) {
	eg, ctx := errgroup.WithContext(ctx)
	return &ContextGroup{ctx: ctx, eg: eg}, ctx
}

type closerWithFlag struct {
	cc     *ContextCloser
	cancel bool
}

func (f *closerWithFlag) finish() {
	if f.cancel {
		f.cc.Cancel()
	} else {
		f.cc.Close()
	}
}

func (g *ContextGroup) addBeforeCloser(cancel bool, closers ...io.Closer) *ContextGroup {
	g.before = append(g.before, &closerWithFlag{CloserWithErrorGroup(g.ctx, g.eg, closers...), cancel})
	return g
}

func (g *ContextGroup) addAfterCloser(cancel bool, closers ...io.Closer) *ContextGroup {
	g.after = append(g.after, &closerWithFlag{Closer(g.ctx, closers...), cancel})
	return g
}

// Adds new ContextCloser, that binds all given closers to group context. ContextCloser will be automatically closed
// when any Wait*() func is called BEFORE waiting for inner errgroup.
func (g *ContextGroup) AddCloser(closers ...io.Closer) *ContextGroup {
	return g.addBeforeCloser(false, closers...)
}

// Adds new ContextCloser, that binds all given closers to group context. ContextCloser will be automatically closed
// when any Wait*() func is called AFTER waiting for inner errgroup.
func (g *ContextGroup) AddAfterCloser(closers ...io.Closer) *ContextGroup {
	return g.addAfterCloser(false, closers...)
}

// Adds new ContextCloser, that binds all given closers to group context. ContextCloser will be automatically cancelled
// when any Wait*() func is called BEFORE waiting for inner errgroup.
func (g *ContextGroup) AddCloserCancelling(closers ...io.Closer) *ContextGroup {
	return g.addBeforeCloser(true, closers...)
}

// Adds new ContextCloser, that binds all given closers to group context. ContextCloser will be automatically cancelled
// when any Wait*() func is called AFTER waiting for inner errgroup.
func (g *ContextGroup) AddAfterCloserCancelling(closers ...io.Closer) *ContextGroup {
	return g.addAfterCloser(true, closers...)
}

// Returns group's context
func (g *ContextGroup) Ctx() context.Context {
	return g.ctx
}

// Adds new goroutine to the group. Simply calls Go() func of inner errgroup
func (g *ContextGroup) Go(f func() error) {
	g.eg.Go(f)
}

// Adds new goroutine, that can't return error to the group. Convenient wrapper for returning nil after f to fulfill
// errgroup's requirements
func (g *ContextGroup) GoNoError(f func()) {
	g.Go(func() error {
		f()
		return nil
	})
}

// Cancels group's context
func (g *ContextGroup) Cancel() {
	g.Go(func() error { return context.Canceled })
}

// Adds new goroutine, that can't return error to the group. Convenient wrapper for returning nil after f to fulfill
// errgroup's requirements.
func (g *ContextGroup) Wait() error {
	for _, f := range g.before {
		f.finish()
	}
	err := g.eg.Wait()
	for _, f := range g.after {
		f.finish()
	}
	return err
}

// Wait()-s, ignoring any errors. Convenient wrapper for usage in defer statements and avoid linter warnings on
// unhandled error
func (g *ContextGroup) WaitQuietly() {
	_ = g.Wait()
}

// Wait()-s, and sets resulting error to its argument if it points to nil or ignores resulting error otherwise.
// Convenient wrapper for usage in defer statements and setting func result error (as named return value), if it wasn't
// already set earlier, e.g. with common return statement, or with assignment in func code
func (g *ContextGroup) WaitAndSetErrorIfNotYet(err *error) {
	e := g.Wait()
	if *err == nil {
		*err = e
	}
}
