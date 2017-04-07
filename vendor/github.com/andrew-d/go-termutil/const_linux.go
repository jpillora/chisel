// +build linux

package termutil

import "syscall"

const ioctlReadTermios = syscall.TCGETS
const ioctlWriteTermios = syscall.TCSETS
