package contextusage_test

import (
	"path/filepath"
	"testing"

	"claude-mesh/internal/contextusage"
)

func fixturesDir() string {
	return "testdata"
}

// TestParseAssistantUsage verifies token extraction from assistant usage block.
// input_tokens=50000, cache_read=80000, cache_creation=20000 → total=150000
// 150000/200000 = 75.0%
func TestParseAssistantUsage(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_usage.jsonl"))

	if u.Tokens != 150000 {
		t.Errorf("Tokens: want 150000, got %d", u.Tokens)
	}
	if u.Limit != 200000 {
		t.Errorf("Limit: want 200000, got %d", u.Limit)
	}
	if u.Method != "usage" {
		t.Errorf("Method: want 'usage', got %q", u.Method)
	}
	if u.Source != "transcript" {
		t.Errorf("Source: want 'transcript', got %q", u.Source)
	}
	wantPct := 75.0
	if u.Percent < wantPct-0.01 || u.Percent > wantPct+0.01 {
		t.Errorf("Percent: want %.1f, got %.4f", wantPct, u.Percent)
	}
}

// TestParseSystemAutoCompact verifies "Context left until auto-compact: X%" parsing.
// 23% left → 77% used.
func TestParseSystemAutoCompact(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_system_autocompact.jsonl"))

	if u.Method != "system" {
		t.Errorf("Method: want 'system', got %q", u.Method)
	}
	wantPct := 77.0
	if u.Percent < wantPct-0.01 || u.Percent > wantPct+0.01 {
		t.Errorf("Percent: want %.1f, got %.4f", wantPct, u.Percent)
	}
	if u.Source != "transcript" {
		t.Errorf("Source: want 'transcript', got %q", u.Source)
	}
}

// TestParseSystemContextLow verifies "Context low (X% remaining)" parsing.
// 15% remaining → 85% used.
func TestParseSystemContextLow(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_system_contextlow.jsonl"))

	if u.Method != "system" {
		t.Errorf("Method: want 'system', got %q", u.Method)
	}
	wantPct := 85.0
	if u.Percent < wantPct-0.01 || u.Percent > wantPct+0.01 {
		t.Errorf("Percent: want %.1f, got %.4f", wantPct, u.Percent)
	}
}

// TestParseFileNotExist verifies graceful degrade when file is missing.
func TestParseFileNotExist(t *testing.T) {
	u := contextusage.Parse("/nonexistent/path/transcript.jsonl")

	if u.Tokens != 0 {
		t.Errorf("Tokens: want 0 for missing file, got %d", u.Tokens)
	}
	if u.Method != "" {
		t.Errorf("Method: want '' for missing file, got %q", u.Method)
	}
	if u.Percent != 0 {
		t.Errorf("Percent: want 0 for missing file, got %f", u.Percent)
	}
}

// TestParseEmptyFile verifies graceful degrade on empty transcript.
func TestParseEmptyFile(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "empty.jsonl"))

	if u.Tokens != 0 {
		t.Errorf("Tokens: want 0 for empty file, got %d", u.Tokens)
	}
	if u.Method != "" {
		t.Errorf("Method: want '' for empty file, got %q", u.Method)
	}
}

// TestParseToolNoise verifies zero Usage when last 15 lines are all tool_use/tool_result.
func TestParseToolNoise(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "tool_noise.jsonl"))

	if u.Tokens != 0 {
		t.Errorf("Tokens: want 0 for tool-noise-only tail, got %d", u.Tokens)
	}
	if u.Method != "" {
		t.Errorf("Method: want '' for tool-noise-only tail, got %q", u.Method)
	}
}

// TestParseMostRecentAssistant verifies that the LAST assistant message wins
// when multiple exist. Last: input=30000, cache_read=50000, cache_creation=10000 → 90000.
func TestParseMostRecentAssistant(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "multiple_assistant.jsonl"))

	if u.Tokens != 90000 {
		t.Errorf("Tokens: want 90000 (most recent), got %d", u.Tokens)
	}
	wantPct := 45.0 // 90000/200000
	if u.Percent < wantPct-0.01 || u.Percent > wantPct+0.01 {
		t.Errorf("Percent: want %.1f, got %.4f", wantPct, u.Percent)
	}
}
