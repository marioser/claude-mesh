// Package publisher provides the PublishCmd function used by the `publish` subcommand.
// It reads a JSON payload from stdin, marshals the correct typed event, and publishes
// to MQTT. Total budget for connect+publish is ≤500ms (enforced via context timeout).
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/marioser/claude-mesh/internal/config"
	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/mqtt"
)

// PublishCmd reads a JSON payload from r, constructs the correct typed event for
// eventType, and publishes it to MQTT at QoS 1. It returns an error if the event
// type is unknown, the payload is malformed, or the publish fails.
//
// eventType must be one of: "session-open", "activity", "session-close".
func PublishCmd(ctx context.Context, eventType string, r io.Reader, client mqtt.Client, cfg config.EnvOptions) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("publisher: read stdin: %w", err)
	}

	switch eventType {
	case "session-open":
		return publishSessionOpen(ctx, data, client, cfg)
	case "activity":
		return publishActivity(ctx, data, client, cfg)
	case "session-close":
		return publishSessionClose(ctx, data, client, cfg)
	default:
		return fmt.Errorf("publisher: unknown event type %q (must be session-open, activity, or session-close)", eventType)
	}
}

func publishSessionOpen(ctx context.Context, data []byte, client mqtt.Client, _ config.EnvOptions) error {
	var ev events.SessionOpen
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("publisher: unmarshal session-open: %w", err)
	}

	out, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("publisher: marshal session-open: %w", err)
	}

	topic := events.BuildSessionTopic(ev.SessionID, "open")
	return client.Publish(ctx, topic, 1, false, out)
}

func publishActivity(ctx context.Context, data []byte, client mqtt.Client, _ config.EnvOptions) error {
	var ev events.Activity
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("publisher: unmarshal activity: %w", err)
	}

	out, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("publisher: marshal activity: %w", err)
	}

	topic := events.BuildSessionTopic(ev.SessionID, "activity")
	return client.Publish(ctx, topic, 1, false, out)
}

func publishSessionClose(ctx context.Context, data []byte, client mqtt.Client, _ config.EnvOptions) error {
	var ev events.SessionClose
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("publisher: unmarshal session-close: %w", err)
	}

	out, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("publisher: marshal session-close: %w", err)
	}

	topic := events.BuildSessionTopic(ev.SessionID, "close")
	return client.Publish(ctx, topic, 1, false, out)
}
