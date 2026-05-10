// Package installer handles idempotent installation of Claude Mesh:
// - Claude Code hooks in ~/.claude/settings.json
// - MCP server entry in ~/.claude/.mcp.json
// - launchd plist at ~/Library/LaunchAgents/com.miobox.claude-mesh-bridge.plist
package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Paths holds all filesystem locations the installer reads/writes.
type Paths struct {
	SettingsJSON string // ~/.claude/settings.json
	MCPJson      string // ~/.claude/.mcp.json
	PlistDst     string // ~/Library/LaunchAgents/com.miobox.claude-mesh-bridge.plist
	BridgeBin    string // path to claude-mesh-bridge binary
	MCPBin       string // path to claude-mesh-mcp binary
	HooksDir     string // scripts/claude-mesh/hooks/
}

// DefaultPaths resolves standard install locations relative to the running binary.
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	exePath, _ := os.Executable()
	binDir := filepath.Dir(exePath)

	return Paths{
		SettingsJSON: filepath.Join(home, ".claude", "settings.json"),
		MCPJson:      filepath.Join(home, ".claude", ".mcp.json"),
		PlistDst:     filepath.Join(home, "Library", "LaunchAgents", "com.miobox.claude-mesh-bridge.plist"),
		BridgeBin:    exePath,
		MCPBin:       filepath.Join(binDir, "claude-mesh-mcp"),
		HooksDir:     filepath.Join(binDir, "..", "..", "hooks"),
	}
}

// Install performs the full installation in the correct order.
func Install(p Paths) error {
	// Verify binaries exist first.
	if _, err := os.Stat(p.BridgeBin); err != nil {
		return fmt.Errorf("installer: bridge binary not found at %s — run make build first", p.BridgeBin)
	}
	if _, err := os.Stat(p.MCPBin); err != nil {
		return fmt.Errorf("installer: MCP binary not found at %s — run make build first", p.MCPBin)
	}

	if err := PatchSettings(p.SettingsJSON, p.BridgeBin, p.HooksDir); err != nil {
		return fmt.Errorf("installer: patch settings.json: %w", err)
	}

	if err := PatchMCP(p.MCPJson, p.MCPBin); err != nil {
		return fmt.Errorf("installer: patch .mcp.json: %w", err)
	}

	if runtime.GOOS == "darwin" {
		if err := InstallLaunchd(p.PlistDst, p.BridgeBin); err != nil {
			return fmt.Errorf("installer: launchd: %w", err)
		}
	}

	fmt.Println("claude-mesh: installed successfully")
	return nil
}

// Uninstall removes all Claude Mesh installation artifacts.
func Uninstall(p Paths) error {
	if runtime.GOOS == "darwin" {
		if err := UninstallLaunchd(p.PlistDst); err != nil {
			fmt.Fprintf(os.Stderr, "claude-mesh uninstall: launchd: %v (continuing)\n", err)
		}
	}

	if err := RemoveHooks(p.SettingsJSON); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove hooks: %v (continuing)\n", err)
	}

	if err := RemoveMCP(p.MCPJson); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove mcp: %v (continuing)\n", err)
	}

	fmt.Println("claude-mesh: uninstalled")
	return nil
}

// launchctlRun executes a launchctl command with output piped to stderr.
func launchctlRun(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
