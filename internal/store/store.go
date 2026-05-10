package store

import (
	"context"

	"claude-mesh/internal/events"
)

// SessionView is the read model returned by GetSession and ListActiveSessions.
type SessionView struct {
	ID             string  `json:"session_id"`
	Cwd            string  `json:"cwd"`
	GitBranch      string  `json:"git_branch"`
	Host           string  `json:"host"`
	TranscriptPath string  `json:"transcript_path"`
	PID            int     `json:"pid"`
	OpenedAtMs     float64 `json:"opened_at_ms"`
	LastSeenMs     float64 `json:"last_seen_ms"`
}

// StoreConfig holds TTL and ring size configuration, injected from EnvOptions.
type StoreConfig struct {
	SessionTTL         int // seconds
	ActivityPerSessTTL int // seconds
	ActivityGlobalTTL  int // seconds
	ActivityRingSize   int // max items in per-session list
	GlobalRingSize     int // max items in global list
}

// DefaultConfig returns the design-specified default TTLs and ring sizes.
func DefaultConfig() StoreConfig {
	return StoreConfig{
		SessionTTL:         90,
		ActivityPerSessTTL: 600,
		ActivityGlobalTTL:  1800,
		ActivityRingSize:   50,
		GlobalRingSize:     200,
	}
}

// Store is the interface that isolates bridge and MCP handlers from Redis.
// All methods accept a context so the caller can enforce latency budgets.
type Store interface {
	// OpenSession writes a new session Hash, sets TTL, and adds to the active ZSET.
	OpenSession(ctx context.Context, ev events.SessionOpen) error

	// TouchSession updates last_seen, resets TTL, and updates the ZSET score.
	TouchSession(ctx context.Context, sid string, lastSeenMs float64) error

	// CloseSession removes the session from the active ZSET and sets a short EXPIRE.
	CloseSession(ctx context.Context, sid string) error

	// GetSession fetches session metadata. Returns nil if not found.
	GetSession(ctx context.Context, sid string) (*SessionView, error)

	// ListActiveSessions returns all members of the active ZSET with their Hash data.
	ListActiveSessions(ctx context.Context) ([]SessionView, error)

	// PushActivity appends to per-session and global ring buffers.
	// If the session Hash doesn't exist, only the global ring is updated.
	PushActivity(ctx context.Context, ev events.Activity) error

	// RecentActivity returns up to limit activity events, newest first.
	// If sid is empty, returns from the global ring; otherwise from the per-session ring.
	RecentActivity(ctx context.Context, limit int, sid string) ([]events.Activity, error)

	// SweepExpired removes ZSET members with score < cutoffMs. Returns count removed.
	SweepExpired(ctx context.Context, cutoffMs float64) (int, error)

	// HealthCheck pings Redis. Returns nil if reachable.
	HealthCheck(ctx context.Context) error
}
