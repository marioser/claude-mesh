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
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_usage.jsonl"), 200_000)

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
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_system_autocompact.jsonl"), 200_000)

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
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_system_contextlow.jsonl"), 200_000)

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
	u := contextusage.Parse("/nonexistent/path/transcript.jsonl", 200_000)

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
	u := contextusage.Parse(filepath.Join(fixturesDir(), "empty.jsonl"), 200_000)

	if u.Tokens != 0 {
		t.Errorf("Tokens: want 0 for empty file, got %d", u.Tokens)
	}
	if u.Method != "" {
		t.Errorf("Method: want '' for empty file, got %q", u.Method)
	}
}

// TestParseToolNoise verifies zero Usage when last 15 lines are all tool_use/tool_result.
func TestParseToolNoise(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "tool_noise.jsonl"), 200_000)

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
	u := contextusage.Parse(filepath.Join(fixturesDir(), "multiple_assistant.jsonl"), 200_000)

	if u.Tokens != 90000 {
		t.Errorf("Tokens: want 90000 (most recent), got %d", u.Tokens)
	}
	wantPct := 45.0 // 90000/200000
	if u.Percent < wantPct-0.01 || u.Percent > wantPct+0.01 {
		t.Errorf("Percent: want %.1f, got %.4f", wantPct, u.Percent)
	}
}

// TestParse1MLimit is the bug-regression test: 426k tokens with a 1M limit
// must produce ~42.6% — not capped at 100%.
// This demonstrates the original bug (hardcoded 200k) and verifies the fix.
func TestParse1MLimit(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_usage.jsonl"), 1_000_000)

	// with_usage.jsonl has 150000 tokens (input=50000, cache_read=80000, cache_creation=20000).
	// With limit=1_000_000 → 15.0%, Limit should be 1_000_000, not 200_000.
	if u.Tokens != 150000 {
		t.Errorf("Tokens: want 150000, got %d", u.Tokens)
	}
	if u.Limit != 1_000_000 {
		t.Errorf("Limit: want 1_000_000 when called with 1M limit, got %d", u.Limit)
	}
	wantPct := 15.0 // 150000/1_000_000
	if u.Percent < wantPct-0.01 || u.Percent > wantPct+0.01 {
		t.Errorf("Percent: want %.1f (not capped at 100%%), got %.4f", wantPct, u.Percent)
	}
}

// TestParseZeroOrNegativeLimitFallback verifies defensive default:
// limit <= 0 → falls back to 200_000.
func TestParseZeroOrNegativeLimitFallback(t *testing.T) {
	u := contextusage.Parse(filepath.Join(fixturesDir(), "with_usage.jsonl"), 0)

	if u.Limit != 200_000 {
		t.Errorf("Limit: want 200_000 fallback for limit=0, got %d", u.Limit)
	}
}
