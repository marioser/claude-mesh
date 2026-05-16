package lock_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/marioser/claude-mesh/internal/lock"
)

// tempLockPath returns a lock path in a temp dir that is cleaned up after the test.
func tempLockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.pid")
}

// TestAcquireNewFile — no lock file exists → Acquire should succeed and write PID.
func TestAcquireNewFile(t *testing.T) {
	path := tempLockPath(t)

	if err := lock.Acquire(path); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
	if string(data) == "" {
		t.Fatal("lock file is empty, expected PID")
	}
}

// TestAcquireAlreadyHeld — lock file exists with PID of current process (alive) → ErrAlreadyHeld.
func TestAcquireAlreadyHeld(t *testing.T) {
	path := tempLockPath(t)

	// Write the PID of the current process — it is definitely alive.
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := lock.Acquire(path)
	if err == nil {
		t.Fatal("expected ErrAlreadyHeld, got nil")
	}
	if err != lock.ErrAlreadyHeld {
		t.Fatalf("expected ErrAlreadyHeld, got: %v", err)
	}
}

// TestAcquireStaleLock — lock file exists with PID that is definitely dead → override OK.
func TestAcquireStaleLock(t *testing.T) {
	path := tempLockPath(t)

	// PID 99999999 is astronomically unlikely to exist on any real system.
	if err := os.WriteFile(path, []byte("99999999"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := lock.Acquire(path); err != nil {
		t.Fatalf("expected nil (stale lock override), got: %v", err)
	}
}

// TestRelease — after Acquire, Release removes the file.
func TestRelease(t *testing.T) {
	path := tempLockPath(t)

	if err := lock.Acquire(path); err != nil {
		t.Fatalf("setup Acquire: %v", err)
	}

	if err := lock.Release(path); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected lock file to be removed after Release")
	}
}

// TestReleaseNonExistent — Release of a file that doesn't exist is not an error.
func TestReleaseNonExistent(t *testing.T) {
	path := tempLockPath(t)

	if err := lock.Release(path); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

// TestAcquireInvalidContent — lock file contains non-numeric content → treat as stale, override.
func TestAcquireInvalidContent(t *testing.T) {
	path := tempLockPath(t)

	if err := os.WriteFile(path, []byte("not-a-number"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := lock.Acquire(path); err != nil {
		t.Fatalf("expected nil (invalid content treated as stale), got: %v", err)
	}
}
