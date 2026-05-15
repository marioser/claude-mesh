package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// MQTTHealthKey is the Redis Hash key where the bridge writes MQTT health state.
const MQTTHealthKey = "claude:mesh:bridge:mqtt_health"

// MQTTHealthFields is the data structure written to MQTTHealthKey.
type MQTTHealthFields struct {
	Connected   bool
	Subscribed  bool
	MsgCount    int64
	LastMsgMs   int64
	StartedAtMs int64
	UpdatedAtMs int64
}

// RedisStore is the concrete implementation of Store backed by go-redis/v9.
// It is safe for concurrent use — *redis.Client manages its own connection pool.
type RedisStore struct {
	client *redis.Client
	cfg    StoreConfig
}

// NewRedisStore creates a RedisStore with the provided *redis.Client and config.
// The client's connection pool is not created here; callers must call HealthCheck
// to verify connectivity before using the store in production paths.
// *RedisStore satisfies the Store interface; concrete type returned so callers
// can access extended methods (WriteMQTTHealth, ReadMQTTHealth) without a type assertion.
func NewRedisStore(client *redis.Client, cfg StoreConfig) *RedisStore {
	return &RedisStore{client: client, cfg: cfg}
}

// HealthCheck pings Redis. Returns nil if the server is reachable.
func (r *RedisStore) HealthCheck(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("store.HealthCheck: %w", err)
	}
	return nil
}

// WriteMQTTHealth writes MQTT subscriber health state to Redis with a 60s TTL.
// Called by the bridge every sweep tick so status queries get fresh data.
func (r *RedisStore) WriteMQTTHealth(ctx context.Context, h MQTTHealthFields) error {
	connected := "0"
	if h.Connected {
		connected = "1"
	}
	subscribed := "0"
	if h.Subscribed {
		subscribed = "1"
	}
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, MQTTHealthKey, map[string]any{
		"connected":    connected,
		"subscribed":   subscribed,
		"msg_count":    h.MsgCount,
		"last_msg_ms":  h.LastMsgMs,
		"started_at":   h.StartedAtMs,
		"updated_at":   h.UpdatedAtMs,
	})
	pipe.Expire(ctx, MQTTHealthKey, 60*time.Second)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("store.WriteMQTTHealth: %w", err)
	}
	return nil
}

// ReadMQTTHealth reads the MQTT health Hash. Returns nil fields (zero value) and
// connected=false when the key is absent or expired.
func (r *RedisStore) ReadMQTTHealth(ctx context.Context) (MQTTHealthFields, bool, error) {
	result, err := r.client.HGetAll(ctx, MQTTHealthKey).Result()
	if err != nil {
		return MQTTHealthFields{}, false, fmt.Errorf("store.ReadMQTTHealth: %w", err)
	}
	if len(result) == 0 {
		return MQTTHealthFields{}, false, nil
	}
	h := MQTTHealthFields{
		Connected:  result["connected"] == "1",
		Subscribed: result["subscribed"] == "1",
	}
	if v, err := strconv.ParseInt(result["msg_count"], 10, 64); err == nil {
		h.MsgCount = v
	}
	if v, err := strconv.ParseInt(result["last_msg_ms"], 10, 64); err == nil {
		h.LastMsgMs = v
	}
	if v, err := strconv.ParseInt(result["started_at"], 10, 64); err == nil {
		h.StartedAtMs = v
	}
	if v, err := strconv.ParseInt(result["updated_at"], 10, 64); err == nil {
		h.UpdatedAtMs = v
	}
	return h, true, nil
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
