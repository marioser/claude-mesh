package handlers

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/marioser/claude-mesh/internal/store"
)

type sessionsResponse struct {
	Sessions []sessionSummary `json:"sessions"`
}

// NewSessionsHandler returns a mesh_active_sessions tool handler.
func NewSessionsHandler(s store.Store, timeoutMs int) ToolFn {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		sessions, err := s.ListActiveSessions(ctx)
		if err != nil {
			return textResult(sessionsResponse{Sessions: []sessionSummary{}})
		}

		summaries := make([]sessionSummary, 0, len(sessions))
		for _, sv := range sessions {
			summaries = append(summaries, sessionSummary{
				SessionID:  sv.ID,
				Cwd:        sv.Cwd,
				GitBranch:  sv.GitBranch,
				Host:       sv.Host,
				LastSeenMs: sv.LastSeenMs,
			})
		}

		return textResult(sessionsResponse{Sessions: summaries})
	}
}
