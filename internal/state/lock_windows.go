//go:build windows

package state

import "syscall"

const (
	lockEx = 0
	lockNb = 0
	lockUn = 0
)

var (
	errWouldBlock = syscall.Errno(0)
	errAgain      = syscall.Errno(0)
)

func flock(fd uintptr, how int) error {
	return errUnsupported
}

func unlock(fd uintptr) error {
	return nil
}

var errUnsupported = syscall.Errno(1 << 30)
