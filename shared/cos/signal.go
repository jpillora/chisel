//go:build !windows
// +build !windows

package cos

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// AfterSignal returns a channel which will be closed
// after the given duration or until a SIGHUP is received
func AfterSignal(d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		select {
		case <-time.After(d):
		case <-sig:
		}
		signal.Stop(sig)
		close(ch)
	}()
	return ch
}
