//+build !windows

package cos

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jpillora/sizestr"
)

//GoStats prints statistics to
//stdout on SIGUSR2 (posix-only)
func GoStats() {
	//silence complaints from windows
	const SIGUSR2 = syscall.Signal(0x1f)
	time.Sleep(time.Second)
	c := make(chan os.Signal, 1)
	signal.Notify(c, SIGUSR2)
	for range c {
		memStats := runtime.MemStats{}
		runtime.ReadMemStats(&memStats)
		log.Printf("recieved SIGUSR2, go-routines: %d, go-memory-usage: %s",
			runtime.NumGoroutine(),
			sizestr.ToString(int64(memStats.Alloc)))
	}
}

//AfterSignal returns a channel which will be closed
//after the given duration or until a SIGHUP is received
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
