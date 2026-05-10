package installer

import (
	"encoding/json"
	"fmt"
	"os"
)

const mcpServerKey = "claude-mesh"

// mcpConfig is the JSON structure for ~/.claude/.mcp.json.
type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// PatchMCP idempotently adds the claude-mesh MCP server entry to .mcp.json.
func PatchMCP(path, mcpBinPath string) error {
	raw, err := readOrCreate(path)
	if err != nil {
		return err
	}

	// Backup.
	if len(raw) > 2 { // not just empty object
		_ = os.WriteFile(path+".bak", raw, 0o644)
	}

	var cfg mcpConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("mcp: malformed .mcp.json: %w", err)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = map[string]mcpServerEntry{}
	}

	// Idempotent: skip if already present.
	if _, exists := cfg.MCPServers[mcpServerKey]; exists {
		return nil
	}

	cfg.MCPServers[mcpServerKey] = mcpServerEntry{
		Command: mcpBinPath,
		Args:    []string{},
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("mcp: marshal: %w", err)
	}

	return atomicWrite(path, out)
}

// RemoveMCP removes the claude-mesh entry from .mcp.json.
func RemoveMCP(path string) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var cfg mcpConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	delete(cfg.MCPServers, mcpServerKey)

	out, _ := json.MarshalIndent(cfg, "", "  ")
	return atomicWrite(path, out)
}
