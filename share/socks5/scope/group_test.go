package scope

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDeferGroupWaitsForChildren(t *testing.T) {
	p := &logOnClose{}

	func(ctx context.Context) {
		g, _ := Group(ctx)
		defer g.WaitQuietly()

		g.GoNoError(func() { p.print(logChild1) })
		g.GoNoError(func() {
			time.Sleep(50 * time.Millisecond)
			p.print(logChild2)
		})
		g.GoNoError(func() {
			time.Sleep(100 * time.Millisecond)
			p.print(logChild3)
		})
	}(context.Background())

	require.Truef(t, p.checkLog(logChild1, logChild2, logChild3),
		"Group does not wait for children (%v)!", p.log)
}

func TestDeferGroupWithCloserClosesAndWaitsForChildren(t *testing.T) {
	p := &logOnClose{}

	func(ctx context.Context) {
		g, _ := Group(ctx)
		defer g.AddCloser(p).WaitQuietly()

		g.GoNoError(func() {
			time.Sleep(50 * time.Millisecond)
			p.print(logChild2)
		})
		g.GoNoError(func() {
			time.Sleep(100 * time.Millisecond)
			p.print(logChild3)
		})
	}(context.Background())

	require.Truef(t, p.checkLog(logClose, logChild2, logChild3),
		"GroupWithCloser does not close or does not wait for children (%v)!", p.log)
}

func TestDeferWaitAndSetErrorIfNotYetSetsNilErrorToNonNil(t *testing.T) {
	msg := "from child"

	err := func(ctx context.Context) (e error) {
		g, _ := Group(ctx)
		defer g.WaitAndSetErrorIfNotYet(&e)

		g.Go(func() error { return errors.New(msg) })
		return nil
	}(context.Background())

	require.Truef(t, err != nil && err.Error() == msg, "WaitAndSetErrorIfNotYet does not set nil error!")
}

func TestDeferWaitAndSetErrorIfNotYetDoesNotResetError(t *testing.T) {
	msg := "from parent"

	err := func(ctx context.Context) (e error) {
		g, _ := Group(ctx)
		defer g.WaitAndSetErrorIfNotYet(&e)

		g.Go(func() error { return errors.New("from child") })
		return errors.New(msg)
	}(context.Background())

	require.Truef(t, err != nil && err.Error() == msg, "WaitAndSetErrorIfNotYet resets non-nil error!")
}

func TestDeferWaitAndSetErrorIfNotYetSetsNilErrorToNil(t *testing.T) {
	err := func(ctx context.Context) (e error) {
		g, _ := Group(ctx)
		defer g.WaitAndSetErrorIfNotYet(&e)

		g.GoNoError(func() {})
		return
	}(context.Background())

	require.Nilf(t, err, "WaitAndSetErrorIfNotYet erroneously resets nil error to non-nil!")
}

func TestDeferWaitAndSetErrorIfNotYetDoesNotResetErrorToNil(t *testing.T) {
	msg := "from parent"

	err := func(ctx context.Context) (e error) {
		g, _ := Group(ctx)
		defer g.WaitAndSetErrorIfNotYet(&e)

		g.GoNoError(func() {})
		return errors.New(msg)
	}(context.Background())

	require.Truef(t, err != nil && err.Error() == msg, "WaitAndSetErrorIfNotYet resets non-nil error!")
}
