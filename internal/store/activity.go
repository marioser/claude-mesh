package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"claude-mesh/internal/events"
)

// PushActivity appends an activity event to per-session and global ring buffers,
// registers the session in the active ZSET, and refreshes the session Hash TTL.
// This allows sessions that never sent a SessionOpen event to become visible in
// mesh_status after their first tool call.
//
// Per-session ring is only written when the session Hash already exists (or was
// just auto-registered). Global ring always receives the event.
func (r *RedisStore) PushActivity(ctx context.Context, ev events.Activity) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("store.PushActivity marshal: %w", err)
	}

	sessKey := SessionKey(ev.SessionID)
	sessionTTL := time.Duration(r.cfg.SessionTTL) * time.Second

	// Check whether the session Hash exists before opening the pipeline.
	existsResult, err := r.client.Exists(ctx, sessKey).Result()
	if err != nil {
		return fmt.Errorf("store.PushActivity exists: %w", err)
	}
	hashExists := existsResult > 0

	pipe := r.client.Pipeline()

	if hashExists {
		// Session already open: write per-session ring and update last_seen.
		actKey := ActivityKey(ev.SessionID)
		ringSize := int64(r.cfg.ActivityRingSize - 1)
		pipe.LPush(ctx, actKey, data)
		pipe.LTrim(ctx, actKey, 0, ringSize)
		pipe.Expire(ctx, actKey, time.Duration(r.cfg.ActivityPerSessTTL)*time.Second)
		pipe.HSet(ctx, sessKey, "last_seen", ev.Ts)
	} else {
		// Session not yet open (e.g., hook fired before session-open reached broker).
		// Create a minimal Hash so ListActiveSessions can surface this session.
		pipe.HSet(ctx, sessKey, map[string]any{
			"session_id": ev.SessionID,
			"cwd":        ev.Cwd,
			"last_seen":  ev.Ts,
		})
	}

	// Always refresh session Hash TTL and register/update in the active ZSET.
	pipe.Expire(ctx, sessKey, sessionTTL)
	pipe.ZAdd(ctx, activeSessions, redis.Z{Score: ev.Ts, Member: ev.SessionID})

	// Always append to global ring.
	globalRingSize := int64(r.cfg.GlobalRingSize - 1)
	pipe.LPush(ctx, activityGlobalKey, data)
	pipe.LTrim(ctx, activityGlobalKey, 0, globalRingSize)
	pipe.Expire(ctx, activityGlobalKey, time.Duration(r.cfg.ActivityGlobalTTL)*time.Second)

	_, err = pipe.Exec(ctx)
	return err
}

// RecentActivity returns up to limit activity events, newest first.
// If sid is empty, reads from the global ring; otherwise from the per-session ring.
func (r *RedisStore) RecentActivity(ctx context.Context, limit int, sid string) ([]events.Activity, error) {
	var key string
	if sid == "" {
		key = activityGlobalKey
	} else {
		key = ActivityKey(sid)
	}

	raw, err := r.client.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("store.RecentActivity: %w", err)
	}

	result := make([]events.Activity, 0, len(raw))
	for _, item := range raw {
		var act events.Activity
		if err := json.Unmarshal([]byte(item), &act); err != nil {
			continue // skip malformed entries
		}
		result = append(result, act)
	}
	return result, nil
}
