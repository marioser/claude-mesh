// Package handlers implements the 4 Claude Mesh MCP tools.
// Each handler is constructed with a store.Store and a timeout milliseconds value,
// following the constructor pattern from mioboxtc.
package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"claude-mesh/internal/store"
)

// ToolFn is the function signature expected by mcp-go tool registrations.
// This must match server.ToolHandlerFunc exactly (same signature, convertible).
type ToolFn = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// textResult creates a successful MCP tool result from a JSON-serializable value.
func textResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

// statusResponse is the JSON shape for mesh_status.
type statusResponse struct {
	ActiveCount int                  `json:"active_count"`
	Sessions    []sessionSummary     `json:"sessions"`
	Degraded    bool                 `json:"degraded,omitempty"`
}

type sessionSummary struct {
	SessionID  string  `json:"session_id"`
	Cwd        string  `json:"cwd"`
	GitBranch  string  `json:"git_branch"`
	Host       string  `json:"host"`
	LastSeenMs float64 `json:"last_seen_ms"`
}

// NewStatusHandler returns a mesh_status tool handler.
// timeoutMs is the per-call Redis latency budget (design §8: HandlerTimeoutMs).
// Supports optional max_age_seconds parameter to filter stale sessions.
func NewStatusHandler(s store.Store, timeoutMs int) ToolFn {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		sessions, err := s.ListActiveSessions(ctx)
		if err != nil {
			return textResult(statusResponse{
				ActiveCount: 0,
				Sessions:    []sessionSummary{},
				Degraded:    true,
			})
		}

		// Apply max_age_seconds filter if provided.
		// cutoffMs is the oldest last_seen_ms that qualifies. Zero means no filter.
		var cutoffMs float64
		args := req.GetArguments()
		if maxAge, ok := args["max_age_seconds"].(float64); ok && maxAge > 0 {
			nowMs := float64(time.Now().UnixMilli())
			cutoffMs = nowMs - maxAge*1000
		}

		summaries := make([]sessionSummary, 0, len(sessions))
		for _, sv := range sessions {
			if cutoffMs > 0 && sv.LastSeenMs < cutoffMs {
				continue // session is older than the requested window
			}
			summaries = append(summaries, sessionSummary{
				SessionID:  sv.ID,
				Cwd:        sv.Cwd,
				GitBranch:  sv.GitBranch,
				Host:       sv.Host,
				LastSeenMs: sv.LastSeenMs,
			})
		}

		return textResult(statusResponse{
			ActiveCount: len(summaries),
			Sessions:    summaries,
		})
	}
}
