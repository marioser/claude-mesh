package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/redis/go-redis/v9"

	"claude-mesh/internal/config"
	"claude-mesh/internal/logging"
	mcpserver "claude-mesh/internal/mcp"
	"claude-mesh/internal/store"
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

	srv := mcpserver.NewServer(s, cfg)
	return server.ServeStdio(srv)
}
