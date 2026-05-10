package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const hookMarkerPrefix = "claude-mesh-"

// hookEntry represents a single Claude Code hook entry in settings.json.
type hookEntry struct {
	Matcher string   `json:"matcher"`
	Hooks   []hookDef `json:"hooks"`
}

type hookDef struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Name    string `json:"name,omitempty"`
}

// PatchSettings idempotently adds Claude Mesh hook entries to settings.json.
// Existing NeuroStack or other hooks are never modified.
// Uses atomic write (temp file + os.Rename).
func PatchSettings(path, bridgeBin, hooksDir string) error {
	// Read existing file or create skeleton.
	raw, err := readOrCreate(path)
	if err != nil {
		return err
	}

	// Backup before modification.
	if len(raw) > 0 {
		if err := os.WriteFile(path+".bak", raw, 0o644); err != nil {
			return fmt.Errorf("hooks: backup: %w", err)
		}
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("hooks: malformed settings.json: %w", err)
	}

	// Parse existing hooks.
	existingHooks := map[string]json.RawMessage{}
	if hooksRaw, ok := settings["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &existingHooks)
	}

	// Build the 3 Claude Mesh hook entries (idempotent by marker name).
	newEntries := buildHookEntries(bridgeBin, hooksDir)
	for event, entry := range newEntries {
		if isClaudeMeshAlreadyPresent(existingHooks, event) {
			continue
		}
		existingHooks[event] = mergeHookEntry(existingHooks[event], entry)
	}

	hooksRaw, err := json.Marshal(existingHooks)
	if err != nil {
		return fmt.Errorf("hooks: marshal hooks: %w", err)
	}
	settings["hooks"] = hooksRaw

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("hooks: marshal settings: %w", err)
	}

	return atomicWrite(path, out)
}

// RemoveHooks removes all Claude Mesh hook entries from settings.json.
func RemoveHooks(path string) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return err
	}

	existingHooks := map[string]json.RawMessage{}
	if hooksRaw, ok := settings["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &existingHooks)
	}

	for event, entriesRaw := range existingHooks {
		filtered := removeClaudeMeshEntries(entriesRaw)
		existingHooks[event] = filtered
	}

	hooksRaw, _ := json.Marshal(existingHooks)
	settings["hooks"] = hooksRaw

	out, _ := json.MarshalIndent(settings, "", "  ")
	return atomicWrite(path, out)
}

// buildHookEntries returns the 3 hook entries Claude Mesh needs.
// bridgeBin is accepted for future use (scripts may embed the binary path).
func buildHookEntries(_ string, hooksDir string) map[string]hookEntry {
	sessionStartScript := filepath.Join(hooksDir, "session-start.sh")
	preToolUseScript := filepath.Join(hooksDir, "pre-tool-use.sh")
	stopScript := filepath.Join(hooksDir, "stop.sh")

	return map[string]hookEntry{
		"SessionStart": {
			Matcher: "startup|clear",
			Hooks: []hookDef{{
				Type:    "command",
				Command: fmt.Sprintf("bash %s", sessionStartScript),
				Name:    hookMarkerPrefix + "session-start",
			}},
		},
		"PreToolUse": {
			Hooks: []hookDef{{
				Type:    "command",
				Command: fmt.Sprintf("bash %s", preToolUseScript),
				Name:    hookMarkerPrefix + "pre-tool-use",
			}},
		},
		"Stop": {
			Hooks: []hookDef{{
				Type:    "command",
				Command: fmt.Sprintf("bash %s", stopScript),
				Name:    hookMarkerPrefix + "stop",
			}},
		},
	}
}

// isClaudeMeshAlreadyPresent checks if a claude-mesh-* named hook already exists
// under the given event key.
func isClaudeMeshAlreadyPresent(hooks map[string]json.RawMessage, event string) bool {
	raw, ok := hooks[event]
	if !ok {
		return false
	}
	return strings.Contains(string(raw), hookMarkerPrefix)
}

// mergeHookEntry appends newEntry's hooks into the existing entry JSON array.
func mergeHookEntry(existingRaw json.RawMessage, newEntry hookEntry) json.RawMessage {
	if len(existingRaw) == 0 {
		out, _ := json.Marshal([]hookEntry{newEntry})
		return out
	}

	var existing []json.RawMessage
	if err := json.Unmarshal(existingRaw, &existing); err != nil {
		// If it's a single object, wrap it.
		existing = []json.RawMessage{existingRaw}
	}

	newRaw, _ := json.Marshal(newEntry)
	existing = append(existing, newRaw)

	out, _ := json.Marshal(existing)
	return out
}

// removeClaudeMeshEntries strips hook entries with claude-mesh-* name from the JSON array.
func removeClaudeMeshEntries(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return raw
	}
	var kept []json.RawMessage
	for _, e := range entries {
		if !strings.Contains(string(e), hookMarkerPrefix) {
			kept = append(kept, e)
		}
	}
	if kept == nil {
		kept = []json.RawMessage{}
	}
	out, _ := json.Marshal(kept)
	return out
}

// readOrCreate reads path or returns an empty JSON object skeleton if missing.
func readOrCreate(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []byte(`{}`), nil
	}
	return raw, err
}

// atomicWrite writes data to path atomically via temp file + rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".claude-mesh-tmp-*")
	if err != nil {
		return fmt.Errorf("atomicWrite: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomicWrite: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomicWrite: close: %w", err)
	}
	return os.Rename(tmpName, path)
}
