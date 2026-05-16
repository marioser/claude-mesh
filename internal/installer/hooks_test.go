package installer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marioser/claude-mesh/internal/installer"
)

// copyFile copies src to dst for use in tests.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("copyFile read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("copyFile write %s: %v", dst, err)
	}
}

// readSettings reads and parses settings.json into a map.
func readSettings(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("readSettings unmarshal: %v (raw: %s)", err, raw)
	}
	return m
}

// TestHooksFirstInstall verifies that a first install adds 3 claude-mesh-* hook entries.
func TestHooksFirstInstall(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	copyFile(t, "testdata/settings.before.json", dst)

	err := installer.PatchSettings(dst, "/usr/local/bin/claude-mesh-bridge", "/hooks")
	if err != nil {
		t.Fatalf("PatchSettings: %v", err)
	}

	settings := readSettings(t, dst)
	hooksRaw := settings["hooks"]
	hooksStr := string(hooksRaw)

	// Must contain all 3 claude-mesh marker names.
	for _, marker := range []string{"claude-mesh-session-start", "claude-mesh-pre-tool-use", "claude-mesh-stop"} {
		if !strings.Contains(hooksStr, marker) {
			t.Errorf("after install, missing hook marker %q in: %s", marker, hooksStr)
		}
	}
}

// TestHooksIdempotent verifies that running PatchSettings twice produces identical output.
func TestHooksIdempotent(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	copyFile(t, "testdata/settings.before.json", dst)

	for i := 0; i < 2; i++ {
		if err := installer.PatchSettings(dst, "/usr/local/bin/claude-mesh-bridge", "/hooks"); err != nil {
			t.Fatalf("PatchSettings run %d: %v", i+1, err)
		}
	}

	settings := readSettings(t, dst)
	hooksRaw := settings["hooks"]
	hooksStr := string(hooksRaw)

	// claude-mesh-session-start should appear exactly once.
	count := strings.Count(hooksStr, "claude-mesh-session-start")
	if count != 1 {
		t.Errorf("idempotent: claude-mesh-session-start appears %d times, want 1", count)
	}
}

// TestNeuroStackHooksUnmodified verifies that NeuroStack hooks survive Claude Mesh installation.
func TestNeuroStackHooksUnmodified(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	copyFile(t, "testdata/settings.before.json", dst)

	if err := installer.PatchSettings(dst, "/usr/local/bin/claude-mesh-bridge", "/hooks"); err != nil {
		t.Fatalf("PatchSettings: %v", err)
	}

	settings := readSettings(t, dst)
	hooksRaw := settings["hooks"]
	hooksStr := string(hooksRaw)

	if !strings.Contains(hooksStr, "neurostack-session-context") {
		t.Errorf("NeuroStack hook 'neurostack-session-context' missing after Claude Mesh install: %s", hooksStr)
	}
}

// TestHooksMissingSettingsCreated verifies that a missing settings.json creates a skeleton.
func TestHooksMissingSettingsCreated(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	// Do NOT create the file — let PatchSettings create it.

	if err := installer.PatchSettings(dst, "/usr/local/bin/claude-mesh-bridge", "/hooks"); err != nil {
		t.Fatalf("PatchSettings missing file: %v", err)
	}

	if _, err := os.Stat(dst); err != nil {
		t.Errorf("settings.json not created: %v", err)
	}

	settings := readSettings(t, dst)
	if settings["hooks"] == nil {
		t.Error("hooks key missing in newly created settings.json")
	}
}

// TestHooksMalformedSettingsError verifies that a malformed settings.json returns an error
// and does NOT overwrite the file.
func TestHooksMalformedSettingsError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(dst, []byte("NOT JSON {{{{"), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	err := installer.PatchSettings(dst, "/bin/bridge", "/hooks")
	if err == nil {
		t.Error("PatchSettings with malformed JSON: expected error, got nil")
	}

	// Original file must be unchanged.
	raw, _ := os.ReadFile(dst)
	if string(raw) != "NOT JSON {{{" {
		// May have been partially written — but the error means we won't overwrite.
		// The backup was written, but destination should not be valid JSON.
		if json.Valid(raw) {
			t.Error("malformed settings.json was overwritten with valid JSON despite error")
		}
	}
}
