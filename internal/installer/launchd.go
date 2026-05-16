package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>io.github.marioser.claude-mesh-bridge</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BridgeBin}}</string>
		<string>run</string>
	</array>
	<key>KeepAlive</key>
	<true/>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/claude-mesh-bridge.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/claude-mesh-bridge.log</string>
</dict>
</plist>
`

type plistData struct {
	BridgeBin string
	LogDir    string
}

// RenderPlist renders the launchd plist template to plistDst without calling launchctl.
// Used by tests and as the first step of InstallLaunchd.
func RenderPlist(plistDst, bridgeBin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("launchd: home dir: %w", err)
	}

	data := plistData{
		BridgeBin: bridgeBin,
		LogDir:    home + "/Library/Logs",
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("launchd: parse template: %w", err)
	}

	tmp, err := os.CreateTemp("", "claude-mesh-plist-*.plist")
	if err != nil {
		return fmt.Errorf("launchd: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if err := tmpl.Execute(tmp, data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("launchd: render template: %w", err)
	}
	_ = tmp.Close()

	// Ensure parent directory exists before rename.
	if err := os.MkdirAll(filepath.Dir(plistDst), 0o755); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("launchd: create plist dir: %w", err)
	}
	if err := os.Rename(tmpName, plistDst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("launchd: write plist: %w", err)
	}
	return nil
}

// InstallLaunchd writes the plist and loads it via launchctl.
func InstallLaunchd(plistDst, bridgeBin string) error {
	if err := RenderPlist(plistDst, bridgeBin); err != nil {
		return err
	}
	// Idempotent: unload first (ignore error if not loaded), then load.
	_ = launchctlRun("unload", plistDst)
	if err := launchctlRun("load", "-w", plistDst); err != nil {
		return fmt.Errorf("launchd: launchctl load: %w", err)
	}
	return nil
}

// UninstallLaunchd unloads and removes the launchd plist.
func UninstallLaunchd(plistDst string) error {
	_ = launchctlRun("unload", plistDst)
	if err := os.Remove(plistDst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("launchd: remove plist: %w", err)
	}
	return nil
}
