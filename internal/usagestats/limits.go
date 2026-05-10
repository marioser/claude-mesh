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

// planDefaults maps tier name → (limit5h, limitWeek).
// limitWeek = limit5h × 7 × 24 / 5 (number of 5h blocks in a week).
var planDefaults = map[string][2]int{
	"pro":   {19_000, 638_400},    // 19_000 × 7 × 24 / 5
	"max5":  {88_000, 2_956_800},  // 88_000 × 7 × 24 / 5
	"max20": {220_000, 7_392_000}, // 220_000 × 7 × 24 / 5
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
