// Package lock provides a simple PID-file-based instance guard for daemons.
//
// Usage:
//
//	if err := lock.Acquire("/tmp/my-daemon.pid"); err != nil {
//	    if errors.Is(err, lock.ErrAlreadyHeld) {
//	        // Another instance is already running — exit cleanly.
//	        return nil
//	    }
//	    return fmt.Errorf("acquire lock: %w", err)
//	}
//	defer lock.Release("/tmp/my-daemon.pid")
package lock

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// ErrAlreadyHeld is returned by Acquire when another live process owns the lock.
var ErrAlreadyHeld = errors.New("lock already held by another process")

// Acquire writes the current PID to lockPath and returns nil.
//
// If the file already exists, Acquire checks whether the recorded PID corresponds
// to a live process (via kill -0). If the process is alive, ErrAlreadyHeld is
// returned. If the PID is dead or the content is invalid (stale lock), the file
// is overwritten with the current PID and nil is returned.
func Acquire(lockPath string) error {
	data, err := os.ReadFile(lockPath)
	if err == nil {
		// File exists — check if the recorded PID is alive.
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && pid > 0 {
			// kill -0 sends no signal but checks whether the process exists and
			// is reachable. ESRCH means no such process (stale); nil means alive.
			proc, findErr := os.FindProcess(pid)
			if findErr == nil {
				if sigErr := proc.Signal(syscall.Signal(0)); sigErr == nil {
					// Process is alive — lock is held.
					return ErrAlreadyHeld
				}
				// sigErr != nil (ESRCH or EPERM) → treat as dead / stale.
			}
		}
		// Stale or invalid content — fall through to overwrite.
	} else if !os.IsNotExist(err) {
		// Unexpected read error — propagate.
		return fmt.Errorf("lock: read %s: %w", lockPath, err)
	}

	// Write current PID atomically via a temp file + rename.
	content := []byte(strconv.Itoa(os.Getpid()))
	tmp := lockPath + ".tmp"
	if writeErr := os.WriteFile(tmp, content, 0o600); writeErr != nil {
		return fmt.Errorf("lock: write %s: %w", lockPath, writeErr)
	}
	if renameErr := os.Rename(tmp, lockPath); renameErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("lock: rename %s: %w", lockPath, renameErr)
	}
	return nil
}

// Release removes the lock file. It is safe to call multiple times; a missing
// file is not considered an error.
func Release(lockPath string) error {
	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("lock: release %s: %w", lockPath, err)
	}
	return nil
}
