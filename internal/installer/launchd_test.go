package installer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marioser/claude-mesh/internal/installer"
)

// TestLaunchdPlistRendered verifies that the plist is rendered with the correct binary path.
// We test the template rendering by checking the written plist content.
// launchctl calls are skipped in unit tests — this is filesystem-only behavior.
func TestLaunchdPlistRendered(t *testing.T) {
	dir := t.TempDir()
	plistDst := filepath.Join(dir, "io.github.marioser.claude-mesh-bridge.plist")
	bridgeBin := "/usr/local/bin/claude-mesh-bridge"

	// We can't call launchctl in tests, so test the rendering directly.
	if err := installer.RenderPlist(plistDst, bridgeBin); err != nil {
		t.Fatalf("RenderPlist: %v", err)
	}

	content, err := os.ReadFile(plistDst)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}

	plistStr := string(content)

	if !strings.Contains(plistStr, bridgeBin) {
		t.Errorf("plist does not contain binary path %q", bridgeBin)
	}
	if !strings.Contains(plistStr, "io.github.marioser.claude-mesh-bridge") {
		t.Error("plist does not contain label io.github.marioser.claude-mesh-bridge")
	}
	if !strings.Contains(plistStr, "<true/>") {
		t.Error("plist does not contain KeepAlive/RunAtLoad <true/>")
	}
	if !strings.Contains(plistStr, "claude-mesh-bridge.log") {
		t.Error("plist does not reference log file")
	}
}

// TestLaunchdPlistIdempotent verifies that writing the plist twice produces identical output.
func TestLaunchdPlistIdempotent(t *testing.T) {
	dir := t.TempDir()
	plistDst := filepath.Join(dir, "io.github.marioser.claude-mesh-bridge.plist")
	bridgeBin := "/usr/local/bin/claude-mesh-bridge"

	if err := installer.RenderPlist(plistDst, bridgeBin); err != nil {
		t.Fatalf("RenderPlist first: %v", err)
	}
	first, _ := os.ReadFile(plistDst)

	if err := installer.RenderPlist(plistDst, bridgeBin); err != nil {
		t.Fatalf("RenderPlist second: %v", err)
	}
	second, _ := os.ReadFile(plistDst)

	if string(first) != string(second) {
		t.Error("plist content differs between two identical renders — not idempotent")
	}
}

// TestLaunchdUninstallRemovesPlist verifies that UninstallLaunchd removes the plist file.
func TestLaunchdUninstallRemovesPlist(t *testing.T) {
	dir := t.TempDir()
	plistDst := filepath.Join(dir, "io.github.marioser.claude-mesh-bridge.plist")

	// Create the plist file.
	if err := installer.RenderPlist(plistDst, "/bin/bridge"); err != nil {
		t.Fatalf("RenderPlist: %v", err)
	}

	// UninstallLaunchd calls launchctl which will fail (no launchd in CI tests),
	// but it should still remove the plist file.
	_ = installer.UninstallLaunchd(plistDst) // ignore launchctl error in unit tests

	if _, err := os.Stat(plistDst); !os.IsNotExist(err) {
		t.Error("plist file still exists after UninstallLaunchd")
	}
}
