package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/marioser/claude-mesh/internal/events"
)

// OpenSession writes a new session Hash, sets TTL, and adds to the active ZSET.
func (r *RedisStore) OpenSession(ctx context.Context, ev events.SessionOpen) error {
	key := SessionKey(ev.SessionID)
	now := ev.Ts
	if now == 0 {
		now = float64(time.Now().UnixMilli())
	}

	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, map[string]any{
		"session_id":      ev.SessionID,
		"cwd":             ev.Cwd,
		"git_branch":      ev.GitBranch,
		"host":            ev.Host,
		"pid":             ev.PID,
		"transcript_path": ev.TranscriptPath,
		"opened_at":       now,
		"last_seen":       now,
	})
	pipe.Expire(ctx, key, time.Duration(r.cfg.SessionTTL)*time.Second)
	pipe.ZAdd(ctx, activeSessions, redis.Z{Score: now, Member: ev.SessionID})

	_, err := pipe.Exec(ctx)
	return err
}

// TouchSession updates last_seen, resets TTL, and updates the ZSET score.
func (r *RedisStore) TouchSession(ctx context.Context, sid string, lastSeenMs float64) error {
	key := SessionKey(sid)
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, "last_seen", lastSeenMs)
	pipe.Expire(ctx, key, time.Duration(r.cfg.SessionTTL)*time.Second)
	pipe.ZAdd(ctx, activeSessions, redis.Z{Score: lastSeenMs, Member: sid})
	_, err := pipe.Exec(ctx)
	return err
}

// TouchOrCreateSession refreshes last_seen and the ZSET score, and additionally
// seeds the session_id / cwd / opened_at fields when the Hash does not exist
// (HSetNX — existing values are preserved). This is the activity-friendly
// variant used by the bridge when handling activity events: a resumed session
// or one whose Hash expired during the brief close-grace window will reappear
// in active listings on its next activity event instead of staying invisible.
func (r *RedisStore) TouchOrCreateSession(ctx context.Context, sid string, lastSeenMs float64, cwd string) error {
	key := SessionKey(sid)
	pipe := r.client.Pipeline()
	// HSetNX seeds identifying fields only when the Hash is absent.
	pipe.HSetNX(ctx, key, "session_id", sid)
	pipe.HSetNX(ctx, key, "cwd", cwd)
	pipe.HSetNX(ctx, key, "opened_at", lastSeenMs)
	// HSet always refreshes last_seen for both existing and freshly seeded sessions.
	pipe.HSet(ctx, key, "last_seen", lastSeenMs)
	pipe.Expire(ctx, key, time.Duration(r.cfg.SessionTTL)*time.Second)
	pipe.ZAdd(ctx, activeSessions, redis.Z{Score: lastSeenMs, Member: sid})
	_, err := pipe.Exec(ctx)
	return err
}

// CloseSession removes the session from the ZSET and sets a short EXPIRE (grace period).
// The Hash is NOT deleted immediately so in-flight reads still succeed.
func (r *RedisStore) CloseSession(ctx context.Context, sid string) error {
	const graceSeconds = 5
	key := SessionKey(sid)
	pipe := r.client.Pipeline()
	pipe.ZRem(ctx, activeSessions, sid)
	pipe.Expire(ctx, key, graceSeconds*time.Second)
	_, err := pipe.Exec(ctx)
	return err
}

// GetSession fetches session metadata from the Hash. Returns nil if not found.
func (r *RedisStore) GetSession(ctx context.Context, sid string) (*SessionView, error) {
	key := SessionKey(sid)
	result, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("store.GetSession: %w", err)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return sessionViewFromHash(sid, result), nil
}

// ListActiveSessions returns all members of the active ZSET with their Hash data.
func (r *RedisStore) ListActiveSessions(ctx context.Context) ([]SessionView, error) {
	members, err := r.client.ZRangeWithScores(ctx, activeSessions, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("store.ListActiveSessions: %w", err)
	}

	views := make([]SessionView, 0, len(members))
	for _, z := range members {
		sid := z.Member.(string)
		view, err := r.GetSession(ctx, sid)
		if err != nil || view == nil {
			continue // session expired between ZRANGE and HGetAll — skip it
		}
		views = append(views, *view)
	}
	return views, nil
}

// TouchActiveSessions resets the ZSET score for every existing active-session member
// to nowMs. Call this at bridge boot to prevent the sweep ticker from evicting sessions
// that were alive before a restart but haven't emitted a fresh activity event yet.
// Only existing ZSET members are updated (ZADD XX); no new entries are created.
func (r *RedisStore) TouchActiveSessions(ctx context.Context, nowMs float64) (int, error) {
	members, err := r.client.ZRange(ctx, activeSessions, 0, -1).Result()
	if err != nil {
		return 0, fmt.Errorf("store.TouchActiveSessions ZRange: %w", err)
	}
	if len(members) == 0 {
		return 0, nil
	}
	zs := make([]redis.Z, len(members))
	for i, sid := range members {
		zs[i] = redis.Z{Score: nowMs, Member: sid}
	}
	// XX: only update existing members, never add new ones.
	n, err := r.client.ZAddArgs(ctx, activeSessions, redis.ZAddArgs{
		XX:      true,
		Members: zs,
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("store.TouchActiveSessions ZAdd: %w", err)
	}
	return int(n), nil
}

// SweepExpired removes ZSET members with score < cutoffMs. Returns count removed.
func (r *RedisStore) SweepExpired(ctx context.Context, cutoffMs float64) (int, error) {
	n, err := r.client.ZRemRangeByScore(ctx, activeSessions, "-inf", fmt.Sprintf("%f", cutoffMs)).Result()
	if err != nil {
		return 0, fmt.Errorf("store.SweepExpired: %w", err)
	}
	return int(n), nil
}

// sessionViewFromHash converts a Redis HGetAll result map to a SessionView.
func sessionViewFromHash(sid string, h map[string]string) *SessionView {
	view := &SessionView{
		ID:             sid,
		Cwd:            h["cwd"],
		GitBranch:      h["git_branch"],
		Host:           h["host"],
		TranscriptPath: h["transcript_path"],
	}
	if pid, err := strconv.Atoi(h["pid"]); err == nil {
		view.PID = pid
	}
	if v, err := strconv.ParseFloat(h["opened_at"], 64); err == nil {
		view.OpenedAtMs = v
	}
	if v, err := strconv.ParseFloat(h["last_seen"], 64); err == nil {
		view.LastSeenMs = v
	}
	return view
}
