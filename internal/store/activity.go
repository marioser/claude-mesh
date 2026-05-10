package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"claude-mesh/internal/events"
)

// PushActivity appends an activity event to per-session and global ring buffers.
// Per-session ring is only written when the session Hash exists.
// Global ring always receives the event.
func (r *RedisStore) PushActivity(ctx context.Context, ev events.Activity) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("store.PushActivity marshal: %w", err)
	}

	pipe := r.client.Pipeline()

	// Check if session exists before writing to per-session ring.
	existsCmd := r.client.Exists(ctx, SessionKey(ev.SessionID))
	existsResult, err := existsCmd.Result()
	if err == nil && existsResult > 0 {
		sessKey := ActivityKey(ev.SessionID)
		ringSize := int64(r.cfg.ActivityRingSize - 1)
		pipe.LPush(ctx, sessKey, data)
		pipe.LTrim(ctx, sessKey, 0, ringSize)
		pipe.Expire(ctx, sessKey, time.Duration(r.cfg.ActivityPerSessTTL)*time.Second)
	}

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
