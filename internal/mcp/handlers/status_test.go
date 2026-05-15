package handlers_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"claude-mesh/internal/events"
	"claude-mesh/internal/mcp/handlers"
	"claude-mesh/internal/store"
)

// fakeStore is a test double for store.Store.
type fakeStore struct {
	sessions []store.SessionView
	activity []events.Activity
	healthy  bool
}

func (f *fakeStore) OpenSession(_ context.Context, _ events.SessionOpen) error { return nil }
func (f *fakeStore) TouchSession(_ context.Context, _ string, _ float64) error { return nil }
func (f *fakeStore) CloseSession(_ context.Context, _ string) error            { return nil }
func (f *fakeStore) GetSession(_ context.Context, _ string) (*store.SessionView, error) {
	return nil, nil
}
func (f *fakeStore) ListActiveSessions(_ context.Context) ([]store.SessionView, error) {
	if !f.healthy {
		return nil, context.DeadlineExceeded
	}
	return f.sessions, nil
}
func (f *fakeStore) PushActivity(_ context.Context, _ events.Activity) error { return nil }
func (f *fakeStore) RecentActivity(_ context.Context, limit int, sid string) ([]events.Activity, error) {
	if limit > len(f.activity) {
		return f.activity, nil
	}
	return f.activity[:limit], nil
}
func (f *fakeStore) SweepExpired(_ context.Context, _ float64) (int, error)         { return 0, nil }
func (f *fakeStore) TouchActiveSessions(_ context.Context, _ float64) (int, error) { return 0, nil }
func (f *fakeStore) HealthCheck(_ context.Context) error {
	if !f.healthy {
		return context.DeadlineExceeded
	}
	return nil
}
func (f *fakeStore) GetString(_ context.Context, _ string) (string, error) {
	return "", context.DeadlineExceeded
}
func (f *fakeStore) GetInt(_ context.Context, _ string) (int, error) {
	return 0, context.DeadlineExceeded
}
func (f *fakeStore) GetFloat(_ context.Context, _ string) (float64, error) {
	return 0, context.DeadlineExceeded
}

var _ store.Store = (*fakeStore)(nil)

// parseResult extracts the text content from an MCP tool result as a map.
func parseResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		t.Fatalf("unmarshal result: %v (text: %s)", err, text.Text)
	}
	return m
}

// TestStatusActiveSessions verifies that mesh_status returns correct active_count and sessions.
func TestStatusActiveSessions(t *testing.T) {
	s := &fakeStore{
		healthy: true,
		sessions: []store.SessionView{
			{ID: "s1", Cwd: "/p1", GitBranch: "main", Host: "h1", LastSeenMs: float64(time.Now().UnixMilli())},
			{ID: "s2", Cwd: "/p2", GitBranch: "dev", Host: "h2", LastSeenMs: float64(time.Now().UnixMilli())},
		},
	}

	handler := handlers.NewStatusHandler(s, 100)
	req := mcp.CallToolRequest{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("StatusHandler: %v", err)
	}

	m := parseResult(t, result)

	count, _ := m["active_count"].(float64)
	if int(count) != 2 {
		t.Errorf("active_count: got %v, want 2", count)
	}

	sessionsRaw, ok := m["sessions"].([]any)
	if !ok {
		t.Fatalf("sessions field missing or wrong type: %T", m["sessions"])
	}
	if len(sessionsRaw) != 2 {
		t.Errorf("sessions length: got %d, want 2", len(sessionsRaw))
	}
}

// TestStatusNoSessions verifies that mesh_status returns {active_count:0, sessions:[]} when empty.
func TestStatusNoSessions(t *testing.T) {
	s := &fakeStore{healthy: true, sessions: []store.SessionView{}}

	handler := handlers.NewStatusHandler(s, 100)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("StatusHandler: %v", err)
	}

	m := parseResult(t, result)
	count, _ := m["active_count"].(float64)
	if int(count) != 0 {
		t.Errorf("active_count: got %v, want 0", count)
	}
	sessions, _ := m["sessions"].([]any)
	if len(sessions) != 0 {
		t.Errorf("sessions: got %d, want 0", len(sessions))
	}
}

// TestStatusMaxAgeFilter verifies that mesh_status filters sessions older than max_age_seconds.
func TestStatusMaxAgeFilter(t *testing.T) {
	nowMs := float64(time.Now().UnixMilli())
	recentMs := nowMs - 10_000  // 10 seconds ago
	staleMs := nowMs - 120_000  // 120 seconds ago (older than 60s filter)

	s := &fakeStore{
		healthy: true,
		sessions: []store.SessionView{
			{ID: "recent", Cwd: "/recent", GitBranch: "main", Host: "h1", LastSeenMs: recentMs},
			{ID: "stale", Cwd: "/stale", GitBranch: "main", Host: "h2", LastSeenMs: staleMs},
		},
	}

	handler := handlers.NewStatusHandler(s, 100)
	req := mcp.CallToolRequest{}
	// Pass max_age_seconds = 60: only sessions seen in the last 60s should appear.
	req.Params.Arguments = map[string]any{
		"max_age_seconds": float64(60),
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("StatusHandler with max_age_seconds: %v", err)
	}

	m := parseResult(t, result)

	count, _ := m["active_count"].(float64)
	if int(count) != 1 {
		t.Errorf("active_count: got %v, want 1 (stale session filtered out)", count)
	}

	sessionsRaw, ok := m["sessions"].([]any)
	if !ok || len(sessionsRaw) != 1 {
		t.Fatalf("sessions: got %v, want 1 entry", m["sessions"])
	}

	sess, _ := sessionsRaw[0].(map[string]any)
	if sid, _ := sess["session_id"].(string); sid != "recent" {
		t.Errorf("expected session_id 'recent', got %q", sid)
	}
}

// TestStatusMaxAgeNoFilter verifies that without max_age_seconds all sessions are returned.
func TestStatusMaxAgeNoFilter(t *testing.T) {
	nowMs := float64(time.Now().UnixMilli())
	s := &fakeStore{
		healthy: true,
		sessions: []store.SessionView{
			{ID: "s1", LastSeenMs: nowMs - 5_000},
			{ID: "s2", LastSeenMs: nowMs - 200_000}, // very old
		},
	}

	handler := handlers.NewStatusHandler(s, 100)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("StatusHandler: %v", err)
	}

	m := parseResult(t, result)
	count, _ := m["active_count"].(float64)
	if int(count) != 2 {
		t.Errorf("active_count without filter: got %v, want 2", count)
	}
}

// TestStatusRedisTimeout verifies that a Redis timeout results in degraded:true and empty sessions.
func TestStatusRedisTimeout(t *testing.T) {
	s := &fakeStore{healthy: false} // HealthCheck returns error, ListActiveSessions also errors

	// Override to simulate timeout.
	s.sessions = nil // won't be used because store returns degraded

	handler := handlers.NewStatusHandler(s, 100)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("StatusHandler with degraded store: %v", err)
	}

	m := parseResult(t, result)
	degraded, _ := m["degraded"].(bool)
	if !degraded {
		t.Error("expected degraded:true when Redis is unavailable")
	}
}
