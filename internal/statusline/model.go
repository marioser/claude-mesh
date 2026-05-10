package statusline

import (
	"strings"
	"unicode"
)

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

// ShortModelName returns a compact human-readable model name for the statusline 🤖 block.
//
// Priority:
//  1. If both DisplayName and ID are empty → returns "" (block will be omitted by caller).
//  2. DisplayName present → strip "Claude " prefix (case-insensitive), compact "(1M context)" → "(1M)".
//     If id contains [1m] and display_name has no 1M marker, append " (1M)".
//  3. Fallback to ID → strip "claude-" prefix, strip "[1m]"/[1M]" suffix (note the presence),
//     replace "-" with spaces and title-case each word, then append " (1M)" if suffix was present.
func ShortModelName(m Model) string {
	if m.DisplayName == "" && m.ID == "" {
		return ""
	}

	if m.DisplayName != "" {
		name := m.DisplayName

		// Strip "Claude " prefix (case-insensitive).
		if strings.HasPrefix(strings.ToLower(name), "claude ") {
			name = name[len("claude "):]
		}

		// Compact "(1M context)" → "(1M)".
		name = strings.ReplaceAll(name, "(1M context)", "(1M)")
		name = strings.ReplaceAll(name, "(1m context)", "(1M)")

		// If the ID has [1m] but the display name has no (1M), append it.
		idLower := strings.ToLower(m.ID)
		nameLower := strings.ToLower(name)
		if strings.Contains(idLower, "[1m]") && !strings.Contains(nameLower, "1m") {
			name += " (1M)"
		}

		return strings.TrimSpace(name)
	}

	// Fallback: derive from ID.
	id := m.ID
	has1M := false

	// Strip "[1m]" or "[1M]" from the end (anywhere in id, but typically suffix).
	idLower := strings.ToLower(id)
	if idx := strings.LastIndex(idLower, "[1m]"); idx != -1 {
		has1M = true
		id = id[:idx] + id[idx+4:]
	}

	// Strip "claude-" prefix.
	if strings.HasPrefix(strings.ToLower(id), "claude-") {
		id = id[len("claude-"):]
	}

	// Trim any trailing dashes left over.
	id = strings.Trim(id, "-")

	// Replace dashes with spaces and title-case each word.
	words := strings.Split(id, "-")
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	result := strings.Join(words, " ")

	if has1M {
		result += " (1M)"
	}

	return strings.TrimSpace(result)
}

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
