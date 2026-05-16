// Package bridge orchestrates the Claude Mesh daemon: receives MQTT messages
// via a Subscriber, routes them to the Store, and runs a cleanup ticker.
package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/mqtt"
	"github.com/marioser/claude-mesh/internal/store"
)

const (
	chanCap              = 256
	workerCount          = 2
	defaultSweepInterval = 10 * time.Second

	// Boot-retry backoff: 1s → 2s → 4s → 8s → 16s → 32s (capped).
	retryInitial = 1 * time.Second
	retryCap     = 32 * time.Second
)

// Subscriber is the minimal interface the bridge depends on for receiving MQTT messages.
type Subscriber interface {
	Subscribe(ctx context.Context, handler mqtt.MessageHandler) error
}

// StatsProvider is an optional interface implemented by *mqtt.Subscriber.
// When the Bridge's Subscriber also implements this, health state is written to Redis
// on every sweep tick.
type StatsProvider interface {
	Stats() mqtt.SubscriberStats
}

// HealthWriter is the Redis-backed writer for MQTT health state.
// Implemented by *store.RedisStore (not on Store interface — bridge-specific).
type HealthWriter interface {
	WriteMQTTHealth(ctx context.Context, h store.MQTTHealthFields) error
}

// envelope wraps an inbound MQTT message for async processing.
type envelope struct {
	topic   string
	payload []byte
}

// Bridge subscribes to the MQTT wildcard topic and routes events to Redis.
type Bridge struct {
	sub           Subscriber
	store         store.Store
	healthWriter  HealthWriter  // optional; nil when store doesn't implement it
	statsProvider StatsProvider // optional; nil when sub doesn't implement it
	log           *zap.Logger
	sweepInterval time.Duration
}

// New creates a Bridge with the default 10s sweep interval.
// Pass nil for log to use a no-op logger.
func New(sub Subscriber, s store.Store, log *zap.Logger) *Bridge {
	return NewWithSweepInterval(sub, s, log, defaultSweepInterval)
}

// NewWithSweepInterval creates a Bridge with a configurable sweep interval.
// Used in tests to accelerate the ticker.
func NewWithSweepInterval(sub Subscriber, s store.Store, log *zap.Logger, sweepInterval time.Duration) *Bridge {
	if log == nil {
		log = zap.NewNop()
	}
	b := &Bridge{sub: sub, store: s, log: log, sweepInterval: sweepInterval}
	// Wire optional interfaces by type assertion — keeps the bridge decoupled from concrete types.
	if hw, ok := s.(HealthWriter); ok {
		b.healthWriter = hw
	}
	if sp, ok := sub.(StatsProvider); ok {
		b.statsProvider = sp
	}
	return b
}

// Run blocks until ctx is cancelled. It subscribes to the MQTT wildcard topic with
// exponential-backoff retries (Bug 2 fix), starts worker goroutines to process events,
// and runs the cleanup + health-publish ticker.
func (b *Bridge) Run(ctx context.Context) {
	ch := make(chan envelope, chanCap)

	// Boot-time subscribe with exponential backoff. The OnConnectHandler (wired by
	// runBridge in main.go) handles mid-run reconnects — this loop only covers the
	// initial connect/subscribe attempt.
	backoff := retryInitial
	for {
		err := b.sub.Subscribe(ctx, func(topic string, payload []byte) {
			select {
			case ch <- envelope{topic: topic, payload: payload}:
			default:
				b.log.Warn("bridge channel full, dropping message", zap.String("topic", topic))
			}
		})
		if err == nil {
			b.log.Info("bridge subscribed to MQTT")
			break
		}
		b.log.Warn("bridge subscribe failed, retrying",
			zap.Error(err),
			zap.Duration("backoff", backoff))

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > retryCap {
			backoff = retryCap
		}
	}

	// Start N worker goroutines.
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case env := <-ch:
					b.handle(ctx, env)
				}
			}
		}()
	}

	// Cleanup + health-publish ticker.
	ticker := time.NewTicker(b.sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			cutoff := float64(time.Now().Add(-90 * time.Second).UnixMilli())
			n, err := b.store.SweepExpired(ctx, cutoff)
			if err != nil {
				b.log.Warn("sweep failed", zap.Error(err))
			} else if n > 0 {
				b.log.Debug("swept stale sessions", zap.Int("count", n))
			}
			b.writeMQTTHealth(ctx)
		}
	}
}

// writeMQTTHealth publishes the subscriber health state to Redis if both
// the health writer and stats provider are available.
func (b *Bridge) writeMQTTHealth(ctx context.Context) {
	if b.healthWriter == nil || b.statsProvider == nil {
		return
	}
	stats := b.statsProvider.Stats()
	h := store.MQTTHealthFields{
		Connected:   stats.Connected,
		Subscribed:  stats.Subscribed,
		MsgCount:    stats.MsgCount,
		LastMsgMs:   stats.LastMsgMs,
		StartedAtMs: stats.StartedAtMs,
		UpdatedAtMs: time.Now().UnixMilli(),
	}
	if err := b.healthWriter.WriteMQTTHealth(ctx, h); err != nil {
		b.log.Warn("bridge: write mqtt health failed", zap.Error(err))
	}
}

// handle routes a single MQTT envelope to the appropriate Store method.
func (b *Bridge) handle(ctx context.Context, env envelope) {
	// Topic format: claude/mesh/session/{sid}/{eventType}
	parts := strings.Split(env.topic, "/")
	if len(parts) < 5 {
		b.log.Warn("bridge: malformed topic", zap.String("topic", env.topic))
		return
	}
	sid := parts[3]
	eventType := parts[4]

	switch eventType {
	case "open":
		var ev events.SessionOpen
		if err := json.Unmarshal(env.payload, &ev); err != nil {
			b.log.Warn("bridge: malformed session-open payload",
				zap.String("sid", sid),
				zap.String("payload", truncate(string(env.payload), 200)),
				zap.Error(err))
			return
		}
		if err := b.store.OpenSession(ctx, ev); err != nil {
			b.log.Error("bridge: OpenSession failed", zap.String("sid", sid), zap.Error(err))
		}

	case "activity":
		var ev events.Activity
		if err := json.Unmarshal(env.payload, &ev); err != nil {
			b.log.Warn("bridge: malformed activity payload",
				zap.String("sid", sid),
				zap.String("payload", truncate(string(env.payload), 200)),
				zap.Error(err))
			return
		}
		if err := b.store.PushActivity(ctx, ev); err != nil {
			b.log.Error("bridge: PushActivity failed", zap.String("sid", sid), zap.Error(err))
			return
		}
		if err := b.store.TouchSession(ctx, sid, ev.Ts); err != nil {
			b.log.Debug("bridge: TouchSession failed (session may not exist)", zap.String("sid", sid), zap.Error(err))
		}

	case "close":
		var ev events.SessionClose
		if err := json.Unmarshal(env.payload, &ev); err != nil {
			b.log.Warn("bridge: malformed session-close payload",
				zap.String("sid", sid),
				zap.String("payload", truncate(string(env.payload), 200)),
				zap.Error(err))
			return
		}
		if err := b.store.CloseSession(ctx, sid); err != nil {
			b.log.Error("bridge: CloseSession failed", zap.String("sid", sid), zap.Error(err))
		}

	default:
		b.log.Debug("bridge: unknown event type", zap.String("sid", sid), zap.String("type", eventType))
	}
}

// truncate limits a string to maxLen bytes for safe log output.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
