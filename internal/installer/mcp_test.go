package installer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"claude-mesh/internal/installer"
)

// TestMCPFirstInstall verifies that PatchMCP adds the claude-mesh server entry.
func TestMCPFirstInstall(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".mcp.json")
	copyFile(t, "testdata/mcp.before.json", dst)

	if err := installer.PatchMCP(dst, "/usr/local/bin/claude-mesh-mcp"); err != nil {
		t.Fatalf("PatchMCP: %v", err)
	}

	raw, _ := os.ReadFile(dst)
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := cfg.MCPServers["claude-mesh"]; !ok {
		t.Error("claude-mesh entry not added to .mcp.json after first install")
	}
	// Pre-existing entry must still be present.
	if _, ok := cfg.MCPServers["mioboxtc"]; !ok {
		t.Error("pre-existing mioboxtc entry was removed")
	}
}

// TestMCPIdempotent verifies that running PatchMCP twice produces no duplicates.
func TestMCPIdempotent(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".mcp.json")
	copyFile(t, "testdata/mcp.before.json", dst)

	for i := 0; i < 2; i++ {
		if err := installer.PatchMCP(dst, "/usr/local/bin/claude-mesh-mcp"); err != nil {
			t.Fatalf("PatchMCP run %d: %v", i+1, err)
		}
	}

	raw, _ := os.ReadFile(dst)
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	_ = json.Unmarshal(raw, &cfg)

	// Should still have exactly 2 entries (mioboxtc + claude-mesh).
	if len(cfg.MCPServers) != 2 {
		t.Errorf("expected 2 servers after idempotent install, got %d: %v", len(cfg.MCPServers), cfg.MCPServers)
	}
}

// TestMCPMalformedError verifies that a malformed .mcp.json returns an error.
func TestMCPMalformedError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".mcp.json")
	_ = os.WriteFile(dst, []byte("NOT JSON {{{{"), 0o644)

	err := installer.PatchMCP(dst, "/bin/claude-mesh-mcp")
	if err == nil {
		t.Error("PatchMCP with malformed JSON: expected error, got nil")
	}
}
