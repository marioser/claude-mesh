package installer

import (
	"encoding/json"
	"fmt"
	"os"
)

const mcpServerKey = "claude-mesh"

// mcpConfig is the JSON structure for .mcp.json.
type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// defaultMCPEnv returns the default environment variables for the MCP server.
func defaultMCPEnv() map[string]string {
	return map[string]string{
		"CLAUDE_MESH_REDIS_ADDR": "localhost:6379",
		"CLAUDE_MESH_MQTT_HOST":  "localhost",
		"CLAUDE_MESH_MQTT_PORT":  "1883",
	}
}

// PatchMCP idempotently adds the claude-mesh MCP server entry to .mcp.json.
// mcpBinPath must be the STABLE binary path (e.g. ~/.local/bin/claude-mesh-mcp).
// The entry includes default env vars for Redis and MQTT connectivity.
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
		Env:     defaultMCPEnv(),
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
