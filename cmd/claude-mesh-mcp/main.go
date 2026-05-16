package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/redis/go-redis/v9"

	"github.com/marioser/claude-mesh/internal/config"
	"github.com/marioser/claude-mesh/internal/logging"
	mcpserver "github.com/marioser/claude-mesh/internal/mcp"
	"github.com/marioser/claude-mesh/internal/mqtt"
	"github.com/marioser/claude-mesh/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log, err := logging.New(cfg)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() { _ = redisClient.Close() }()

	s := store.NewRedisStore(redisClient, store.StoreConfig{
		SessionTTL:         cfg.SessionTTL,
		ActivityPerSessTTL: cfg.ActivityPerSessTTL,
		ActivityGlobalTTL:  cfg.ActivityGlobalTTL,
		ActivityRingSize:   cfg.ActivityRingSize,
		GlobalRingSize:     cfg.GlobalRingSize,
	})

	// Verify Redis is reachable; MCP server degrades gracefully if it goes down later.
	if err := s.HealthCheck(context.Background()); err != nil {
		// Log warning but don't exit — handlers will return degraded: true.
		log.Warn("redis unreachable at startup; handlers will return degraded responses")
	}

	// Create MQTT client for mesh_announce. Use a unique client ID per process (includes PID).
	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTHost, cfg.MQTTPort)
	clientID := fmt.Sprintf("claude-mesh-mcp-%d", os.Getpid())
	mqttClient := mqtt.NewPahoClient(brokerURL, clientID, cfg.MQTTUsername, cfg.MQTTPassword, nil)
	// Best-effort connect; mesh_announce returns degraded if MQTT is down.
	if err := mqttClient.Connect(context.Background()); err != nil {
		log.Warn("mqtt unreachable at startup; mesh_announce will return degraded responses")
	}
	defer mqttClient.Disconnect(500)

	srv := mcpserver.NewServer(s, cfg, mqttClient)
	return server.ServeStdio(srv)
}
