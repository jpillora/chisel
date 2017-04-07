// +build !windows,!linux,!darwin,!freebsd,cgo

package termutil

/*
#include <unistd.h>
*/
import "C"

import "os"

func Isatty(fd uintptr) bool {
    return int(C.isatty(C.int(fd))) != 0
}
