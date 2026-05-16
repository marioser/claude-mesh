package installer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marioser/claude-mesh/internal/installer"
)

// TestStatusLineFirstInstall verifies that PatchStatusLine adds the statusLine entry.
func TestStatusLineFirstInstall(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	copyFile(t, "testdata/settings.before.json", dst)

	bridgeBin := "/home/user/.local/bin/claude-mesh-bridge"
	if err := installer.PatchStatusLine(dst, bridgeBin); err != nil {
		t.Fatalf("PatchStatusLine: %v", err)
	}

	settings := readSettings(t, dst)
	raw, ok := settings["statusLine"]
	if !ok {
		t.Fatal("statusLine key not present after PatchStatusLine")
	}

	var sl map[string]string
	if err := json.Unmarshal(raw, &sl); err != nil {
		t.Fatalf("unmarshal statusLine: %v", err)
	}

	if sl["type"] != "command" {
		t.Errorf("statusLine.type = %q, want 'command'", sl["type"])
	}
	wantCmd := bridgeBin + " statusline"
	if sl["command"] != wantCmd {
		t.Errorf("statusLine.command = %q, want %q", sl["command"], wantCmd)
	}
}

// TestStatusLineIdempotent verifies that running PatchStatusLine twice produces identical output.
func TestStatusLineIdempotent(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	copyFile(t, "testdata/settings.before.json", dst)

	bridgeBin := "/home/user/.local/bin/claude-mesh-bridge"
	for i := 0; i < 2; i++ {
		if err := installer.PatchStatusLine(dst, bridgeBin); err != nil {
			t.Fatalf("PatchStatusLine run %d: %v", i+1, err)
		}
	}

	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// statusLine key should appear exactly once.
	count := strings.Count(string(raw), `"statusLine"`)
	if count != 1 {
		t.Errorf("statusLine appears %d times after idempotent install, want 1\n%s", count, raw)
	}
}

// TestStatusLineRemove verifies that RemoveStatusLine removes the entry only if it points to claude-mesh.
func TestStatusLineRemove(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	copyFile(t, "testdata/settings.before.json", dst)

	bridgeBin := "/home/user/.local/bin/claude-mesh-bridge"
	if err := installer.PatchStatusLine(dst, bridgeBin); err != nil {
		t.Fatalf("PatchStatusLine (setup): %v", err)
	}

	// Verify it's there before removing.
	before := readSettings(t, dst)
	if _, ok := before["statusLine"]; !ok {
		t.Fatal("statusLine not found before RemoveStatusLine")
	}

	if err := installer.RemoveStatusLine(dst, bridgeBin); err != nil {
		t.Fatalf("RemoveStatusLine: %v", err)
	}

	after := readSettings(t, dst)
	if _, ok := after["statusLine"]; ok {
		t.Error("statusLine still present after RemoveStatusLine")
	}

	// Existing hooks must remain untouched.
	if !strings.Contains(string(after["hooks"]), "neurostack-session-context") {
		t.Error("NeuroStack hook was removed during statusLine removal")
	}
}

// TestStatusLineRemoveDoesNotRemoveOtherStatusLine verifies that RemoveStatusLine
// does NOT remove a statusLine entry pointing to a different command.
func TestStatusLineRemoveDoesNotRemoveOtherStatusLine(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")

	// Write a settings.json with a statusLine from a different tool.
	initial := `{
  "statusLine": {"type": "command", "command": "/usr/local/bin/other-tool statusline"},
  "hooks": {}
}`
	if err := os.WriteFile(dst, []byte(initial), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	bridgeBin := "/home/user/.local/bin/claude-mesh-bridge"
	if err := installer.RemoveStatusLine(dst, bridgeBin); err != nil {
		t.Fatalf("RemoveStatusLine: %v", err)
	}

	settings := readSettings(t, dst)
	if _, ok := settings["statusLine"]; !ok {
		t.Error("RemoveStatusLine removed a statusLine entry not belonging to claude-mesh")
	}
}

// TestStatusLineMalformedError verifies that PatchStatusLine returns error on malformed JSON.
func TestStatusLineMalformedError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(dst, []byte("NOT JSON {{{{"), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	err := installer.PatchStatusLine(dst, "/bin/bridge")
	if err == nil {
		t.Error("PatchStatusLine with malformed JSON: expected error, got nil")
	}
}
