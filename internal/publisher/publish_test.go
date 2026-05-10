package publisher_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"claude-mesh/internal/config"
	"claude-mesh/internal/mqtt"
	"claude-mesh/internal/publisher"
)

// fakeClient is an in-test implementation of mqtt.Client.
type fakeClient struct {
	mu         sync.Mutex
	connected  bool
	published  []fakeMsg
	connectErr error
	publishErr error
}

type fakeMsg struct {
	topic   string
	qos     byte
	payload []byte
}

func (f *fakeClient) Connect(_ context.Context) error {
	if f.connectErr != nil {
		return f.connectErr
	}
	f.connected = true
	return nil
}

func (f *fakeClient) Publish(_ context.Context, topic string, qos byte, _ bool, payload []byte) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.mu.Lock()
	f.published = append(f.published, fakeMsg{topic: topic, qos: qos, payload: payload})
	f.mu.Unlock()
	return nil
}

func (f *fakeClient) Subscribe(_ context.Context, _ string, _ byte, _ mqtt.MessageHandler) error {
	return nil
}

func (f *fakeClient) Disconnect(_ uint) { f.connected = false }

var _ mqtt.Client = (*fakeClient)(nil)

func defaultCfg() config.EnvOptions {
	cfg, _ := config.Load()
	return cfg
}

// TestPublishSessionOpen verifies that PublishCmd with "session-open" builds
// the correct topic and publishes at QoS 1.
func TestPublishSessionOpen(t *testing.T) {
	fake := &fakeClient{}
	cfg := defaultCfg()

	payload := map[string]any{
		"ts":              1715300000000.0,
		"session_id":     "test-sess",
		"cwd":            "/project",
		"transcript_path": "/tmp/t.txt",
		"git_branch":     "main",
		"host":           "host1",
		"pid":            42,
	}
	raw, _ := json.Marshal(payload)

	err := publisher.PublishCmd(context.Background(), "session-open", bytes.NewReader(raw), fake, cfg)
	if err != nil {
		t.Fatalf("PublishCmd session-open: %v", err)
	}

	if len(fake.published) != 1 {
		t.Fatalf("published count: got %d, want 1", len(fake.published))
	}
	if !strings.HasSuffix(fake.published[0].topic, "/open") {
		t.Errorf("topic: got %q, expected suffix /open", fake.published[0].topic)
	}
	if fake.published[0].qos != 1 {
		t.Errorf("QoS: got %d, want 1", fake.published[0].qos)
	}
}

// TestPublishActivity verifies that PublishCmd with "activity" uses /activity topic.
func TestPublishActivity(t *testing.T) {
	fake := &fakeClient{}
	cfg := defaultCfg()

	payload := map[string]any{
		"ts":         1715300001000.0,
		"session_id": "act-sess",
		"tool":       "Edit",
		"target":     "main.go",
		"cwd":        "/p",
	}
	raw, _ := json.Marshal(payload)

	err := publisher.PublishCmd(context.Background(), "activity", bytes.NewReader(raw), fake, cfg)
	if err != nil {
		t.Fatalf("PublishCmd activity: %v", err)
	}

	if !strings.HasSuffix(fake.published[0].topic, "/activity") {
		t.Errorf("topic: got %q, expected suffix /activity", fake.published[0].topic)
	}
}

// TestPublishSessionClose verifies that PublishCmd with "session-close" uses /close topic.
func TestPublishSessionClose(t *testing.T) {
	fake := &fakeClient{}
	cfg := defaultCfg()

	payload := map[string]any{
		"ts":         1715300002000.0,
		"session_id": "close-sess",
		"reason":     "stop",
	}
	raw, _ := json.Marshal(payload)

	err := publisher.PublishCmd(context.Background(), "session-close", bytes.NewReader(raw), fake, cfg)
	if err != nil {
		t.Fatalf("PublishCmd session-close: %v", err)
	}

	if !strings.HasSuffix(fake.published[0].topic, "/close") {
		t.Errorf("topic: got %q, expected suffix /close", fake.published[0].topic)
	}
}

// TestPublishFailureReturnsError verifies that a MQTT publish failure returns
// an error instead of panicking.
func TestPublishFailureReturnsError(t *testing.T) {
	fake := &fakeClient{publishErr: errors.New("broker down")}
	cfg := defaultCfg()

	payload := map[string]any{
		"ts":         1715300003000.0,
		"session_id": "fail-sess",
		"reason":     "stop",
	}
	raw, _ := json.Marshal(payload)

	err := publisher.PublishCmd(context.Background(), "session-close", bytes.NewReader(raw), fake, cfg)
	if err == nil {
		t.Error("expected error from PublishCmd when broker is down, got nil")
	}
}

// TestPublishUnknownEventType verifies that an unknown event type returns an error.
func TestPublishUnknownEventType(t *testing.T) {
	fake := &fakeClient{}
	cfg := defaultCfg()

	err := publisher.PublishCmd(context.Background(), "unknown-type", strings.NewReader(`{}`), fake, cfg)
	if err == nil {
		t.Error("expected error for unknown event type, got nil")
	}
}
