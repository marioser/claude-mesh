// subscriber.go wraps Client.Subscribe with the Claude Mesh wildcard topic and
// tracks subscriber health state via atomic fields.
package mqtt

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// SessionWildcardTopic is the MQTT wildcard that catches all session events.
	SessionWildcardTopic = "claude/mesh/session/+/+"
)

// SubscriberStats is the health snapshot exposed by Subscriber.Stats().
type SubscriberStats struct {
	Connected   bool
	Subscribed  bool
	MsgCount    int64
	LastMsgMs   int64 // unix millis of last received message, 0 if none
	StartedAtMs int64 // unix millis when Subscribe() succeeded, 0 if never
}

// Subscriber routes incoming MQTT messages to the bridge event channel and
// tracks liveness state that the bridge writes to Redis for status queries.
type Subscriber struct {
	client Client

	connected   atomic.Bool
	subscribed  atomic.Bool
	msgCount    atomic.Int64
	lastMsgMs   atomic.Int64
	startedAtMs atomic.Int64

	// mu protects wrappedHandler. wrappedHandler is set at subscribe time and read by
	// ResubscribeOnReconnect from the OnConnectHandler goroutine.
	mu             sync.Mutex
	wrappedHandler MessageHandler

	// subscribedTopics is set once at subscribe time. Safe to read after Subscribe returns.
	subscribedTopics []string
}

// NewSubscriber creates a Subscriber backed by the given Client.
func NewSubscriber(client Client) *Subscriber {
	return &Subscriber{client: client}
}

// SetConnected updates the connected flag. Called from the OnConnectHandler
// callback (true on reconnect) or connection-lost callback (false).
func (s *Subscriber) SetConnected(v bool) {
	s.connected.Store(v)
}

// Subscribe registers a handler on the session wildcard topic using QoS 1.
// It wraps the caller's handler to count and timestamp each received message.
// The wrapped handler is stored so ResubscribeOnReconnect can re-use it.
func (s *Subscriber) Subscribe(ctx context.Context, handler MessageHandler) error {
	wrapped := func(topic string, payload []byte) {
		s.msgCount.Add(1)
		s.lastMsgMs.Store(time.Now().UnixMilli())
		handler(topic, payload)
	}

	if err := s.client.Subscribe(ctx, SessionWildcardTopic, 1, wrapped); err != nil {
		return err
	}

	s.mu.Lock()
	s.wrappedHandler = wrapped
	s.mu.Unlock()

	s.subscribed.Store(true)
	s.startedAtMs.Store(time.Now().UnixMilli())
	s.subscribedTopics = []string{SessionWildcardTopic}
	return nil
}

// ResubscribeOnReconnect re-issues Subscribe using the wrapped handler captured
// during the most recent Subscribe call. Intended to be called from the paho
// OnConnectHandler goroutine after a broker reconnect.
// Returns an error if Subscribe was never called (no handler captured yet).
func (s *Subscriber) ResubscribeOnReconnect(ctx context.Context) error {
	s.mu.Lock()
	h := s.wrappedHandler
	s.mu.Unlock()

	if h == nil {
		return fmt.Errorf("mqtt subscriber: no handler registered yet, skipping resubscribe")
	}

	if err := s.client.Subscribe(ctx, SessionWildcardTopic, 1, h); err != nil {
		return fmt.Errorf("mqtt resubscribe: %w", err)
	}

	s.startedAtMs.Store(time.Now().UnixMilli())
	return nil
}

// SubscribedTopics returns the topics registered at subscribe time (nil if never subscribed).
func (s *Subscriber) SubscribedTopics() []string {
	return s.subscribedTopics
}

// Stats returns a snapshot of the subscriber's current health state.
func (s *Subscriber) Stats() SubscriberStats {
	return SubscriberStats{
		Connected:   s.connected.Load(),
		Subscribed:  s.subscribed.Load(),
		MsgCount:    s.msgCount.Load(),
		LastMsgMs:   s.lastMsgMs.Load(),
		StartedAtMs: s.startedAtMs.Load(),
	}
}
