//+build windows

package cos

import (
	"time"
)

func GoStats() {
	//noop
}

func AfterSignal(d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		<-time.After(d)
		close(ch)
	}()
	return ch
}
