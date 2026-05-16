package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/mqtt"
)

// announceResponse is the JSON shape for mesh_announce.
type announceResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// NewAnnounceHandler returns a mesh_announce tool handler.
// It publishes a manual intent event to MQTT as an Activity event with tool="announce".
// timeoutMs is the per-call budget for the MQTT publish.
func NewAnnounceHandler(client mqtt.Client, timeoutMs int) ToolFn {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		intent, _ := args["intent"].(string)
		sessionID, _ := args["session_id"].(string)

		ev := events.Activity{
			Ts:        float64(time.Now().UnixMilli()),
			SessionID: sessionID,
			Tool:      "announce",
			Target:    intent,
		}

		payload, err := json.Marshal(ev)
		if err != nil {
			return textResult(announceResponse{OK: false, Error: "mqtt unavailable"})
		}

		topic := events.BuildSessionTopic(sessionID, "activity")

		pubCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		if err := client.Publish(pubCtx, topic, 1, false, payload); err != nil {
			return textResult(announceResponse{OK: false, Error: "mqtt unavailable"})
		}

		return textResult(announceResponse{OK: true})
	}
}
