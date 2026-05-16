// publisher.go wraps Client.Publish with topic builders for the Claude Mesh protocol.
package mqtt

import (
	"context"

	"github.com/marioser/claude-mesh/internal/events"
)

// Publisher sends typed events to the MQTT broker using the correct topic scheme.
type Publisher struct {
	client Client
}

// NewPublisher creates a Publisher backed by the given Client.
func NewPublisher(client Client) *Publisher {
	return &Publisher{client: client}
}

// PublishSessionOpen publishes a SessionOpen event at QoS 1.
func (p *Publisher) PublishSessionOpen(ctx context.Context, ev events.SessionOpen, payload []byte) error {
	topic := events.BuildSessionTopic(ev.SessionID, "open")
	return p.client.Publish(ctx, topic, 1, false, payload)
}

// PublishActivity publishes an Activity event at QoS 1.
func (p *Publisher) PublishActivity(ctx context.Context, ev events.Activity, payload []byte) error {
	topic := events.BuildSessionTopic(ev.SessionID, "activity")
	return p.client.Publish(ctx, topic, 1, false, payload)
}

// PublishSessionClose publishes a SessionClose event at QoS 1.
func (p *Publisher) PublishSessionClose(ctx context.Context, ev events.SessionClose, payload []byte) error {
	topic := events.BuildSessionTopic(ev.SessionID, "close")
	return p.client.Publish(ctx, topic, 1, false, payload)
}
