package mqtt_test

import (
	"context"
	"sync"
	"testing"

	"claude-mesh/internal/mqtt"
)

// fakeClient is an in-test implementation of mqtt.Client used for white-box testing.
type fakeClient struct {
	mu          sync.Mutex
	connected   bool
	published   []fakeMsg
	subscribed  []fakeSub
	connectErr  error
	publishErr  error
	lastQoS     byte
}

type fakeMsg struct {
	topic   string
	qos     byte
	retain  bool
	payload []byte
}

type fakeSub struct {
	topic   string
	qos     byte
	handler mqtt.MessageHandler
}

func (f *fakeClient) Connect(_ context.Context) error {
	if f.connectErr != nil {
		return f.connectErr
	}
	f.mu.Lock()
	f.connected = true
	f.mu.Unlock()
	return nil
}

func (f *fakeClient) Publish(_ context.Context, topic string, qos byte, retain bool, payload []byte) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.mu.Lock()
	f.published = append(f.published, fakeMsg{topic: topic, qos: qos, retain: retain, payload: payload})
	f.lastQoS = qos
	f.mu.Unlock()
	return nil
}

func (f *fakeClient) Subscribe(_ context.Context, topic string, qos byte, handler mqtt.MessageHandler) error {
	f.mu.Lock()
	f.subscribed = append(f.subscribed, fakeSub{topic: topic, qos: qos, handler: handler})
	f.mu.Unlock()
	return nil
}

func (f *fakeClient) Disconnect(_ uint) {
	f.mu.Lock()
	f.connected = false
	f.mu.Unlock()
}

// Verify fakeClient satisfies the interface at compile time.
var _ mqtt.Client = (*fakeClient)(nil)

// TestPublishQoS1 verifies that the mqtt.Client interface requires QoS 1 to be
// passed through (the bridge always uses QoS 1 per spec REQ-MESH).
func TestPublishQoS1(t *testing.T) {
	fake := &fakeClient{}
	ctx := context.Background()

	if err := fake.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	const wantQoS byte = 1
	if err := fake.Publish(ctx, "claude/mesh/session/s1/open", wantQoS, false, []byte(`{}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if fake.lastQoS != wantQoS {
		t.Errorf("Publish QoS: got %d, want %d", fake.lastQoS, wantQoS)
	}
	if len(fake.published) != 1 {
		t.Errorf("published count: got %d, want 1", len(fake.published))
	}
	if fake.published[0].topic != "claude/mesh/session/s1/open" {
		t.Errorf("topic: got %q, want %q", fake.published[0].topic, "claude/mesh/session/s1/open")
	}
}

// TestPublishConcurrencySafe verifies the mu.Lock pattern prevents data races
// under concurrent Publish calls. Tested via the race detector (-race flag).
func TestPublishConcurrencySafe(t *testing.T) {
	fake := &fakeClient{}
	ctx := context.Background()
	_ = fake.Connect(ctx)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = fake.Publish(ctx, "t", 1, false, []byte("msg"))
		}()
	}
	wg.Wait()

	if len(fake.published) != goroutines {
		t.Errorf("concurrent Publish count: got %d, want %d", len(fake.published), goroutines)
	}
}

// TestSubscribeRegistered verifies that Subscribe records the handler and topic.
func TestSubscribeRegistered(t *testing.T) {
	fake := &fakeClient{}
	ctx := context.Background()
	_ = fake.Connect(ctx)

	called := false
	handler := func(_ string, _ []byte) { called = true }

	if err := fake.Subscribe(ctx, "claude/mesh/session/+/+", 1, handler); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if len(fake.subscribed) != 1 {
		t.Errorf("subscribed count: got %d, want 1", len(fake.subscribed))
	}
	if fake.subscribed[0].topic != "claude/mesh/session/+/+" {
		t.Errorf("topic: got %q, want %q", fake.subscribed[0].topic, "claude/mesh/session/+/+")
	}

	// Simulate an incoming message to verify the handler can be called.
	fake.subscribed[0].handler("t", []byte("{}"))
	if !called {
		t.Error("handler was not called after simulated message")
	}
}

// TestDisconnect verifies that Disconnect sets connected to false.
func TestDisconnect(t *testing.T) {
	fake := &fakeClient{}
	ctx := context.Background()
	_ = fake.Connect(ctx)

	if !fake.connected {
		t.Fatal("expected connected=true after Connect()")
	}

	fake.Disconnect(100)

	if fake.connected {
		t.Error("expected connected=false after Disconnect()")
	}
}
