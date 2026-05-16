// Package config parses Claude Mesh configuration from environment variables
// using caarlos0/env/v11. All packages receive config via constructor injection —
// none read env directly.
package config

import (
	"runtime"

	"github.com/caarlos0/env/v11"
)

// EnvOptions holds all configuration for Claude Mesh, loaded from environment variables.
type EnvOptions struct {
	// MQTT broker connection.
	MQTTHost     string `env:"CLAUDE_MESH_MQTT_HOST"      envDefault:"localhost"`
	MQTTPort     int    `env:"CLAUDE_MESH_MQTT_PORT"      envDefault:"1883"`
	MQTTUsername string `env:"CLAUDE_MESH_MQTT_USERNAME"  envDefault:""`
	MQTTPassword string `env:"CLAUDE_MESH_MQTT_PASSWORD"  envDefault:""`
	MQTTClientID string `env:"CLAUDE_MESH_MQTT_CLIENT_ID" envDefault:"claude-mesh"`

	// Redis connection.
	RedisAddr     string `env:"CLAUDE_MESH_REDIS_ADDR"     envDefault:"localhost:6379"`
	RedisPassword string `env:"CLAUDE_MESH_REDIS_PASSWORD" envDefault:""`
	RedisDB       int    `env:"CLAUDE_MESH_REDIS_DB"       envDefault:"0"`

	// Redis TTLs (seconds).
	SessionTTL         int `env:"CLAUDE_MESH_SESSION_TTL_S"  envDefault:"600"`
	ActivityPerSessTTL int `env:"CLAUDE_MESH_ACTIVITY_TTL_S" envDefault:"600"`
	ActivityGlobalTTL  int `env:"CLAUDE_MESH_GLOBAL_TTL_S"   envDefault:"1800"`
	ActivityRingSize   int `env:"CLAUDE_MESH_RING_SIZE"      envDefault:"50"`
	GlobalRingSize     int `env:"CLAUDE_MESH_GLOBAL_RING"    envDefault:"200"`

	// Bridge sweep ticker.
	SweepIntervalMs int `env:"CLAUDE_MESH_SWEEP_MS" envDefault:"10000"`

	// MCP handler latency budget.
	HandlerTimeoutMs int `env:"CLAUDE_MESH_HANDLER_TIMEOUT_MS" envDefault:"100"`

	// Logging.
	// LogPath is the full path to the log file. If empty, it is derived from LogDir.
	// LogDir is the directory containing the log file. If both are empty, an
	// OS-appropriate default is used: ~/Library/Logs on macOS, ~/.local/state/claude-mesh
	// (XDG_STATE_HOME) on Linux.
	LogPath  string `env:"CLAUDE_MESH_LOG_PATH"  envDefault:""`
	LogDir   string `env:"CLAUDE_MESH_LOG_DIR"   envDefault:""`
	LogLevel string `env:"CLAUDE_MESH_LOG_LEVEL" envDefault:"info"`

	// Anthropic usage API (optional).
	// When both fields are set, the daemon uses the official claude.ai usage API
	// as the primary source for 5h and weekly token percentages.
	// When empty (default), the daemon falls back to local JSONL parsing.
	AnthropicOrgID  string `env:"CLAUDE_MESH_ANTHROPIC_ORG_ID"  envDefault:""`
	AnthropicCookie string `env:"CLAUDE_MESH_ANTHROPIC_COOKIE"  envDefault:""`
}

// Load parses environment variables into EnvOptions and returns an error if any
// required field is missing or has an invalid value.
func Load() (EnvOptions, error) {
	var cfg EnvOptions
	if err := env.Parse(&cfg); err != nil {
		return EnvOptions{}, err
	}
	cfg.LogPath = resolveLogPath(cfg.LogPath, cfg.LogDir)
	return cfg, nil
}

// resolveLogPath returns the final log file path based on user overrides and OS.
// Precedence: LOG_PATH > LOG_DIR/claude-mesh-bridge.log > OS default.
func resolveLogPath(logPath, logDir string) string {
	if logPath != "" {
		return logPath
	}
	if logDir != "" {
		return logDir + "/claude-mesh-bridge.log"
	}
	if runtime.GOOS == "linux" {
		return "~/.local/state/claude-mesh/claude-mesh-bridge.log"
	}
	return "~/Library/Logs/claude-mesh-bridge.log"
}
