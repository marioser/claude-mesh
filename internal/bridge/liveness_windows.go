//go:build windows

package bridge

// isProcessAlive is a no-op fallback on Windows.
//
// The bridge currently ships only as a launchd agent (darwin) or systemd
// unit (linux); Windows support has not been validated. Returning false
// here means the liveness optimization is disabled on Windows and sessions
// rely purely on the configurable session TTL for retention, which is the
// pre-liveness behavior.
//
// If/when Windows becomes a supported platform, replace this with an
// OpenProcess-based liveness check via golang.org/x/sys/windows.
func isProcessAlive(_ int) bool {
	return false
}
