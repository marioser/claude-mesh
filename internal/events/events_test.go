package events_test

import (
	"encoding/json"
	"testing"

	"claude-mesh/internal/events"
)

// TestSessionOpenRoundTrip verifies that SessionOpen marshals to and from JSON
// preserving all required MQTT schema fields.
func TestSessionOpenRoundTrip(t *testing.T) {
	orig := events.SessionOpen{
		Ts:             1715300000123.456,
		SessionID:      "sess-abc",
		Cwd:            "/Users/dev/project",
		TranscriptPath: "/tmp/transcript.txt",
		GitBranch:      "feature/SMBX-177",
		Host:           "macbook.local",
		PID:            42,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal SessionOpen: %v", err)
	}

	var got events.SessionOpen
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal SessionOpen: %v", err)
	}

	if got.Ts != orig.Ts {
		t.Errorf("Ts: got %v, want %v", got.Ts, orig.Ts)
	}
	if got.SessionID != orig.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, orig.SessionID)
	}
	if got.Cwd != orig.Cwd {
		t.Errorf("Cwd: got %q, want %q", got.Cwd, orig.Cwd)
	}
	if got.TranscriptPath != orig.TranscriptPath {
		t.Errorf("TranscriptPath: got %q, want %q", got.TranscriptPath, orig.TranscriptPath)
	}
	if got.GitBranch != orig.GitBranch {
		t.Errorf("GitBranch: got %q, want %q", got.GitBranch, orig.GitBranch)
	}
	if got.Host != orig.Host {
		t.Errorf("Host: got %q, want %q", got.Host, orig.Host)
	}
	if got.PID != orig.PID {
		t.Errorf("PID: got %d, want %d", got.PID, orig.PID)
	}
}

// TestSessionCloseRoundTrip verifies SessionClose round-trip with required reason field.
func TestSessionCloseRoundTrip(t *testing.T) {
	orig := events.SessionClose{
		Ts:        1715300000999.0,
		SessionID: "sess-xyz",
		Reason:    "stop",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal SessionClose: %v", err)
	}

	var got events.SessionClose
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal SessionClose: %v", err)
	}

	if got.Ts != orig.Ts {
		t.Errorf("Ts: got %v, want %v", got.Ts, orig.Ts)
	}
	if got.SessionID != orig.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, orig.SessionID)
	}
	if got.Reason != orig.Reason {
		t.Errorf("Reason: got %q, want %q", got.Reason, orig.Reason)
	}
}

// TestActivityRoundTrip verifies Activity round-trip preserving all required fields.
func TestActivityRoundTrip(t *testing.T) {
	orig := events.Activity{
		Ts:        1715300001234.789,
		SessionID: "sess-def",
		Tool:      "Edit",
		Target:    "internal/store/store.go",
		Cwd:       "/Users/dev/project",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal Activity: %v", err)
	}

	var got events.Activity
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal Activity: %v", err)
	}

	if got.Ts != orig.Ts {
		t.Errorf("Ts: got %v, want %v", got.Ts, orig.Ts)
	}
	if got.SessionID != orig.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, orig.SessionID)
	}
	if got.Tool != orig.Tool {
		t.Errorf("Tool: got %q, want %q", got.Tool, orig.Tool)
	}
	if got.Target != orig.Target {
		t.Errorf("Target: got %q, want %q", got.Target, orig.Target)
	}
	if got.Cwd != orig.Cwd {
		t.Errorf("Cwd: got %q, want %q", got.Cwd, orig.Cwd)
	}
}

// TestBuildSessionTopic verifies topic construction for different event types.
func TestBuildSessionTopic(t *testing.T) {
	cases := []struct {
		sid       string
		eventType string
		want      string
	}{
		{"sess-abc", "open", "claude/mesh/session/sess-abc/open"},
		{"sess-abc", "activity", "claude/mesh/session/sess-abc/activity"},
		{"sess-abc", "close", "claude/mesh/session/sess-abc/close"},
	}

	for _, tc := range cases {
		got := events.BuildSessionTopic(tc.sid, tc.eventType)
		if got != tc.want {
			t.Errorf("BuildSessionTopic(%q, %q) = %q, want %q", tc.sid, tc.eventType, got, tc.want)
		}
	}
}

// TestJSONFieldNames verifies the JSON field names match the MQTT schema (snake_case).
func TestJSONFieldNames(t *testing.T) {
	ev := events.SessionOpen{
		Ts:             1715300000000.0,
		SessionID:      "s",
		Cwd:            "/",
		TranscriptPath: "/t",
		GitBranch:      "main",
		Host:           "h",
		PID:            1,
	}
	data, _ := json.Marshal(ev)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"ts", "session_id", "cwd", "transcript_path", "git_branch", "host", "pid"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected JSON field %q not found in: %s", key, data)
		}
	}
}
