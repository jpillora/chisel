//+build !windows

package chshare

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

//SleepSignal sleeps for the given duration,
//or until a SIGHUP is received
func SleepSignal(d time.Duration) {
	//during this time, also listen for SIGHUP
	//(this uses 0xc to allow windows to compile)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP)
	select {
	case <-time.After(d):
	case <-sig:
	}
	signal.Stop(sig)
}
