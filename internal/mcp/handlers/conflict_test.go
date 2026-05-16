package handlers_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/mcp/handlers"
)

// conflictFakeStore embeds fakeStore but overrides RecentActivity for conflict tests.
type conflictFakeStore struct {
	fakeStore
	globalActivity []events.Activity
}

func (f *conflictFakeStore) RecentActivity(_ context.Context, limit int, sid string) ([]events.Activity, error) {
	if sid != "" {
		return nil, nil
	}
	if limit > len(f.globalActivity) {
		return f.globalActivity, nil
	}
	return f.globalActivity[:limit], nil
}

// TestConflictFileTouchedByOtherSession verifies that mesh_check_conflict returns
// sessions that have the given file in their activity ring, excluding the caller.
func TestConflictFileTouchedByOtherSession(t *testing.T) {
	globalRing := []events.Activity{
		{Ts: float64(time.Now().UnixMilli()), SessionID: "other-sess", Tool: "Edit", Target: "scripts/claude-mesh/main.go", Cwd: "/p"},
		{Ts: float64(time.Now().UnixMilli()), SessionID: "caller-sess", Tool: "Edit", Target: "scripts/claude-mesh/main.go", Cwd: "/p"},
		{Ts: float64(time.Now().UnixMilli()), SessionID: "other-sess", Tool: "Read", Target: "other/file.go", Cwd: "/p"},
	}

	s := &conflictFakeStore{
		fakeStore:      fakeStore{healthy: true},
		globalActivity: globalRing,
	}

	handler := handlers.NewConflictHandler(s, 100)
	rawReq := mcp.CallToolRequest{}
	rawReq.Params.Arguments = map[string]any{
		"file_path":  "scripts/claude-mesh/main.go",
		"session_id": "caller-sess",
	}

	result, err := handler(context.Background(), rawReq)
	if err != nil {
		t.Fatalf("ConflictHandler: %v", err)
	}

	m := parseResult(t, result)
	conflicts, _ := m["conflicts"].([]any)

	// Should have exactly 1 conflict: other-sess (caller-sess excluded).
	if len(conflicts) != 1 {
		t.Errorf("conflicts count: got %d, want 1", len(conflicts))
	}
	if len(conflicts) > 0 {
		c, _ := conflicts[0].(map[string]any)
		sid, _ := c["session_id"].(string)
		if sid != "other-sess" {
			t.Errorf("conflict session_id: got %q, want %q", sid, "other-sess")
		}
	}
}

// TestConflictNoConflict verifies that {conflicts:[]} is returned when no other session
// has touched the given file.
func TestConflictNoConflict(t *testing.T) {
	globalRing := []events.Activity{
		{Ts: float64(time.Now().UnixMilli()), SessionID: "other-sess", Tool: "Edit", Target: "completely/different.go", Cwd: "/p"},
	}

	s := &conflictFakeStore{
		fakeStore:      fakeStore{healthy: true},
		globalActivity: globalRing,
	}

	handler := handlers.NewConflictHandler(s, 100)
	rawReq := mcp.CallToolRequest{}
	rawReq.Params.Arguments = map[string]any{
		"file_path":  "scripts/claude-mesh/main.go",
		"session_id": "caller-sess",
	}

	result, err := handler(context.Background(), rawReq)
	if err != nil {
		t.Fatalf("ConflictHandler: %v", err)
	}

	m := parseResult(t, result)
	conflictsRaw := m["conflicts"]

	// Must be present and empty.
	conflicts, _ := conflictsRaw.([]any)
	if len(conflicts) != 0 {
		t.Errorf("conflicts: got %d entries, want 0", len(conflicts))
	}

	// Explicitly check the field exists.
	if _, ok := m["conflicts"]; !ok {
		t.Error("response missing 'conflicts' field")
	}

	// Verify raw JSON.
	text := result.Content[0].(mcp.TextContent).Text
	var raw map[string]json.RawMessage
	_ = json.Unmarshal([]byte(text), &raw)
	if _, ok := raw["conflicts"]; !ok {
		t.Error("JSON response missing 'conflicts' key")
	}
}
