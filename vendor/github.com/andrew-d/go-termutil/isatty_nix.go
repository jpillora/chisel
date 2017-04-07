// +build linux darwin freebsd

package termutil

import (
    "syscall"
    "unsafe"
)

func Isatty(fd uintptr) bool {
    var termios syscall.Termios

    _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd,
        uintptr(ioctlReadTermios),
        uintptr(unsafe.Pointer(&termios)),
        0,
        0,
        0)
    return err == 0
}
