package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"claude-mesh/internal/events"
	"claude-mesh/internal/mcp/handlers"
)

type recentFakeStore struct {
	fakeStore
	activity []events.Activity
}

func (f *recentFakeStore) RecentActivity(_ context.Context, limit int, _ string) ([]events.Activity, error) {
	if limit > len(f.activity) {
		return f.activity, nil
	}
	return f.activity[:limit], nil
}

// TestRecentActivityWithinWindow verifies that events within the minutes window are returned.
func TestRecentActivityWithinWindow(t *testing.T) {
	now := float64(time.Now().UnixMilli())
	activity := []events.Activity{
		{Ts: now - 60000, SessionID: "s1", Tool: "Edit", Target: "f.go", Cwd: "/p"},
		{Ts: now - 120000, SessionID: "s2", Tool: "Read", Target: "g.go", Cwd: "/p"},
		{Ts: now - 400000, SessionID: "s3", Tool: "Bash", Target: "cmd", Cwd: "/p"}, // > 5 minutes ago
	}

	s := &recentFakeStore{fakeStore: fakeStore{healthy: true}, activity: activity}

	handler := handlers.NewRecentHandler(s, 100)
	rawReq := mcp.CallToolRequest{}
	rawReq.Params.Arguments = map[string]any{
		"minutes": float64(5),
	}

	result, err := handler(context.Background(), rawReq)
	if err != nil {
		t.Fatalf("RecentHandler: %v", err)
	}

	m := parseResult(t, result)
	eventsRaw, _ := m["events"].([]any)

	// Only the 2 events within 5 minutes should appear.
	if len(eventsRaw) != 2 {
		t.Errorf("events count: got %d, want 2", len(eventsRaw))
	}
}

// TestRecentActivityEmptyBuffer verifies that {events:[]} is returned for empty buffer.
func TestRecentActivityEmptyBuffer(t *testing.T) {
	s := &recentFakeStore{fakeStore: fakeStore{healthy: true}, activity: nil}

	handler := handlers.NewRecentHandler(s, 100)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("RecentHandler empty: %v", err)
	}

	m := parseResult(t, result)
	eventsRaw, _ := m["events"].([]any)
	if len(eventsRaw) != 0 {
		t.Errorf("events: got %d, want 0", len(eventsRaw))
	}
}
