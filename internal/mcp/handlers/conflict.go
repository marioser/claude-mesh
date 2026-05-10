package handlers

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"claude-mesh/internal/store"
)

type conflictEntry struct {
	SessionID  string  `json:"session_id"`
	LastSeenMs float64 `json:"last_seen_ms"`
	Tool       string  `json:"tool"`
}

type conflictResponse struct {
	Conflicts []conflictEntry `json:"conflicts"`
}

// NewConflictHandler returns a mesh_check_conflict tool handler.
// It reads the global ring buffer and filters by target path, excluding the caller's session.
func NewConflictHandler(s store.Store, timeoutMs int) ToolFn {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		args := req.GetArguments()
		filePath, _ := args["file_path"].(string)
		callerSID, _ := args["session_id"].(string)

		// Single LRANGE 0 199 on the global ring.
		activities, err := s.RecentActivity(ctx, 200, "")
		if err != nil {
			return textResult(conflictResponse{Conflicts: []conflictEntry{}})
		}

		seen := map[string]conflictEntry{}
		for _, act := range activities {
			if act.Target != filePath {
				continue
			}
			if act.SessionID == callerSID {
				continue
			}
			// Keep the most recent entry per session (list is newest-first).
			if _, exists := seen[act.SessionID]; !exists {
				seen[act.SessionID] = conflictEntry{
					SessionID:  act.SessionID,
					LastSeenMs: act.Ts,
					Tool:       act.Tool,
				}
			}
		}

		conflicts := make([]conflictEntry, 0, len(seen))
		for _, e := range seen {
			conflicts = append(conflicts, e)
		}
		if conflicts == nil {
			conflicts = []conflictEntry{}
		}

		return textResult(conflictResponse{Conflicts: conflicts})
	}
}
