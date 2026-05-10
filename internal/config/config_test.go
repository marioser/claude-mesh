package config_test

import (
	"testing"

	"claude-mesh/internal/config"
)

// TestDefaultValues verifies that Load() returns design-specified defaults when no env vars are set.
func TestDefaultValues(t *testing.T) {
	// Clear any env that could leak from the outer environment.
	unsetEnv(t, allEnvKeys...)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	cases := []struct {
		name string
		got  any
		want any
	}{
		{"MQTTHost", cfg.MQTTHost, "localhost"},
		{"MQTTPort", cfg.MQTTPort, 1883},
		{"MQTTClientID", cfg.MQTTClientID, "claude-mesh"},
		{"RedisAddr", cfg.RedisAddr, "localhost:6379"},
		{"RedisDB", cfg.RedisDB, 0},
		{"SessionTTL", cfg.SessionTTL, 600},
		{"ActivityPerSessTTL", cfg.ActivityPerSessTTL, 600},
		{"ActivityGlobalTTL", cfg.ActivityGlobalTTL, 1800},
		{"ActivityRingSize", cfg.ActivityRingSize, 50},
		{"GlobalRingSize", cfg.GlobalRingSize, 200},
		{"SweepIntervalMs", cfg.SweepIntervalMs, 10000},
		{"HandlerTimeoutMs", cfg.HandlerTimeoutMs, 100},
		{"LogLevel", cfg.LogLevel, "info"},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestEnvOverrides verifies that env vars override all default values.
func TestEnvOverrides(t *testing.T) {
	unsetEnv(t, allEnvKeys...)

	t.Setenv("CLAUDE_MESH_MQTT_HOST", "broker.local")
	t.Setenv("CLAUDE_MESH_MQTT_PORT", "8883")
	t.Setenv("CLAUDE_MESH_MQTT_USERNAME", "user1")
	t.Setenv("CLAUDE_MESH_MQTT_PASSWORD", "secret")
	t.Setenv("CLAUDE_MESH_MQTT_CLIENT_ID", "custom-id")
	t.Setenv("CLAUDE_MESH_REDIS_ADDR", "redis.local:6380")
	t.Setenv("CLAUDE_MESH_REDIS_PASSWORD", "redispass")
	t.Setenv("CLAUDE_MESH_REDIS_DB", "2")
	t.Setenv("CLAUDE_MESH_SESSION_TTL_S", "120")
	t.Setenv("CLAUDE_MESH_ACTIVITY_TTL_S", "900")
	t.Setenv("CLAUDE_MESH_GLOBAL_TTL_S", "3600")
	t.Setenv("CLAUDE_MESH_RING_SIZE", "100")
	t.Setenv("CLAUDE_MESH_GLOBAL_RING", "500")
	t.Setenv("CLAUDE_MESH_SWEEP_MS", "5000")
	t.Setenv("CLAUDE_MESH_HANDLER_TIMEOUT_MS", "200")
	t.Setenv("CLAUDE_MESH_LOG_LEVEL", "debug")
	t.Setenv("CLAUDE_MESH_LOG_PATH", "/tmp/test.log")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.MQTTHost != "broker.local" {
		t.Errorf("MQTTHost: got %q, want %q", cfg.MQTTHost, "broker.local")
	}
	if cfg.MQTTPort != 8883 {
		t.Errorf("MQTTPort: got %d, want %d", cfg.MQTTPort, 8883)
	}
	if cfg.MQTTUsername != "user1" {
		t.Errorf("MQTTUsername: got %q, want %q", cfg.MQTTUsername, "user1")
	}
	if cfg.MQTTPassword != "secret" {
		t.Errorf("MQTTPassword: got %q, want %q", cfg.MQTTPassword, "secret")
	}
	if cfg.MQTTClientID != "custom-id" {
		t.Errorf("MQTTClientID: got %q, want %q", cfg.MQTTClientID, "custom-id")
	}
	if cfg.RedisAddr != "redis.local:6380" {
		t.Errorf("RedisAddr: got %q, want %q", cfg.RedisAddr, "redis.local:6380")
	}
	if cfg.RedisPassword != "redispass" {
		t.Errorf("RedisPassword: got %q, want %q", cfg.RedisPassword, "redispass")
	}
	if cfg.RedisDB != 2 {
		t.Errorf("RedisDB: got %d, want %d", cfg.RedisDB, 2)
	}
	if cfg.SessionTTL != 120 {
		t.Errorf("SessionTTL: got %d, want %d", cfg.SessionTTL, 120)
	}
	if cfg.ActivityPerSessTTL != 900 {
		t.Errorf("ActivityPerSessTTL: got %d, want %d", cfg.ActivityPerSessTTL, 900)
	}
	if cfg.ActivityGlobalTTL != 3600 {
		t.Errorf("ActivityGlobalTTL: got %d, want %d", cfg.ActivityGlobalTTL, 3600)
	}
	if cfg.ActivityRingSize != 100 {
		t.Errorf("ActivityRingSize: got %d, want %d", cfg.ActivityRingSize, 100)
	}
	if cfg.GlobalRingSize != 500 {
		t.Errorf("GlobalRingSize: got %d, want %d", cfg.GlobalRingSize, 500)
	}
	if cfg.SweepIntervalMs != 5000 {
		t.Errorf("SweepIntervalMs: got %d, want %d", cfg.SweepIntervalMs, 5000)
	}
	if cfg.HandlerTimeoutMs != 200 {
		t.Errorf("HandlerTimeoutMs: got %d, want %d", cfg.HandlerTimeoutMs, 200)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.LogPath != "/tmp/test.log" {
		t.Errorf("LogPath: got %q, want %q", cfg.LogPath, "/tmp/test.log")
	}
}

// allEnvKeys lists every env var consumed by the config package.
var allEnvKeys = []string{
	"CLAUDE_MESH_MQTT_HOST",
	"CLAUDE_MESH_MQTT_PORT",
	"CLAUDE_MESH_MQTT_USERNAME",
	"CLAUDE_MESH_MQTT_PASSWORD",
	"CLAUDE_MESH_MQTT_CLIENT_ID",
	"CLAUDE_MESH_REDIS_ADDR",
	"CLAUDE_MESH_REDIS_PASSWORD",
	"CLAUDE_MESH_REDIS_DB",
	"CLAUDE_MESH_SESSION_TTL_S",
	"CLAUDE_MESH_ACTIVITY_TTL_S",
	"CLAUDE_MESH_GLOBAL_TTL_S",
	"CLAUDE_MESH_RING_SIZE",
	"CLAUDE_MESH_GLOBAL_RING",
	"CLAUDE_MESH_SWEEP_MS",
	"CLAUDE_MESH_HANDLER_TIMEOUT_MS",
	"CLAUDE_MESH_LOG_PATH",
	"CLAUDE_MESH_LOG_LEVEL",
}

// unsetEnv clears all specified env vars and restores them via t.Cleanup.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, "") // t.Setenv handles cleanup automatically
	}
}
