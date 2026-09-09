//go:build darwin || linux

package preferences

import (
	"fmt"
	"os"
	"syscall"
)

// Keep the lock path after close so all instances continue to lock the same inode.
func lockFile(path string) (*os.File, error) {
	// #nosec G304 -- The caller supplies the application preferences lock path.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	fd := f.Fd()
	if fd > uintptr(^uint(0)>>1) {
		_ = f.Close()
		return nil, fmt.Errorf("invalid preferences lock descriptor")
	}
	// #nosec G115 -- The descriptor was bounded to the platform int range.
	if err := syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("preferences are in use; try again: %w", err)
	}
	return f, nil
}
