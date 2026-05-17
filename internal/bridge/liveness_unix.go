//go:build !windows

package bridge

import (
	"errors"
	"os"
	"syscall"
)

// isProcessAlive reports whether a process with the given PID is alive
// in the current host's process namespace. It uses signal 0 ("null signal"),
// which performs the standard permission and existence checks without
// actually delivering anything to the target.
//
// Semantics:
//   - pid <= 0: not alive (defensive — invalid PID).
//   - kill(pid, 0) returns nil: process exists and we can signal it.
//   - errno ESRCH: process does not exist → not alive.
//   - errno EPERM: process exists but we lack permission → treated as ALIVE
//     to avoid false eviction of long-running sessions owned by another user.
//   - any other error: treated as not alive (defensive).
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
