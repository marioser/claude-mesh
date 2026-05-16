// Package installer handles idempotent installation of Claude Mesh:
// - Claude Code hooks in ~/.claude/settings.json
// - MCP server entry in <repo-root>/.mcp.json
// - launchd plist at ~/Library/LaunchAgents/io.github.marioser.claude-mesh-bridge.plist
// - Stable binary copies in ~/.local/bin/
// - Stable hook copies in ~/.claude/hooks/claude-mesh/
package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Paths holds all filesystem locations the installer reads/writes.
type Paths struct {
	// Source paths (workspace / build output).
	SrcBridgeBin string // e.g. dist/claude-mesh-bridge
	SrcMCPBin    string // e.g. dist/claude-mesh-mcp
	SrcHooksDir  string // e.g. hooks/ (contains *.sh scripts)

	// Stable target directories (survive mioboxwt clean).
	StableBinDir   string // ~/.local/bin/
	StableHooksDir string // ~/.claude/hooks/claude-mesh/

	// Derived stable binary paths (set by DefaultPaths or tests).
	BridgeBin string // StableBinDir/claude-mesh-bridge
	MCPBin    string // StableBinDir/claude-mesh-mcp
	HooksDir  string // StableHooksDir (alias, used by PatchSettings)

	// Config file targets.
	SettingsJSON string // ~/.claude/settings.json
	MCPJson      string // <repo-root>/.mcp.json (or ~/.claude/.mcp.json as fallback)
	PlistDst     string // ~/Library/LaunchAgents/io.github.marioser.claude-mesh-bridge.plist
}

// DefaultPaths resolves standard install locations relative to the running binary.
// SrcBridgeBin = current executable (dist/claude-mesh-bridge)
// SrcMCPBin    = dist/claude-mesh-mcp (sibling)
// SrcHooksDir  = dist/../../hooks (relative to dist/)
// StableBinDir = ~/.local/bin/
// StableHooksDir = ~/.claude/hooks/claude-mesh/
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	exePath, _ := os.Executable()
	distDir := filepath.Dir(exePath)
	// hooks/ is a sibling of dist/ in the claude-mesh project root.
	// dist/claude-mesh-bridge → dist/ → ../hooks → scripts/claude-mesh/hooks/
	srcHooksDir := filepath.Clean(filepath.Join(distDir, "..", "hooks"))

	stableBinDir := filepath.Join(home, ".local", "bin")
	stableHooksDir := filepath.Join(home, ".claude", "hooks", "claude-mesh")

	// Detect repo root for .mcp.json placement; fallback to ~/.claude/.mcp.json.
	mcpJsonPath := detectMCPJsonPath(home)

	return Paths{
		SrcBridgeBin: exePath,
		SrcMCPBin:    filepath.Join(distDir, "claude-mesh-mcp"),
		SrcHooksDir:  srcHooksDir,

		StableBinDir:   stableBinDir,
		StableHooksDir: stableHooksDir,

		BridgeBin: filepath.Join(stableBinDir, "claude-mesh-bridge"),
		MCPBin:    filepath.Join(stableBinDir, "claude-mesh-mcp"),
		HooksDir:  stableHooksDir,

		SettingsJSON: filepath.Join(home, ".claude", "settings.json"),
		MCPJson:      mcpJsonPath,
		PlistDst:     filepath.Join(home, "Library", "LaunchAgents", "io.github.marioser.claude-mesh-bridge.plist"),
	}
}

// detectMCPJsonPath finds the repo root via git and returns <root>/.mcp.json.
// Falls back to ~/.claude/.mcp.json if not in a git repo.
func detectMCPJsonPath(home string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		repoRoot := filepath.Clean(string(out[:len(out)-1])) // strip trailing newline
		return filepath.Join(repoRoot, ".mcp.json")
	}
	// Fallback: ~/.claude/.mcp.json
	return filepath.Join(home, ".claude", ".mcp.json")
}

// Install performs the full installation in the correct order:
// 1. Create stable dirs
// 2. Copy binaries and hooks to stable locations
// 3. Patch settings.json (hooks pointing to stable locations)
// 4. Patch .mcp.json (MCP server entry with stable bin path)
// 5. Install launchd plist (darwin only, using stable bin path)
func Install(p Paths) error {
	// Verify source binaries exist.
	if _, err := os.Stat(p.SrcBridgeBin); err != nil {
		return fmt.Errorf("installer: bridge binary not found at %s — run make build first", p.SrcBridgeBin)
	}
	if _, err := os.Stat(p.SrcMCPBin); err != nil {
		return fmt.Errorf("installer: MCP binary not found at %s — run make build first", p.SrcMCPBin)
	}

	// 1. Create stable directories.
	if err := os.MkdirAll(p.StableBinDir, 0o755); err != nil {
		return fmt.Errorf("installer: create stable bin dir %s: %w", p.StableBinDir, err)
	}
	if err := os.MkdirAll(p.StableHooksDir, 0o755); err != nil {
		return fmt.Errorf("installer: create stable hooks dir %s: %w", p.StableHooksDir, err)
	}

	// 2a. Copy binaries to stable dir.
	for _, pair := range [][2]string{
		{p.SrcBridgeBin, p.BridgeBin},
		{p.SrcMCPBin, p.MCPBin},
	} {
		if err := copyFileExec(pair[0], pair[1]); err != nil {
			return fmt.Errorf("installer: copy binary %s → %s: %w", pair[0], pair[1], err)
		}
	}

	// 2b. Copy hook scripts to stable hooks dir (executable).
	for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
		src := filepath.Join(p.SrcHooksDir, name)
		dst := filepath.Join(p.StableHooksDir, name)
		if err := copyFile(src, dst, 0o755); err != nil {
			return fmt.Errorf("installer: copy hook %s → %s: %w", src, dst, err)
		}
	}

	// 3. Patch settings.json with stable hooks dir and statusLine entry.
	if err := PatchSettings(p.SettingsJSON, p.BridgeBin, p.HooksDir); err != nil {
		return fmt.Errorf("installer: patch settings.json: %w", err)
	}
	if err := PatchStatusLine(p.SettingsJSON, p.BridgeBin); err != nil {
		return fmt.Errorf("installer: patch statusLine: %w", err)
	}

	// 4. Patch .mcp.json with stable MCP bin + env vars.
	if err := PatchMCP(p.MCPJson, p.MCPBin); err != nil {
		return fmt.Errorf("installer: patch .mcp.json: %w", err)
	}

	// 5. Install launchd plist (darwin only).
	if runtime.GOOS == "darwin" {
		if err := InstallLaunchd(p.PlistDst, p.BridgeBin); err != nil {
			return fmt.Errorf("installer: launchd: %w", err)
		}
	}

	fmt.Println("claude-mesh: installed successfully")
	fmt.Printf("  binaries → %s\n", p.StableBinDir)
	fmt.Printf("  hooks    → %s\n", p.StableHooksDir)
	fmt.Printf("  mcp.json → %s\n", p.MCPJson)
	return nil
}

// Uninstall removes all Claude Mesh installation artifacts:
// - Stable binaries from ~/.local/bin/
// - Stable hook scripts from ~/.claude/hooks/claude-mesh/
// - claude-mesh-* entries from settings.json
// - claude-mesh entry from .mcp.json
// - launchd plist (darwin only)
//
// NeuroStack hooks and other unrelated entries are NEVER removed.
func Uninstall(p Paths) error {
	// Remove launchd plist (darwin only) — ignore error, continue.
	if runtime.GOOS == "darwin" {
		if err := UninstallLaunchd(p.PlistDst); err != nil {
			fmt.Fprintf(os.Stderr, "claude-mesh uninstall: launchd: %v (continuing)\n", err)
		}
	}

	// Remove stable binaries.
	for _, bin := range []string{p.BridgeBin, p.MCPBin} {
		if err := os.Remove(bin); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove %s: %v (continuing)\n", bin, err)
		}
	}

	// Remove stable hook scripts.
	for _, name := range []string{"session-start.sh", "pre-tool-use.sh", "stop.sh"} {
		path := filepath.Join(p.StableHooksDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove hook %s: %v (continuing)\n", path, err)
		}
	}

	// Remove hooks and statusLine from settings.json.
	if err := RemoveHooks(p.SettingsJSON); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove hooks: %v (continuing)\n", err)
	}
	if err := RemoveStatusLine(p.SettingsJSON, p.BridgeBin); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove statusLine: %v (continuing)\n", err)
	}

	// Remove MCP entry from .mcp.json.
	if err := RemoveMCP(p.MCPJson); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh uninstall: remove mcp: %v (continuing)\n", err)
	}

	fmt.Println("claude-mesh: uninstalled")
	return nil
}

// copyFileExec copies src to dst preserving/forcing executable permission (0o755).
func copyFileExec(src, dst string) error {
	return copyFile(src, dst, 0o755)
}

// copyFile copies src to dst with the given file mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %s: %w", src, err)
	}
	defer in.Close()

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open dst %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return nil
}

// launchctlRun executes a launchctl command with output piped to stderr.
func launchctlRun(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
