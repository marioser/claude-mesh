package usagestats_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marioser/claude-mesh/internal/usagestats"
)

const testProjectsDir = "testdata/projects"

// setupProjects creates a temporary projects-dir mimicking ~/.claude/projects/
// with subdirs each containing a *.jsonl file. Returns the tempdir path.
func setupTestProjects(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "projects")

	// Project A — two messages: 2026-05-01T10:00Z and 10:30Z
	projA := filepath.Join(dir, "-Users-test-project-a")
	if err := os.MkdirAll(projA, 0755); err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(filepath.Join("testdata", "session_a.jsonl"))
	if err := os.WriteFile(filepath.Join(projA, "abc123.jsonl"), src, 0644); err != nil {
		t.Fatal(err)
	}

	// Project B — one message: 2026-05-08T14:01Z
	projB := filepath.Join(dir, "-Users-test-project-b")
	if err := os.MkdirAll(projB, 0755); err != nil {
		t.Fatal(err)
	}
	src2, _ := os.ReadFile(filepath.Join("testdata", "session_b.jsonl"))
	if err := os.WriteFile(filepath.Join(projB, "def456.jsonl"), src2, 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

// TestScanProjectsAllEntries verifies that ScanProjects returns all assistant usage entries
// when called with a zero since time (no cutoff).
// session_a: 2 entries (5000+3000+2000=10000, 8000+1000+4000=13000)
// session_b: 1 entry (12000+0+6000=18000)
// Total: 3 entries.
func TestScanProjectsAllEntries(t *testing.T) {
	dir := setupTestProjects(t)

	entries, err := usagestats.ScanProjects(dir, time.Time{})
	if err != nil {
		t.Fatalf("ScanProjects: unexpected error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("entry count: want 3, got %d", len(entries))
	}

	// Sum all tokens: 10000 + 13000 + 18000 = 41000
	total := 0
	for _, e := range entries {
		total += e.Tokens
	}
	if total != 41_000 {
		t.Errorf("total tokens: want 41_000, got %d", total)
	}
}

// TestScanProjectsSinceFilter verifies that entries older than `since` are excluded.
// session_a entries are on 2026-05-01 — older than 2026-05-08 cutoff.
// session_b entry is on 2026-05-08 14:01Z — newer.
func TestScanProjectsSinceFilter(t *testing.T) {
	dir := setupTestProjects(t)

	// Cutoff: 2026-05-07 00:00:00 UTC — keeps only session_b entry.
	since := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	entries, err := usagestats.ScanProjects(dir, since)
	if err != nil {
		t.Fatalf("ScanProjects: unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("entry count after cutoff: want 1, got %d (entries: %+v)", len(entries), entries)
	}

	if entries[0].Tokens != 18_000 {
		t.Errorf("filtered entry tokens: want 18_000, got %d", entries[0].Tokens)
	}
}

// TestScanProjectsNonExistentDir verifies graceful error on missing directory.
func TestScanProjectsNonExistentDir(t *testing.T) {
	_, err := usagestats.ScanProjects("/nonexistent/projects/dir", time.Time{})
	if err == nil {
		t.Error("ScanProjects: want error for nonexistent dir, got nil")
	}
}

// TestScanProjectsEmptyDir verifies zero entries for an empty projects directory.
func TestScanProjectsEmptyDir(t *testing.T) {
	dir := t.TempDir()

	entries, err := usagestats.ScanProjects(dir, time.Time{})
	if err != nil {
		t.Fatalf("ScanProjects: unexpected error for empty dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entry count: want 0 for empty dir, got %d", len(entries))
	}
}

// TestScanProjectsTokenCalculation verifies the token formula:
// input_tokens + cache_creation_input_tokens + cache_read_input_tokens.
// session_b: 12000 + 0 + 6000 = 18000 (output_tokens NOT included).
func TestScanProjectsTokenCalculation(t *testing.T) {
	dir := setupTestProjects(t)

	// Narrow to session_b only (after 2026-05-07).
	since := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	entries, err := usagestats.ScanProjects(dir, since)
	if err != nil {
		t.Fatalf("ScanProjects: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	// Verify output_tokens are NOT counted (formula = input + cache_read + cache_creation).
	if entries[0].Tokens != 18_000 {
		t.Errorf("token formula: want 18_000 (no output_tokens), got %d", entries[0].Tokens)
	}
}
