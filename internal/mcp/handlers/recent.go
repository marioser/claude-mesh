package handlers

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/store"
)

type recentEntry struct {
	SessionID string  `json:"session_id"`
	Ts        float64 `json:"ts"`
	Tool      string  `json:"tool"`
	Target    string  `json:"target"`
	Cwd       string  `json:"cwd"`
}

type recentResponse struct {
	Events []recentEntry `json:"events"`
}

// NewRecentHandler returns a mesh_recent_activity tool handler.
func NewRecentHandler(s store.Store, timeoutMs int) ToolFn {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		minutes := 10.0
		args := req.GetArguments()
		if v, ok := args["minutes"].(float64); ok && v > 0 {
			minutes = v
		}
		sid, _ := args["session_id"].(string)

		cutoffMs := float64(time.Now().UnixMilli()) - minutes*60*1000

		activities, err := s.RecentActivity(ctx, 200, sid)
		if err != nil {
			return textResult(recentResponse{Events: []recentEntry{}})
		}

		filtered := filterByTime(activities, cutoffMs)
		return textResult(recentResponse{Events: filtered})
	}
}

func filterByTime(acts []events.Activity, cutoffMs float64) []recentEntry {
	result := make([]recentEntry, 0, len(acts))
	for _, act := range acts {
		if act.Ts >= cutoffMs {
			result = append(result, recentEntry{
				SessionID: act.SessionID,
				Ts:        act.Ts,
				Tool:      act.Tool,
				Target:    act.Target,
				Cwd:       act.Cwd,
			})
		}
	}
	return result
}
