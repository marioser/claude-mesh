package mqtt_test

import (
	"context"
	"testing"
	"time"

	"claude-mesh/internal/mqtt"
)

// fakeSubClient is a Client implementation for subscriber tests.
// It records Subscribe calls and allows triggering messages.
type fakeSubClient struct {
	fakeClient
	subscribeErr error
}

func (f *fakeSubClient) Subscribe(_ context.Context, topic string, qos byte, handler mqtt.MessageHandler) error {
	if f.subscribeErr != nil {
		return f.subscribeErr
	}
	f.mu.Lock()
	f.subscribed = append(f.subscribed, fakeSub{topic: topic, qos: qos, handler: handler})
	f.mu.Unlock()
	return nil
}

// deliver calls all registered handlers with the given message.
func (f *fakeSubClient) deliver(topic string, payload []byte) {
	f.mu.Lock()
	handlers := make([]fakeSub, len(f.subscribed))
	copy(handlers, f.subscribed)
	f.mu.Unlock()
	for _, s := range handlers {
		s.handler(topic, payload)
	}
}

// TestSubscriberStatsAfterSubscribe verifies that Stats() reflects subscribed=true
// and msg_count increments on each delivered message.
func TestSubscriberStatsAfterSubscribe(t *testing.T) {
	client := &fakeSubClient{}
	sub := mqtt.NewSubscriber(client)

	before := time.Now().UnixMilli()
	if err := sub.Subscribe(context.Background(), func(_ string, _ []byte) {}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	stats := sub.Stats()
	if !stats.Subscribed {
		t.Error("expected Subscribed=true after Subscribe()")
	}
	if stats.StartedAtMs < before {
		t.Errorf("StartedAtMs %d < before %d", stats.StartedAtMs, before)
	}
	if stats.MsgCount != 0 {
		t.Errorf("initial MsgCount: got %d, want 0", stats.MsgCount)
	}

	// Deliver two messages.
	client.deliver("claude/mesh/session/s1/open", []byte(`{}`))
	client.deliver("claude/mesh/session/s1/activity", []byte(`{}`))

	stats = sub.Stats()
	if stats.MsgCount != 2 {
		t.Errorf("MsgCount after 2 deliveries: got %d, want 2", stats.MsgCount)
	}
	if stats.LastMsgMs < before {
		t.Errorf("LastMsgMs %d < before %d; should be recent", stats.LastMsgMs, before)
	}
}

// TestSubscriberConnectedFlag verifies that SetConnected controls the connected flag.
func TestSubscriberConnectedFlag(t *testing.T) {
	sub := mqtt.NewSubscriber(&fakeSubClient{})

	if sub.Stats().Connected {
		t.Error("expected Connected=false before SetConnected(true)")
	}
	sub.SetConnected(true)
	if !sub.Stats().Connected {
		t.Error("expected Connected=true after SetConnected(true)")
	}
	sub.SetConnected(false)
	if sub.Stats().Connected {
		t.Error("expected Connected=false after SetConnected(false)")
	}
}

// TestSubscriberResubscribeOnReconnect verifies that ResubscribeOnReconnect re-issues
// Subscribe on the underlying client, using the same wrapped handler.
func TestSubscriberResubscribeOnReconnect(t *testing.T) {
	client := &fakeSubClient{}
	sub := mqtt.NewSubscriber(client)

	var received []string
	handler := func(topic string, _ []byte) {
		received = append(received, topic)
	}

	if err := sub.Subscribe(context.Background(), handler); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Simulate reconnect — should re-register the handler.
	if err := sub.ResubscribeOnReconnect(context.Background()); err != nil {
		t.Fatalf("ResubscribeOnReconnect: %v", err)
	}

	// After resubscribe, delivering a message should still reach the handler.
	// (The last registered handler in fakeSubClient — deliver calls all registered.)
	client.deliver("claude/mesh/session/s2/open", []byte(`{}`))

	if len(received) == 0 {
		t.Error("handler not called after ResubscribeOnReconnect + message delivery")
	}
}

// TestSubscriberResubscribeBeforeSubscribe verifies that ResubscribeOnReconnect returns
// an error when called before any Subscribe.
func TestSubscriberResubscribeBeforeSubscribe(t *testing.T) {
	sub := mqtt.NewSubscriber(&fakeSubClient{})
	if err := sub.ResubscribeOnReconnect(context.Background()); err == nil {
		t.Error("expected error from ResubscribeOnReconnect before Subscribe, got nil")
	}
}
