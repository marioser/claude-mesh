package statusline

import "strings"

// Model represents the Claude model passed by Claude Code via the statusline stdin payload.
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

const (
	// contextLimit1M is the context window for 1M-variant models (Opus 4.7 [1m] etc.).
	contextLimit1M = 1_000_000

	// contextLimitDefault is the standard context window for all other models.
	contextLimitDefault = 200_000
)

// ContextLimitForModel returns the context window size in tokens for the given model.
//
// Detection heuristic (evaluated in order):
//  1. If model.id contains "[1m]" or "[1M]" → 1_000_000
//  2. If model.display_name contains "1M" or "1m" (case-insensitive substring) → 1_000_000
//  3. Default → 200_000
//
// This is intentionally heuristic-based so that future models with the [1m] marker
// are handled correctly without requiring a code change.
func ContextLimitForModel(m Model) int {
	idLower := strings.ToLower(m.ID)
	if strings.Contains(idLower, "[1m]") {
		return contextLimit1M
	}

	nameLower := strings.ToLower(m.DisplayName)
	if strings.Contains(nameLower, "1m") {
		return contextLimit1M
	}

	return contextLimitDefault
}
