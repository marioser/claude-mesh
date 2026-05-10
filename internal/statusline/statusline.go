// Package statusline renders a single-line status string for Claude Code's statusLine config.
// Format: 🌳 <branch> │ 🔵 <N> sesión(es) │ 📡 <N> eventos/5m │ <icon> daemon
//
// Performance: Render must complete within the caller's context deadline.
// If Redis is unreachable or the context is canceled, a minimal fallback line is returned
// silently — no errors are written to stderr.
package statusline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"claude-mesh/internal/store"
)

const (
	sep            = " │ "
	windowMs       = 5 * 60 * 1000 // 5 minutes in milliseconds
)

// Render returns the formatted statusline string.
// branch is the current git branch (empty string → "-").
// s is the Store used to query Redis; it must be non-nil.
// If Redis is unreachable or the context is already done, a minimal fallback line is returned.
func Render(ctx context.Context, s store.Store, branch string) string {
	if branch == "" {
		branch = "-"
	}

	branchPart := "🌳 " + branch

	// Use a short-lived context to enforce the Redis budget.
	// If the caller's context is already done, we still want to return the fallback fast.
	redisCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	// Query session count.
	sessions, sessErr := s.ListActiveSessions(redisCtx)
	if sessErr != nil || ctx.Err() != nil {
		// Daemon down fallback.
		return branchPart + sep + "⚪ daemon down"
	}

	sessionCount := len(sessions)

	// Query recent activity (global ring, all entries).
	activities, actErr := s.RecentActivity(redisCtx, 200, "")
	if actErr != nil {
		// Still show partial data: session count but no events.
		activities = nil
	}

	// Count events within 5m window.
	nowMs := float64(time.Now().UnixMilli())
	cutoffMs := nowMs - windowMs
	eventCount := 0
	for _, a := range activities {
		if a.Ts >= cutoffMs {
			eventCount++
		}
	}

	return strings.Join([]string{
		branchPart,
		sessionPart(sessionCount),
		fmt.Sprintf("📡 %d eventos/5m", eventCount),
		"✅ daemon",
	}, sep)
}

// sessionPart returns the sessions component with correct Spanish plural.
func sessionPart(n int) string {
	if n == 1 {
		return "🔵 1 sesión"
	}
	return fmt.Sprintf("🔵 %d sesiones", n)
}
