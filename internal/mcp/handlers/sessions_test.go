package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/marioser/claude-mesh/internal/mcp/handlers"
	"github.com/marioser/claude-mesh/internal/store"
)

// TestSessionsListActive verifies that mesh_active_sessions returns all active sessions.
func TestSessionsListActive(t *testing.T) {
	s := &fakeStore{
		healthy: true,
		sessions: []store.SessionView{
			{ID: "s1", Cwd: "/p1", Host: "h1", LastSeenMs: float64(time.Now().UnixMilli())},
			{ID: "s2", Cwd: "/p2", Host: "h2", LastSeenMs: float64(time.Now().UnixMilli())},
			{ID: "s3", Cwd: "/p3", Host: "h3", LastSeenMs: float64(time.Now().UnixMilli())},
		},
	}

	handler := handlers.NewSessionsHandler(s, 100)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("SessionsHandler: %v", err)
	}

	m := parseResult(t, result)
	sessionsRaw, _ := m["sessions"].([]any)
	if len(sessionsRaw) != 3 {
		t.Errorf("sessions count: got %d, want 3", len(sessionsRaw))
	}
}

// TestSessionsEmpty verifies that an empty ZSET returns {sessions:[]}.
func TestSessionsEmpty(t *testing.T) {
	s := &fakeStore{healthy: true, sessions: []store.SessionView{}}

	handler := handlers.NewSessionsHandler(s, 100)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("SessionsHandler empty: %v", err)
	}

	m := parseResult(t, result)
	sessionsRaw, _ := m["sessions"].([]any)
	if len(sessionsRaw) != 0 {
		t.Errorf("sessions: got %d, want 0", len(sessionsRaw))
	}
}
