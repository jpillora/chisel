// +build !windows,!linux,!darwin,!freebsd,!cgo

package termutil

func Isatty(fd uintptr) bool {
    panic("Not implemented")
}
