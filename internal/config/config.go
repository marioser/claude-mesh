// Package config parses Claude Mesh configuration from environment variables
// using caarlos0/env/v11. All packages receive config via constructor injection —
// none read env directly.
package config

import "github.com/caarlos0/env/v11"

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
	SessionTTL         int `env:"CLAUDE_MESH_SESSION_TTL_S"  envDefault:"90"`
	ActivityPerSessTTL int `env:"CLAUDE_MESH_ACTIVITY_TTL_S" envDefault:"600"`
	ActivityGlobalTTL  int `env:"CLAUDE_MESH_GLOBAL_TTL_S"   envDefault:"1800"`
	ActivityRingSize   int `env:"CLAUDE_MESH_RING_SIZE"      envDefault:"50"`
	GlobalRingSize     int `env:"CLAUDE_MESH_GLOBAL_RING"    envDefault:"200"`

	// Bridge sweep ticker.
	SweepIntervalMs int `env:"CLAUDE_MESH_SWEEP_MS" envDefault:"10000"`

	// MCP handler latency budget.
	HandlerTimeoutMs int `env:"CLAUDE_MESH_HANDLER_TIMEOUT_MS" envDefault:"100"`

	// Logging.
	LogPath  string `env:"CLAUDE_MESH_LOG_PATH"  envDefault:"~/Library/Logs/claude-mesh-bridge.log"`
	LogLevel string `env:"CLAUDE_MESH_LOG_LEVEL" envDefault:"info"`
}

// Load parses environment variables into EnvOptions and returns an error if any
// required field is missing or has an invalid value.
func Load() (EnvOptions, error) {
	var cfg EnvOptions
	if err := env.Parse(&cfg); err != nil {
		return EnvOptions{}, err
	}
	return cfg, nil
}
