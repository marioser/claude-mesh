//go:build integration

package installer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marioser/claude-mesh/internal/installer"
)

// setupFakeHome creates a temp directory tree mimicking $HOME structure.
// Returns: homeDir, srcBinDir (simulates dist/), srcHooksDir (simulates hooks/).
func setupFakeHome(t *testing.T) (homeDir, srcBinDir, srcHooksDir string) {
	t.Helper()

	homeDir = t.TempDir()

	// Create src bin dir with fake binaries.
	srcBinDir = t.TempDir()
	for _, name := range []string{"claude-mesh-bridge", "claude-mesh-mcp"} {
		path := filepath.Join(srcBinDir, name)
		// Write a trivially executable shell script that exits 0.
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("create fake binary %s: %v", name, err)
		}
	}

	// Create src hooks dir with fake hook scripts.
	srcHooksDir = t.TempDir()
	for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
		path := filepath.Join(srcHooksDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("create fake hook %s: %v", name, err)
		}
	}

	// Seed a settings.json with a NeuroStack hook so we can verify coexistence.
	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsJSON := `{
  "hooks": {
    "SessionStart": [
      {
        "type": "command",
        "command": "bash /path/to/neurostack/session-context.sh",
        "name": "neurostack-session-context"
      }
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settingsJSON), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	// Seed a .mcp.json with an unrelated entry.
	mcpJSON := `{"mcpServers":{"mioboxtc":{"command":"/path/to/mioboxtc","args":[]}}}`
	if err := os.WriteFile(filepath.Join(homeDir, ".mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	return homeDir, srcBinDir, srcHooksDir
}

// buildTestPaths builds a Paths struct for integration tests with a fake home.
func buildTestPaths(homeDir, srcBinDir, srcHooksDir string) installer.Paths {
	stableBinDir := filepath.Join(homeDir, ".local", "bin")
	stableHooksDir := filepath.Join(homeDir, ".claude", "hooks", "claude-mesh")

	return installer.Paths{
		SettingsJSON:   filepath.Join(homeDir, ".claude", "settings.json"),
		MCPJson:        filepath.Join(homeDir, ".mcp.json"),
		PlistDst:       filepath.Join(homeDir, "Library", "LaunchAgents", "io.github.marioser.claude-mesh-bridge.plist"),
		SrcBridgeBin:   filepath.Join(srcBinDir, "claude-mesh-bridge"),
		SrcMCPBin:      filepath.Join(srcBinDir, "claude-mesh-mcp"),
		SrcHooksDir:    srcHooksDir,
		StableBinDir:   stableBinDir,
		StableHooksDir: stableHooksDir,
		// Derived stable paths (computed by installer).
		BridgeBin: filepath.Join(stableBinDir, "claude-mesh-bridge"),
		MCPBin:    filepath.Join(stableBinDir, "claude-mesh-mcp"),
		HooksDir:  stableHooksDir,
	}
}

// TestIntegrationInstall runs the full install flow against a fake home.
func TestIntegrationInstall(t *testing.T) {
	homeDir, srcBinDir, srcHooksDir := setupFakeHome(t)
	p := buildTestPaths(homeDir, srcBinDir, srcHooksDir)

	if err := installer.Install(p); err != nil {
		t.Fatalf("Install: %v", err)
	}

	t.Run("binaries_copied_to_stable_dir", func(t *testing.T) {
		for _, name := range []string{"claude-mesh-bridge", "claude-mesh-mcp"} {
			path := filepath.Join(p.StableBinDir, name)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("%s not found at stable path %s: %v", name, path, err)
			}
			// Must be executable (owner execute bit).
			if info.Mode()&0o100 == 0 {
				t.Errorf("%s at %s is not executable: mode=%s", name, path, info.Mode())
			}
		}
	})

	t.Run("hook_scripts_copied_to_stable_dir", func(t *testing.T) {
		for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
			path := filepath.Join(p.StableHooksDir, name)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("hook %s not found at stable path %s: %v", name, path, err)
			}
			if info.Mode()&0o100 == 0 {
				t.Errorf("hook %s at %s is not executable: mode=%s", name, path, info.Mode())
			}
		}
	})

	t.Run("settings_json_has_stable_hook_paths", func(t *testing.T) {
		raw, err := os.ReadFile(p.SettingsJSON)
		if err != nil {
			t.Fatalf("read settings.json: %v", err)
		}
		content := string(raw)
		// All hook commands must reference the stable hooks dir.
		for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
			stablePath := filepath.Join(p.StableHooksDir, name)
			if !strings.Contains(content, stablePath) {
				t.Errorf("settings.json missing stable hook path %q\ncontent: %s", stablePath, content)
			}
		}
		// Must NOT reference the workspace/srcHooksDir path.
		if strings.Contains(content, srcHooksDir) {
			t.Errorf("settings.json still references workspace hooks dir %q — must use stable path", srcHooksDir)
		}
	})

	t.Run("settings_json_hook_paths_exist_on_disk", func(t *testing.T) {
		// Extract hook commands from settings.json and verify each file exists.
		raw, _ := os.ReadFile(p.SettingsJSON)
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(raw, &settings); err != nil {
			t.Fatalf("unmarshal settings.json: %v", err)
		}

		var hooks map[string]json.RawMessage
		if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
			t.Fatalf("unmarshal hooks: %v", err)
		}

		for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
			hookPath := filepath.Join(p.StableHooksDir, name)
			if _, err := os.Stat(hookPath); err != nil {
				t.Errorf("hook referenced in settings.json does not exist on disk: %s", hookPath)
			}
		}
	})

	t.Run("mcp_json_has_stable_bin_path", func(t *testing.T) {
		raw, err := os.ReadFile(p.MCPJson)
		if err != nil {
			t.Fatalf("read .mcp.json: %v", err)
		}

		var cfg struct {
			MCPServers map[string]struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("unmarshal .mcp.json: %v", err)
		}

		entry, ok := cfg.MCPServers["claude-mesh"]
		if !ok {
			t.Fatal("claude-mesh entry missing from .mcp.json")
		}

		stableMCPBin := filepath.Join(p.StableBinDir, "claude-mesh-mcp")
		if entry.Command != stableMCPBin {
			t.Errorf("MCP command = %q, want %q", entry.Command, stableMCPBin)
		}

		// Command path must exist on disk.
		if _, err := os.Stat(entry.Command); err != nil {
			t.Errorf("MCP command %q does not exist on disk: %v", entry.Command, err)
		}

		// Must have env vars.
		if entry.Env["CLAUDE_MESH_REDIS_ADDR"] == "" {
			t.Error("MCP env missing CLAUDE_MESH_REDIS_ADDR")
		}
		if entry.Env["CLAUDE_MESH_MQTT_HOST"] == "" {
			t.Error("MCP env missing CLAUDE_MESH_MQTT_HOST")
		}
		if entry.Env["CLAUDE_MESH_MQTT_PORT"] == "" {
			t.Error("MCP env missing CLAUDE_MESH_MQTT_PORT")
		}

		// Must NOT reference workspace src dir.
		if strings.Contains(entry.Command, srcBinDir) {
			t.Errorf("MCP command still references workspace bin dir %q — must use stable path", srcBinDir)
		}
	})

	t.Run("settings_json_has_statusline_entry", func(t *testing.T) {
		raw, err := os.ReadFile(p.SettingsJSON)
		if err != nil {
			t.Fatalf("read settings.json: %v", err)
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(raw, &settings); err != nil {
			t.Fatalf("unmarshal settings.json: %v", err)
		}
		slRaw, ok := settings["statusLine"]
		if !ok {
			t.Fatal("statusLine key missing from settings.json after install")
		}
		wantCmd := p.BridgeBin + " statusline"
		if !strings.Contains(string(slRaw), wantCmd) {
			t.Errorf("statusLine command = %s, want to contain %q", slRaw, wantCmd)
		}
	})

	t.Run("neurostack_hook_untouched", func(t *testing.T) {
		raw, _ := os.ReadFile(p.SettingsJSON)
		if !strings.Contains(string(raw), "neurostack-session-context") {
			t.Error("NeuroStack hook removed after Claude Mesh install — must not be touched")
		}
	})

	t.Run("unrelated_mcp_entry_untouched", func(t *testing.T) {
		raw, _ := os.ReadFile(p.MCPJson)
		if !strings.Contains(string(raw), "mioboxtc") {
			t.Error("pre-existing mioboxtc MCP entry removed — must not be touched")
		}
	})
}

// TestIntegrationUninstall verifies the full uninstall flow.
func TestIntegrationUninstall(t *testing.T) {
	homeDir, srcBinDir, srcHooksDir := setupFakeHome(t)
	p := buildTestPaths(homeDir, srcBinDir, srcHooksDir)

	// Install first.
	if err := installer.Install(p); err != nil {
		t.Fatalf("Install (setup for uninstall): %v", err)
	}

	// Then uninstall.
	if err := installer.Uninstall(p); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	t.Run("stable_binaries_removed", func(t *testing.T) {
		for _, name := range []string{"claude-mesh-bridge", "claude-mesh-mcp"} {
			path := filepath.Join(p.StableBinDir, name)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("stable binary %s still exists after uninstall: %v", path, err)
			}
		}
	})

	t.Run("stable_hook_scripts_removed", func(t *testing.T) {
		for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
			path := filepath.Join(p.StableHooksDir, name)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("stable hook %s still exists after uninstall: %v", path, err)
			}
		}
	})

	t.Run("settings_json_has_no_claude_mesh_entries", func(t *testing.T) {
		raw, err := os.ReadFile(p.SettingsJSON)
		if err != nil {
			t.Fatalf("read settings.json after uninstall: %v", err)
		}
		if strings.Contains(string(raw), "claude-mesh-") {
			t.Errorf("settings.json still contains claude-mesh-* entries after uninstall:\n%s", raw)
		}
	})

	t.Run("settings_json_has_no_statusline_entry", func(t *testing.T) {
		raw, err := os.ReadFile(p.SettingsJSON)
		if err != nil {
			t.Fatalf("read settings.json after uninstall: %v", err)
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(raw, &settings); err != nil {
			t.Fatalf("unmarshal settings.json: %v", err)
		}
		if _, ok := settings["statusLine"]; ok {
			t.Errorf("statusLine still present in settings.json after uninstall:\n%s", raw)
		}
	})

	t.Run("mcp_json_has_no_claude_mesh_entry", func(t *testing.T) {
		raw, err := os.ReadFile(p.MCPJson)
		if err != nil {
			t.Fatalf("read .mcp.json after uninstall: %v", err)
		}
		if strings.Contains(string(raw), `"claude-mesh"`) {
			t.Errorf(".mcp.json still contains claude-mesh entry after uninstall:\n%s", raw)
		}
	})

	t.Run("neurostack_hook_still_present", func(t *testing.T) {
		raw, _ := os.ReadFile(p.SettingsJSON)
		if !strings.Contains(string(raw), "neurostack-session-context") {
			t.Error("NeuroStack hook was removed during uninstall — must NOT be touched")
		}
	})

	t.Run("unrelated_mcp_entry_still_present", func(t *testing.T) {
		raw, _ := os.ReadFile(p.MCPJson)
		if !strings.Contains(string(raw), "mioboxtc") {
			t.Error("pre-existing mioboxtc MCP entry removed during uninstall — must NOT be touched")
		}
	})
}

// TestIntegrationIdempotentInstall verifies installing twice does not duplicate entries.
func TestIntegrationIdempotentInstall(t *testing.T) {
	homeDir, srcBinDir, srcHooksDir := setupFakeHome(t)
	p := buildTestPaths(homeDir, srcBinDir, srcHooksDir)

	for i := 0; i < 2; i++ {
		if err := installer.Install(p); err != nil {
			t.Fatalf("Install run %d: %v", i+1, err)
		}
	}

	// settings.json must have exactly one claude-mesh-session-start entry.
	raw, _ := os.ReadFile(p.SettingsJSON)
	count := strings.Count(string(raw), "claude-mesh-session-start")
	if count != 1 {
		t.Errorf("idempotent install: claude-mesh-session-start appears %d times, want 1", count)
	}

	// .mcp.json must have exactly one claude-mesh entry.
	rawMCP, _ := os.ReadFile(p.MCPJson)
	mcpCount := strings.Count(string(rawMCP), `"claude-mesh"`)
	if mcpCount != 1 {
		t.Errorf("idempotent install: claude-mesh appears %d times in .mcp.json, want 1", mcpCount)
	}
}
