// Package events defines typed event structs for the Claude Mesh MQTT protocol.
// All timestamps (Ts) are float64 milliseconds since epoch with 3 decimal places.
package events

import "fmt"

// SessionOpen is published when a Claude Code session starts.
// MQTT topic: claude/mesh/session/{session_id}/open
type SessionOpen struct {
	Ts             float64 `json:"ts"`
	SessionID      string  `json:"session_id"`
	Cwd            string  `json:"cwd"`
	TranscriptPath string  `json:"transcript_path"`
	GitBranch      string  `json:"git_branch"`
	Host           string  `json:"host"`
	PID            int     `json:"pid"`
}

// SessionClose is published when a Claude Code session ends.
// MQTT topic: claude/mesh/session/{session_id}/close
// Reason MUST be "stop" or "error".
type SessionClose struct {
	Ts        float64 `json:"ts"`
	SessionID string  `json:"session_id"`
	Reason    string  `json:"reason"`
}

// Activity is published when Claude Code fires a PreToolUse event.
// MQTT topic: claude/mesh/session/{session_id}/activity
type Activity struct {
	Ts        float64 `json:"ts"`
	SessionID string  `json:"session_id"`
	Tool      string  `json:"tool"`
	Target    string  `json:"target"`
	Cwd       string  `json:"cwd"`
}

// BuildSessionTopic returns the MQTT topic for a given session ID and event type.
// eventType must be "open", "activity", or "close".
func BuildSessionTopic(sid, eventType string) string {
	return fmt.Sprintf("claude/mesh/session/%s/%s", sid, eventType)
}
