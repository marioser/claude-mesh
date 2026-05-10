package statusline_test

import (
	"strings"
	"testing"

	"claude-mesh/internal/statusline"
)

// TestContextLimitForModel verifies the context window detection heuristic.
// Rules:
//   - model.id contains "[1m]" or "[1M]" → 1_000_000
//   - model.display_name contains "1M" or "1m" (case-insensitive) → 1_000_000
//   - empty model or no marker → 200_000 (default)

func TestContextLimitOpus1MByID(t *testing.T) {
	m := statusline.Model{ID: "claude-opus-4-7[1m]", DisplayName: "Opus 4.7"}
	if got := statusline.ContextLimitForModel(m); got != 1_000_000 {
		t.Errorf("Opus [1m] id: want 1_000_000, got %d", got)
	}
}

func TestContextLimitOpus1MByIDUpperCase(t *testing.T) {
	m := statusline.Model{ID: "claude-opus-4-7[1M]", DisplayName: "Opus 4.7"}
	if got := statusline.ContextLimitForModel(m); got != 1_000_000 {
		t.Errorf("Opus [1M] id: want 1_000_000, got %d", got)
	}
}

func TestContextLimitOpusDefaultByID(t *testing.T) {
	m := statusline.Model{ID: "claude-opus-4-7", DisplayName: "Opus 4.7"}
	if got := statusline.ContextLimitForModel(m); got != 200_000 {
		t.Errorf("Opus default id: want 200_000, got %d", got)
	}
}

func TestContextLimitSonnetByID(t *testing.T) {
	m := statusline.Model{ID: "claude-sonnet-4-6", DisplayName: "Sonnet 4.6"}
	if got := statusline.ContextLimitForModel(m); got != 200_000 {
		t.Errorf("Sonnet id: want 200_000, got %d", got)
	}
}

func TestContextLimitByDisplayName1M(t *testing.T) {
	// ID has no [1m] marker but display_name says 1M context.
	m := statusline.Model{ID: "claude-opus-4-7", DisplayName: "Opus 4.7 (1M context)"}
	if got := statusline.ContextLimitForModel(m); got != 1_000_000 {
		t.Errorf("display_name 1M: want 1_000_000, got %d", got)
	}
}

func TestContextLimitByDisplayNameLower1m(t *testing.T) {
	m := statusline.Model{ID: "claude-opus-4-7", DisplayName: "Opus 4.7 (1m context)"}
	if got := statusline.ContextLimitForModel(m); got != 1_000_000 {
		t.Errorf("display_name 1m lower: want 1_000_000, got %d", got)
	}
}

func TestContextLimitByDisplayNameSonnet(t *testing.T) {
	m := statusline.Model{ID: "claude-sonnet-4-6", DisplayName: "Sonnet 4.6"}
	if got := statusline.ContextLimitForModel(m); got != 200_000 {
		t.Errorf("Sonnet display_name: want 200_000, got %d", got)
	}
}

func TestContextLimitEmptyModel(t *testing.T) {
	m := statusline.Model{}
	if got := statusline.ContextLimitForModel(m); got != 200_000 {
		t.Errorf("empty model: want 200_000, got %d", got)
	}
}

func TestContextLimitFutureModel1M(t *testing.T) {
	// Heuristic: any unknown model with [1m] marker → 1M.
	m := statusline.Model{ID: "future-model[1m]", DisplayName: "Future Model"}
	if got := statusline.ContextLimitForModel(m); got != 1_000_000 {
		t.Errorf("future-model[1m]: want 1_000_000, got %d", got)
	}
}

// --- ShortModelName tests ---
// TestShortModelNameOpus1MContext verifies "(1M context)" in display_name is compacted to "(1M)".
func TestShortModelNameOpus1MContext(t *testing.T) {
	m := statusline.Model{DisplayName: "Opus 4.7 (1M context)", ID: "claude-opus-4-7[1m]"}
	if got := statusline.ShortModelName(m); got != "Opus 4.7 (1M)" {
		t.Errorf("ShortModelName Opus 1M context: want %q, got %q", "Opus 4.7 (1M)", got)
	}
}

// TestShortModelNameOpusPlain verifies a plain display_name (no 1M) passes through cleanly.
func TestShortModelNameOpusPlain(t *testing.T) {
	m := statusline.Model{DisplayName: "Opus 4.7", ID: "claude-opus-4-7"}
	if got := statusline.ShortModelName(m); got != "Opus 4.7" {
		t.Errorf("ShortModelName Opus plain: want %q, got %q", "Opus 4.7", got)
	}
}

// TestShortModelNameClaudePrefixStripped verifies "Claude " prefix is stripped from display_name.
func TestShortModelNameClaudePrefixStripped(t *testing.T) {
	m := statusline.Model{DisplayName: "Claude Sonnet 4.6", ID: "claude-sonnet-4-6"}
	if got := statusline.ShortModelName(m); got != "Sonnet 4.6" {
		t.Errorf("ShortModelName Claude prefix: want %q, got %q", "Sonnet 4.6", got)
	}
}

// TestShortModelNameFallbackToID verifies fallback to ID when display_name is empty.
// claude-opus-4-7[1m] → strip prefix, strip [1m], title-case, append "(1M)".
func TestShortModelNameFallbackToID(t *testing.T) {
	m := statusline.Model{DisplayName: "", ID: "claude-opus-4-7[1m]"}
	got := statusline.ShortModelName(m)
	// Must contain "(1M)" and not be empty, and strip "claude-" prefix.
	if got == "" {
		t.Errorf("ShortModelName ID fallback: want non-empty, got empty")
	}
	if !strings.Contains(got, "(1M)") {
		t.Errorf("ShortModelName ID fallback 1m: want '(1M)' in %q", got)
	}
	// Must not start with "claude-"
	if strings.HasPrefix(got, "claude-") {
		t.Errorf("ShortModelName ID fallback: must not start with 'claude-', got %q", got)
	}
}

// TestShortModelNameEmpty verifies empty Model → empty string (block omitted).
func TestShortModelNameEmpty(t *testing.T) {
	m := statusline.Model{}
	if got := statusline.ShortModelName(m); got != "" {
		t.Errorf("ShortModelName empty: want %q, got %q", "", got)
	}
}

// TestShortModelNameHaikuDisplayName verifies a model with just DisplayName and no 1M markers.
func TestShortModelNameHaikuDisplayName(t *testing.T) {
	m := statusline.Model{DisplayName: "Haiku 4.5", ID: "claude-haiku-4-5-20251001"}
	if got := statusline.ShortModelName(m); got != "Haiku 4.5" {
		t.Errorf("ShortModelName Haiku: want %q, got %q", "Haiku 4.5", got)
	}
}
