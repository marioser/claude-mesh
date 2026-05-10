package statusline_test

import (
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
