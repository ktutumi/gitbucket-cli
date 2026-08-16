//go:build darwin

package state

import (
	"fmt"
	"syscall"
)

const (
	lockEx = syscall.LOCK_EX
	lockNb = syscall.LOCK_NB
	lockUn = syscall.LOCK_UN

	errWouldBlock = syscall.EWOULDBLOCK
	errAgain      = syscall.EAGAIN
)

func flock(fd uintptr, how int) error {
	err := syscall.Flock(int(fd), how)
	if err != nil {
		return fmt.Errorf("flock: %w", err)
	}
	return nil
}

func unlock(fd uintptr) error {
	err := syscall.Flock(int(fd), lockUn)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	return nil
}
