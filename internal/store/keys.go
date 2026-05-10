// Package store implements the Redis schema for Claude Mesh session and activity data.
// All Redis operations are routed through the Store interface, enabling miniredis
// backed unit tests without Docker.
package store

import "fmt"

const (
	// activeSessions is the ZSET of active session IDs (score = last_seen_ms).
	activeSessions = "claude:mesh:sessions:active"

	// activityGlobalKey is the global ring buffer of all activity events.
	activityGlobalKey = "claude:mesh:activity:global"
)

// SessionKey returns the Hash key for a session.
func SessionKey(sid string) string {
	return fmt.Sprintf("claude:mesh:session:%s", sid)
}

// ActivityKey returns the per-session List key.
func ActivityKey(sid string) string {
	return fmt.Sprintf("claude:mesh:activity:%s", sid)
}
