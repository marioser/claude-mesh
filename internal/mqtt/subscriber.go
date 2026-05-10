// subscriber.go wraps Client.Subscribe with the Claude Mesh wildcard topic.
package mqtt

import "context"

const (
	// SessionWildcardTopic is the MQTT wildcard that catches all session events.
	SessionWildcardTopic = "claude/mesh/session/+/+"
)

// Subscriber routes incoming MQTT messages to the bridge event channel.
type Subscriber struct {
	client Client
}

// NewSubscriber creates a Subscriber backed by the given Client.
func NewSubscriber(client Client) *Subscriber {
	return &Subscriber{client: client}
}

// Subscribe registers a handler on the session wildcard topic using QoS 1.
func (s *Subscriber) Subscribe(ctx context.Context, handler MessageHandler) error {
	return s.client.Subscribe(ctx, SessionWildcardTopic, 1, handler)
}
