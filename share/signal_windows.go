//+build windows

package chshare

import "time"

//Sleep unless Signal
func SleepSignal(d time.Duration) {
	time.Sleep(d) //not supported
}
