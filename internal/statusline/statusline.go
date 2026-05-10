// Package statusline renders a single-line status string for Claude Code's statusLine config.
//
// Format (with context and usage info):
//
//	🌳 <branch>[ (<changes>)] │ 🤖 <model> │ 🧠 <icon> <pct>% (<used>/<limit>) │ 📊 <icon> Sesión X% Semana Y% │ 🔵 <N> sesión(es) │ 📡 <N> eventos/5m │ ✅ daemon
//
// If context Usage.Tokens == 0, the 🧠 block is omitted entirely.
// If plan usage data is not yet available (pct5h == 0 && pctWeek == 0), the 📊 block is omitted.
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

	// Redis keys written by the usage poll goroutine in the daemon.
	usageKeyPct5h   = "claude:mesh:usage:pct:5h"
	usageKeyPctWeek = "claude:mesh:usage:pct:week"
	usageKeyTokens5h = "claude:mesh:usage:tokens:5h"
	usageKeyPlan    = "claude:mesh:usage:plan"
)

// PlanUsage holds the usage percentages read from Redis (written by the daemon poll loop).
// Zero values indicate that the daemon hasn't computed stats yet.
type PlanUsage struct {
	Pct5h    float64 // 0..100 — percentage of 5h rolling block consumed
	PctWeek  float64 // 0..100 — percentage of 7-day rolling window consumed
	Tokens5h int     // raw tokens in the 5h window
	Plan     string  // plan tier, e.g. "max20"
}

// ReadUsage reads plan usage data from Redis with a 30ms timeout.
// Returns zero PlanUsage if any key is missing or Redis is unavailable.
func ReadUsage(ctx context.Context, s store.Store) PlanUsage {
	rctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()

	pct5h, err := s.GetFloat(rctx, usageKeyPct5h)
	if err != nil {
		return PlanUsage{}
	}
	pctWeek, err := s.GetFloat(rctx, usageKeyPctWeek)
	if err != nil {
		return PlanUsage{}
	}

	// Best-effort fields — ignore errors.
	tokens5h, _ := s.GetInt(rctx, usageKeyTokens5h)
	plan, _ := s.GetString(rctx, usageKeyPlan)

	return PlanUsage{
		Pct5h:    pct5h,
		PctWeek:  pctWeek,
		Tokens5h: tokens5h,
		Plan:     plan,
	}
}

// Input holds the JSON payload that Claude Code passes to the statusline command via stdin.
// Fields not listed here are ignored.
type Input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	// Model is the active Claude model passed by Claude Code in the stdin payload.
	// Used to derive the correct context window limit via ContextLimitForModel.
	Model Model `json:"model"`
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

	// Read plan usage (best-effort, 30ms budget inside ReadUsage).
	pu := ReadUsage(ctx, s)

	// Query session count.
	sessions, sessErr := s.ListActiveSessions(redisCtx)
	if sessErr != nil || ctx.Err() != nil {
		// Daemon down fallback — still include model and context if available.
		parts := []string{branchPart}
		if mp := modelPart(in.Model); mp != "" {
			parts = append(parts, mp)
		}
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

	// Build parts list: branch → model → context → usage → sessions → events → daemon.
	parts := []string{branchPart}
	if mp := modelPart(in.Model); mp != "" {
		parts = append(parts, mp)
	}
	if u.Tokens > 0 {
		parts = append(parts, contextPart(u))
	}
	if up := usagePart(pu); up != "" {
		parts = append(parts, up)
	}
	parts = append(parts,
		sessionPart(sessionCount),
		fmt.Sprintf("📡 %d eventos/5m", eventCount),
		"✅ daemon",
	)

	return strings.Join(parts, sep)
}

// usagePart formats the 📊 plan usage block.
// Format: "📊 🟢 Sesión X% Semana Y%"
// Returns empty string if both percentages are zero (data not yet computed by daemon).
func usagePart(pu PlanUsage) string {
	if pu.Pct5h == 0 && pu.PctWeek == 0 {
		return ""
	}
	// Icon uses the worst of the two percentages.
	maxPct := pu.Pct5h
	if pu.PctWeek > maxPct {
		maxPct = pu.PctWeek
	}
	icon := contextIcon(maxPct)
	pct5h := int(pu.Pct5h + 0.5)
	pctWeek := int(pu.PctWeek + 0.5)
	return fmt.Sprintf("📊 %s Sesión %d%% Semana %d%%", icon, pct5h, pctWeek)
}

// contextPart formats the 🧠 context usage block.
// Format: "🧠 🟢 45% (90k/200k)" or "🧠 🟢 42% (426k/1M)"
func contextPart(u contextusage.Usage) string {
	icon := contextIcon(u.Percent)
	usedK := formatK(u.Tokens)
	limitStr := formatLimit(u.Limit)
	pct := int(u.Percent + 0.5) // round to nearest integer
	return fmt.Sprintf("🧠 %s %d%% (%s/%s)", icon, pct, usedK, limitStr)
}

// formatLimit formats a context window limit for display.
// 1_000_000 → "1M"; all others → formatK (e.g. "200k").
func formatLimit(limit int) string {
	if limit == 1_000_000 {
		return "1M"
	}
	return formatK(limit)
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

// modelPart formats the 🤖 model block. Returns empty string if no model info is available,
// which signals the caller to omit this block entirely.
func modelPart(m Model) string {
	short := ShortModelName(m)
	if short == "" {
		return ""
	}
	return "🤖 " + short
}
