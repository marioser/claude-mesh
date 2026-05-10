package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisStore is the concrete implementation of Store backed by go-redis/v9.
// It is safe for concurrent use — *redis.Client manages its own connection pool.
type RedisStore struct {
	client *redis.Client
	cfg    StoreConfig
}

// NewRedisStore creates a RedisStore with the provided *redis.Client and config.
// The client's connection pool is not created here; callers must call HealthCheck
// to verify connectivity before using the store in production paths.
func NewRedisStore(client *redis.Client, cfg StoreConfig) Store {
	return &RedisStore{client: client, cfg: cfg}
}

// HealthCheck pings Redis. Returns nil if the server is reachable.
func (r *RedisStore) HealthCheck(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("store.HealthCheck: %w", err)
	}
	return nil
}
