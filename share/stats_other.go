// +build !linux,!darwin

package chshare

//ShowStats prints statistics to
//stdout on SIGUSR1 (posix-only)
func ShowStats() {
	//noop
}
