// Package statusline renders a single-line status string for Claude Code's statusLine config.
//
// Format (with context info):
//
//	🌳 <branch>[ (<changes>)] │ 🧠 <icon> <pct>% (<used>/<limit>) │ 🔵 <N> sesión(es) │ 📡 <N> eventos/5m │ ✅ daemon
//
// If context Usage.Tokens == 0, the 🧠 block is omitted entirely.
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

	"claude-mesh/internal/contextusage"
	"claude-mesh/internal/store"
)

const (
	sep      = " │ "
	windowMs = 5 * 60 * 1000 // 5 minutes in milliseconds
)

// Input holds the JSON payload that Claude Code passes to the statusline command via stdin.
// Fields not listed here are ignored.
type Input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	// Changes is the count of dirty files from `git status --porcelain`.
	// Populated by the caller (main.go) before invoking Render.
	Changes int `json:"-"`
}

// Render returns the formatted statusline string.
//
//   - branch is the current git branch (empty string → "-").
//   - s is the Store used to query Redis; it must be non-nil.
//   - in is the parsed stdin Input from Claude Code.
//   - u is the already-parsed context Usage (caller resolves this).
//
// If Redis is unreachable or the context is already done, a minimal fallback line is returned.
func Render(ctx context.Context, s store.Store, branch string, in Input, u contextusage.Usage) string {
	if branch == "" {
		branch = "-"
	}

	// Branch part: "🌳 develop" or "🌳 develop (5)"
	branchPart := "🌳 " + branch
	if in.Changes > 0 {
		branchPart += fmt.Sprintf(" (%d)", in.Changes)
	}

	// Use a short-lived context to enforce the Redis budget.
	// If the caller's context is already done, we still want to return the fallback fast.
	redisCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	// Query session count.
	sessions, sessErr := s.ListActiveSessions(redisCtx)
	if sessErr != nil || ctx.Err() != nil {
		// Daemon down fallback — still include context if available.
		parts := []string{branchPart}
		if u.Tokens > 0 {
			parts = append(parts, contextPart(u))
		}
		parts = append(parts, "⚪ daemon down")
		return strings.Join(parts, sep)
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

	// Build parts list, inserting context block after branch if available.
	parts := []string{branchPart}
	if u.Tokens > 0 {
		parts = append(parts, contextPart(u))
	}
	parts = append(parts,
		sessionPart(sessionCount),
		fmt.Sprintf("📡 %d eventos/5m", eventCount),
		"✅ daemon",
	)

	return strings.Join(parts, sep)
}

// contextPart formats the 🧠 context usage block.
// Format: "🧠 🟢 45% (90k/200k)"
func contextPart(u contextusage.Usage) string {
	icon := contextIcon(u.Percent)
	usedK := formatK(u.Tokens)
	limitK := formatK(u.Limit)
	pct := int(u.Percent + 0.5) // round to nearest integer
	return fmt.Sprintf("🧠 %s %d%% (%s/%s)", icon, pct, usedK, limitK)
}

// contextIcon returns the visual icon for the given usage percentage.
// Thresholds: <60% green, 60-80% yellow, 80-95% red, >=95% critical.
func contextIcon(pct float64) string {
	switch {
	case pct >= 95.0:
		return "🚨"
	case pct >= 80.0:
		return "🔴"
	case pct >= 60.0:
		return "🟡"
	default:
		return "🟢"
	}
}

// formatK formats a token count as "Nk" (nearest integer k).
// 94312 → "94k", 123456 → "123k", 1234 → "1k".
func formatK(n int) string {
	k := (n + 500) / 1000
	return fmt.Sprintf("%dk", k)
}

// sessionPart returns the sessions component with correct Spanish plural.
func sessionPart(n int) string {
	if n == 1 {
		return "🔵 1 sesión"
	}
	return fmt.Sprintf("🔵 %d sesiones", n)
}
