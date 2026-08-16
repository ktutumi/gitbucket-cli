//go:build windows

package state

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const (
	lockEx = windows.LOCKFILE_EXCLUSIVE_LOCK
	lockNb = windows.LOCKFILE_FAIL_IMMEDIATELY
	lockUn = 0

	errWouldBlock = windows.ERROR_LOCK_VIOLATION
	errAgain      = windows.ERROR_LOCK_VIOLATION
)

func flock(fd uintptr, how int) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(fd), uint32(how), 0, 1, 0, &overlapped)
	if err != nil {
		return fmt.Errorf("lock file: %w", err)
	}
	return nil
}

func unlock(fd uintptr) error {
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(windows.Handle(fd), 0, 1, 0, &overlapped)
	if err != nil {
		return fmt.Errorf("unlock file: %w", err)
	}
	return nil
}
