package store

import (
	"context"
	"fmt"
	"strconv"

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

// GetString returns the string value at key. Returns error if the key does not exist.
func (r *RedisStore) GetString(ctx context.Context, key string) (string, error) {
	v, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("store.GetString %q: %w", key, err)
	}
	return v, nil
}

// GetInt parses the value at key as an integer. Returns error if key is missing or not numeric.
func (r *RedisStore) GetInt(ctx context.Context, key string) (int, error) {
	s, err := r.GetString(ctx, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("store.GetInt %q: %w", key, err)
	}
	return n, nil
}

// GetFloat parses the value at key as a float64. Returns error if key is missing or not numeric.
func (r *RedisStore) GetFloat(ctx context.Context, key string) (float64, error) {
	s, err := r.GetString(ctx, key)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("store.GetFloat %q: %w", key, err)
	}
	return f, nil
}
