package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const statusLineMarker = "claude-mesh-bridge statusline"

// statusLineEntry is the shape of Claude Code's statusLine config block.
type statusLineEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// PatchStatusLine idempotently adds a statusLine entry pointing to the given bridge binary.
// The entry uses Claude Code's "statusLine" top-level key in settings.json.
// If the entry already points to a claude-mesh-bridge binary, it is left unchanged.
// Uses atomic write (temp file + os.Rename).
func PatchStatusLine(path, bridgeBin string) error {
	raw, err := readOrCreate(path)
	if err != nil {
		return err
	}

	// Backup before modification (best-effort — ignore error).
	if len(raw) > 0 {
		_ = os.WriteFile(path+".bak", raw, 0o644)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("statusline: malformed settings.json: %w", err)
	}

	// Idempotency: if statusLine already points to a claude-mesh-bridge, do nothing.
	if existing, ok := settings["statusLine"]; ok {
		if strings.Contains(string(existing), statusLineMarker) {
			return nil
		}
	}

	entry := statusLineEntry{
		Type:    "command",
		Command: bridgeBin + " statusline",
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("statusline: marshal: %w", err)
	}
	settings["statusLine"] = entryRaw

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("statusline: marshal settings: %w", err)
	}

	return atomicWrite(path, out)
}

// RemoveStatusLine removes the statusLine entry from settings.json, but ONLY if
// the command points to a claude-mesh-bridge binary. Other statusLine entries are
// left untouched.
func RemoveStatusLine(path, bridgeBin string) error {
	raw, err := readFileIfExists(path)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil // file does not exist, nothing to do
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("statusline: malformed settings.json: %w", err)
	}

	existing, ok := settings["statusLine"]
	if !ok {
		return nil // nothing to remove
	}

	// Only remove if it points to our binary.
	if !strings.Contains(string(existing), statusLineMarker) {
		return nil
	}

	delete(settings, "statusLine")

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("statusline: marshal settings: %w", err)
	}

	return atomicWrite(path, out)
}

// readFileIfExists reads a file, returning nil if it does not exist.
func readFileIfExists(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return raw, err
}
