// +build linux darwin

package chshare

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jpillora/sizestr"
)

//ShowStats prints statistics to
//stdout on SIGUSR1 (posix-only)
func ShowStats() {
	time.Sleep(time.Second)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	for range c {
		memStats := runtime.MemStats{}
		runtime.ReadMemStats(&memStats)
		log.Printf("# go-routines: %d", runtime.NumGoroutine())
		log.Printf(" memory usage: %s", sizestr.ToString(int64(memStats.Alloc)))
	}
}
