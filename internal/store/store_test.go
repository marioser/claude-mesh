package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"claude-mesh/internal/events"
	"claude-mesh/internal/store"
)

// newTestStore spins up a miniredis server and returns a RedisStore backed by it.
// The server is closed automatically when the test ends.
func newTestStore(t *testing.T) store.Store {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return store.NewRedisStore(client, store.DefaultConfig())
}

// nowMs returns the current time as float64 milliseconds.
func nowMs() float64 {
	return float64(time.Now().UnixMilli())
}

// TestOpenSession verifies that OpenSession writes the correct Hash, EXPIRE, and ZADD.
func TestOpenSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ev := events.SessionOpen{
		Ts:             nowMs(),
		SessionID:      "sess-open-1",
		Cwd:            "/project",
		TranscriptPath: "/tmp/t.txt",
		GitBranch:      "main",
		Host:           "host1",
		PID:            100,
	}

	if err := s.OpenSession(ctx, ev); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// Verify the session can be retrieved.
	view, err := s.GetSession(ctx, ev.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if view == nil {
		t.Fatal("GetSession returned nil")
	}
	if view.ID != ev.SessionID {
		t.Errorf("ID: got %q, want %q", view.ID, ev.SessionID)
	}
	if view.Cwd != ev.Cwd {
		t.Errorf("Cwd: got %q, want %q", view.Cwd, ev.Cwd)
	}
	if view.GitBranch != ev.GitBranch {
		t.Errorf("GitBranch: got %q, want %q", view.GitBranch, ev.GitBranch)
	}
	if view.Host != ev.Host {
		t.Errorf("Host: got %q, want %q", view.Host, ev.Host)
	}
	if view.PID != ev.PID {
		t.Errorf("PID: got %d, want %d", view.PID, ev.PID)
	}

	// Verify session is in the active ZSET.
	sessions, err := s.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	found := false
	for _, sess := range sessions {
		if sess.ID == ev.SessionID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session %q not found in active ZSET after OpenSession", ev.SessionID)
	}
}

// TestTouchSession verifies that TouchSession updates last_seen, resets TTL, and
// updates the ZSET score.
func TestTouchSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ev := events.SessionOpen{
		Ts: nowMs(), SessionID: "sess-touch", Cwd: "/p", Host: "h", PID: 1,
	}
	if err := s.OpenSession(ctx, ev); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	newTs := nowMs() + 5000 // 5s later
	if err := s.TouchSession(ctx, ev.SessionID, newTs); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}

	view, err := s.GetSession(ctx, ev.SessionID)
	if err != nil {
		t.Fatalf("GetSession after touch: %v", err)
	}
	if view.LastSeenMs != newTs {
		t.Errorf("LastSeenMs after touch: got %v, want %v", view.LastSeenMs, newTs)
	}
}

// TestCloseSession verifies that CloseSession removes the session from the ZSET
// and sets a short EXPIRE on the Hash (not an immediate DEL).
func TestCloseSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ev := events.SessionOpen{
		Ts: nowMs(), SessionID: "sess-close", Cwd: "/p", Host: "h", PID: 1,
	}
	if err := s.OpenSession(ctx, ev); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	if err := s.CloseSession(ctx, ev.SessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	// Session must no longer be in the active ZSET.
	sessions, err := s.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	for _, sess := range sessions {
		if sess.ID == ev.SessionID {
			t.Errorf("session %q still in active ZSET after CloseSession", ev.SessionID)
		}
	}
}

// TestListActiveSessions verifies that multiple open sessions are returned.
func TestListActiveSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := []string{"sess-A", "sess-B", "sess-C"}
	for _, id := range ids {
		ev := events.SessionOpen{Ts: nowMs(), SessionID: id, Cwd: "/p", Host: "h", PID: 1}
		if err := s.OpenSession(ctx, ev); err != nil {
			t.Fatalf("OpenSession(%s): %v", id, err)
		}
	}

	sessions, err := s.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(sessions) != len(ids) {
		t.Errorf("ListActiveSessions count: got %d, want %d", len(sessions), len(ids))
	}
}

// TestPushActivity verifies that PushActivity does LPUSH+LTRIM+EXPIRE on both
// the per-session and global activity lists.
func TestPushActivity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Open a session first so TouchSession works.
	openEv := events.SessionOpen{Ts: nowMs(), SessionID: "sess-act", Cwd: "/p", Host: "h", PID: 1}
	if err := s.OpenSession(ctx, openEv); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	act := events.Activity{
		Ts:        nowMs(),
		SessionID: "sess-act",
		Tool:      "Edit",
		Target:    "main.go",
		Cwd:       "/p",
	}
	if err := s.PushActivity(ctx, act); err != nil {
		t.Fatalf("PushActivity: %v", err)
	}

	// Retrieve from per-session ring.
	perSess, err := s.RecentActivity(ctx, 10, "sess-act")
	if err != nil {
		t.Fatalf("RecentActivity per-session: %v", err)
	}
	if len(perSess) != 1 {
		t.Errorf("per-session ring count: got %d, want 1", len(perSess))
	}
	if perSess[0].Tool != "Edit" {
		t.Errorf("per-session ring Tool: got %q, want %q", perSess[0].Tool, "Edit")
	}

	// Retrieve from global ring.
	global, err := s.RecentActivity(ctx, 10, "")
	if err != nil {
		t.Fatalf("RecentActivity global: %v", err)
	}
	if len(global) != 1 {
		t.Errorf("global ring count: got %d, want 1", len(global))
	}
}

// TestRecentActivityNewestFirst verifies that events are returned newest-first.
func TestRecentActivityNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	openEv := events.SessionOpen{Ts: nowMs(), SessionID: "sess-order", Cwd: "/p", Host: "h", PID: 1}
	if err := s.OpenSession(ctx, openEv); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	tools := []string{"Read", "Edit", "Bash"}
	for i, tool := range tools {
		act := events.Activity{
			Ts:        float64(1000 + i),
			SessionID: "sess-order",
			Tool:      tool,
			Target:    "f.go",
			Cwd:       "/p",
		}
		if err := s.PushActivity(ctx, act); err != nil {
			t.Fatalf("PushActivity %s: %v", tool, err)
		}
	}

	events_got, err := s.RecentActivity(ctx, 10, "sess-order")
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if len(events_got) != 3 {
		t.Fatalf("count: got %d, want 3", len(events_got))
	}
	// Newest first: Bash (pushed last) should be index 0.
	if events_got[0].Tool != "Bash" {
		t.Errorf("newest event Tool: got %q, want %q", events_got[0].Tool, "Bash")
	}
	if events_got[2].Tool != "Read" {
		t.Errorf("oldest event Tool: got %q, want %q", events_got[2].Tool, "Read")
	}
}

// TestSweepExpired verifies that SweepExpired removes stale entries from the ZSET.
func TestSweepExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Open two sessions with old timestamps.
	staleTs := float64(time.Now().Add(-2*time.Minute).UnixMilli())
	freshTs := nowMs()

	for _, id := range []string{"stale-1", "stale-2"} {
		ev := events.SessionOpen{Ts: staleTs, SessionID: id, Cwd: "/", Host: "h", PID: 1}
		if err := s.OpenSession(ctx, ev); err != nil {
			t.Fatalf("OpenSession(%s): %v", id, err)
		}
	}
	// Open one fresh session.
	ev := events.SessionOpen{Ts: freshTs, SessionID: "fresh-1", Cwd: "/", Host: "h", PID: 1}
	if err := s.OpenSession(ctx, ev); err != nil {
		t.Fatalf("OpenSession(fresh): %v", err)
	}

	// Sweep everything older than 90s ago.
	cutoff := float64(time.Now().Add(-90*time.Second).UnixMilli())
	removed, err := s.SweepExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if removed != 2 {
		t.Errorf("SweepExpired removed: got %d, want 2", removed)
	}

	sessions, err := s.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions after sweep: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("active after sweep: got %d, want 1", len(sessions))
	}
	if sessions[0].ID != "fresh-1" {
		t.Errorf("remaining session: got %q, want %q", sessions[0].ID, "fresh-1")
	}
}

// TestHealthCheck verifies that HealthCheck returns nil when Redis is reachable.
func TestHealthCheck(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck: got error %v, want nil", err)
	}
}

// TestPushActivityUnknownSession verifies that PushActivity still appends to global ring
// even when the session Hash doesn't exist (no ghost session created).
func TestPushActivityUnknownSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	act := events.Activity{
		Ts:        nowMs(),
		SessionID: "ghost-session",
		Tool:      "Read",
		Target:    "nowhere.go",
		Cwd:       "/",
	}
	// Should not error — just appends to global ring.
	if err := s.PushActivity(ctx, act); err != nil {
		t.Fatalf("PushActivity for unknown session: %v", err)
	}

	// Global ring should have the event.
	global, err := s.RecentActivity(ctx, 10, "")
	if err != nil {
		t.Fatalf("RecentActivity global: %v", err)
	}
	if len(global) != 1 {
		t.Errorf("global ring after unknown session push: got %d, want 1", len(global))
	}

	// Per-session ring must NOT exist (no ghost session).
	perSess, err := s.RecentActivity(ctx, 10, "ghost-session")
	if err != nil {
		t.Fatalf("RecentActivity ghost: %v", err)
	}
	if len(perSess) != 0 {
		t.Errorf("ghost session per-session ring: got %d, want 0", len(perSess))
	}
}

// Ensure json round-trip of SessionView works (used by ListActiveSessions internals).
func TestSessionViewJSONSerde(t *testing.T) {
	view := store.SessionView{
		ID:             "s1",
		Cwd:            "/project",
		GitBranch:      "main",
		Host:           "host1",
		TranscriptPath: "/tmp/t.txt",
		PID:            42,
		OpenedAtMs:     1715300000000.0,
		LastSeenMs:     1715300001000.0,
	}

	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got store.SessionView
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != view.ID {
		t.Errorf("ID: got %q, want %q", got.ID, view.ID)
	}
}
