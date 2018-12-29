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
