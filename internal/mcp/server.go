// Package mcp builds the Claude Mesh MCP server and registers all tool handlers.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/marioser/claude-mesh/internal/config"
	"github.com/marioser/claude-mesh/internal/mcp/handlers"
	"github.com/marioser/claude-mesh/internal/mqtt"
	"github.com/marioser/claude-mesh/internal/store"
)

// NewServer creates an MCP server with all Claude Mesh tools registered.
// mqttClient is used exclusively by the mesh_announce handler to publish intent events.
// It may be nil if MQTT is unavailable; the handler will return degraded: true in that case.
func NewServer(s store.Store, cfg config.EnvOptions, mqttClient mqtt.Client) *server.MCPServer {
	srv := server.NewMCPServer(
		"claude-mesh-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithInstructions(
			"Claude Mesh observability tools: query active Claude Code sessions, "+
				"check file conflicts, view recent activity.",
		),
	)

	timeoutMs := cfg.HandlerTimeoutMs

	// mesh_status — active session count and summaries.
	srv.AddTool(
		mcp.NewTool("mesh_status",
			mcp.WithDescription("Returns the count and summaries of all active Claude Code sessions."),
			mcp.WithNumber("max_age_seconds",
				mcp.Description("Only include sessions with last_seen_ms >= now - max_age_seconds*1000 (optional)."),
			),
		),
		handlers.NewStatusHandler(s, timeoutMs),
	)

	// mesh_check_conflict — detect concurrent edits to a file.
	srv.AddTool(
		mcp.NewTool("mesh_check_conflict",
			mcp.WithDescription("Reports which other sessions recently touched the given file path."),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Relative or absolute file path to check for concurrent edits."),
			),
			mcp.WithString("session_id",
				mcp.Description("Calling session ID to exclude from results (optional)."),
			),
		),
		handlers.NewConflictHandler(s, timeoutMs),
	)

	// mesh_recent_activity — global cross-session activity ring.
	srv.AddTool(
		mcp.NewTool("mesh_recent_activity",
			mcp.WithDescription("Returns cross-session activity from the global ring buffer, newest first."),
			mcp.WithNumber("minutes",
				mcp.Description("Time window in minutes (default: 10)."),
			),
			mcp.WithString("session_id",
				mcp.Description("Filter to a specific session (optional)."),
			),
		),
		handlers.NewRecentHandler(s, timeoutMs),
	)

	// mesh_active_sessions — list all active sessions.
	srv.AddTool(
		mcp.NewTool("mesh_active_sessions",
			mcp.WithDescription("Lists all currently active Claude Code sessions."),
		),
		handlers.NewSessionsHandler(s, timeoutMs),
	)

	// mesh_announce — publish a manual intent event to MQTT for the current session.
	srv.AddTool(
		mcp.NewTool("mesh_announce",
			mcp.WithDescription("Publishes a manual intent event to MQTT so other sessions can observe it."),
			mcp.WithString("intent",
				mcp.Required(),
				mcp.Description("Free-form description of the work being started (e.g. 'starting redis refactor')."),
			),
			mcp.WithString("session_id",
				mcp.Description("Calling session ID (optional; used to stamp the activity event)."),
			),
		),
		handlers.NewAnnounceHandler(mqttClient, timeoutMs),
	)

	return srv
}
