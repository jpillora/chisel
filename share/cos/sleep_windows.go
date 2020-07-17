//+build windows

package cos

import "time"

//Sleep unless Signal
func SleepSignal(d time.Duration) {
	time.Sleep(d) //not supported
}
