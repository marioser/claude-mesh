// Package usagestats tracks Claude Code token usage by parsing local JSONL transcript files.
// It replicates the approach used by Claude-Code-Usage-Monitor (Python) natively in Go.
package usagestats

import (
	"os"
	"strconv"
	"strings"
)

// Plan represents the user's subscription tier and computed token limits.
type Plan struct {
	Tier      string // "pro" | "max5" | "max20"
	Limit5h   int    // tokens per 5-hour rolling block
	LimitWeek int    // tokens per 7-day rolling window
}

// planDefaults maps tier name → (limit5h, limitWeek) in tokens.
// Calibrated against real /usage screenshots (Session 2%, Week 6%) cross-referenced
// with ccusage block totals. The numbers from Claude-Code-Usage-Monitor (Python)
// were 4 orders of magnitude too small — turns out they referred to a different metric.
//
// These are TOKENS (input + cache_read + cache_creation per assistant message),
// matching the JSONL parser's counting model. Real Anthropic limits may differ
// slightly per account; users can override via CLAUDE_MESH_5H_LIMIT_TOKENS and
// CLAUDE_MESH_WEEK_LIMIT_TOKENS for precise calibration.
var planDefaults = map[string][2]int{
	"pro":   {37_000_000, 2_440_000_000},  // ~8% of Max20 (rough)
	"max5":  {115_000_000, 7_625_000_000}, // ~25% of Max20 (rough)
	"max20": {463_000_000, 30_500_000_000}, // calibrated 2026-05-10: /usage 22%/12% → 101.8M/3.66B → 463M/30.5B
}

// defaultTier is used when CLAUDE_MESH_PLAN_TIER is empty or unknown.
const defaultTier = "max20"

// ResolveFromEnv reads plan configuration from environment variables.
//
// Variables:
//   - CLAUDE_MESH_PLAN_TIER: "pro" | "max5" | "max20" (default "max20")
//   - CLAUDE_MESH_5H_LIMIT_TOKENS: integer override for 5h block limit
//   - CLAUDE_MESH_WEEK_LIMIT_TOKENS: integer override for weekly limit
func ResolveFromEnv() Plan {
	tier := strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_MESH_PLAN_TIER")))
	if tier == "" {
		tier = defaultTier
	}

	defaults, ok := planDefaults[tier]
	if !ok {
		// Unknown tier — fall back to max20.
		tier = defaultTier
		defaults = planDefaults[defaultTier]
	}

	limit5h := defaults[0]
	limitWeek := defaults[1]

	// Apply per-variable overrides.
	if v := strings.TrimSpace(os.Getenv("CLAUDE_MESH_5H_LIMIT_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit5h = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("CLAUDE_MESH_WEEK_LIMIT_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limitWeek = n
		}
	}

	return Plan{
		Tier:      tier,
		Limit5h:   limit5h,
		LimitWeek: limitWeek,
	}
}
