package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/marioser/claude-mesh/internal/mcp/handlers"
	"github.com/marioser/claude-mesh/internal/mqtt"
)

// fakeMQTTClient is a test double for mqtt.Client used in announce tests.
type fakeMQTTClient struct {
	publishErr error
	published  []mqttPublish
}

type mqttPublish struct {
	topic   string
	payload []byte
}

func (f *fakeMQTTClient) Connect(_ context.Context) error { return nil }
func (f *fakeMQTTClient) Publish(_ context.Context, topic string, _ byte, _ bool, payload []byte) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published = append(f.published, mqttPublish{topic: topic, payload: payload})
	return nil
}
func (f *fakeMQTTClient) Subscribe(_ context.Context, _ string, _ byte, _ mqtt.MessageHandler) error {
	return nil
}
func (f *fakeMQTTClient) Disconnect(_ uint) {}

var _ mqtt.Client = (*fakeMQTTClient)(nil)

// TestAnnounceHappyPath verifies that mesh_announce publishes an activity event
// and returns {"ok": true} when MQTT is available.
func TestAnnounceHappyPath(t *testing.T) {
	mqttClient := &fakeMQTTClient{}

	handler := handlers.NewAnnounceHandler(mqttClient, 100)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"intent":     "starting redis refactor",
		"session_id": "sess-abc",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("AnnounceHandler: %v", err)
	}

	m := parseResult(t, result)

	ok, _ := m["ok"].(bool)
	if !ok {
		t.Errorf("expected ok:true, got %v", m["ok"])
	}
	if _, hasErr := m["error"]; hasErr {
		t.Errorf("expected no error field on success, got %v", m["error"])
	}

	// Verify that exactly one MQTT publish happened.
	if len(mqttClient.published) != 1 {
		t.Fatalf("expected 1 MQTT publish, got %d", len(mqttClient.published))
	}

	// Verify the topic follows the activity pattern for the given session.
	wantTopicSuffix := "sess-abc/activity"
	topic := mqttClient.published[0].topic
	if len(topic) < len(wantTopicSuffix) || topic[len(topic)-len(wantTopicSuffix):] != wantTopicSuffix {
		t.Errorf("topic %q does not end with %q", topic, wantTopicSuffix)
	}
}

// TestAnnounceWithoutSessionID verifies that mesh_announce works when no session_id
// is provided (uses empty string session_id).
func TestAnnounceWithoutSessionID(t *testing.T) {
	mqttClient := &fakeMQTTClient{}

	handler := handlers.NewAnnounceHandler(mqttClient, 100)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"intent": "rewriting the whole thing",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("AnnounceHandler without session_id: %v", err)
	}

	m := parseResult(t, result)
	ok, _ := m["ok"].(bool)
	if !ok {
		t.Errorf("expected ok:true, got %v", m["ok"])
	}

	// Should still publish (with empty session_id in topic).
	if len(mqttClient.published) != 1 {
		t.Fatalf("expected 1 MQTT publish, got %d", len(mqttClient.published))
	}
}

// TestAnnounceMQTTDown verifies that mesh_announce returns {"ok":false,"error":"mqtt unavailable"}
// without panicking when the MQTT broker is unreachable.
func TestAnnounceMQTTDown(t *testing.T) {
	mqttClient := &fakeMQTTClient{
		publishErr: errors.New("connection refused"),
	}

	handler := handlers.NewAnnounceHandler(mqttClient, 100)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"intent":     "starting redis refactor",
		"session_id": "sess-xyz",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("AnnounceHandler with MQTT down should not return Go error: %v", err)
	}

	m := parseResult(t, result)

	ok, _ := m["ok"].(bool)
	if ok {
		t.Error("expected ok:false when MQTT is down")
	}

	errMsg, _ := m["error"].(string)
	if errMsg != "mqtt unavailable" {
		t.Errorf("error field: got %q, want %q", errMsg, "mqtt unavailable")
	}
}

// TestAnnouncePublishedPayload verifies the published JSON payload contains
// tool="announce" and target=intent value.
func TestAnnouncePublishedPayload(t *testing.T) {
	mqttClient := &fakeMQTTClient{}

	handler := handlers.NewAnnounceHandler(mqttClient, 100)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"intent":     "checking auth module",
		"session_id": "sess-111",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("AnnounceHandler: %v", err)
	}
	m := parseResult(t, result)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok:true")
	}

	if len(mqttClient.published) == 0 {
		t.Fatal("no MQTT publish recorded")
	}

	// Parse the published payload and verify its fields.
	pub := mqttClient.published[0]
	var payload map[string]any
	if err := json.Unmarshal(pub.payload, &payload); err != nil {
		t.Fatalf("parse published payload: %v", err)
	}

	if tool, _ := payload["tool"].(string); tool != "announce" {
		t.Errorf("payload tool: got %q, want %q", tool, "announce")
	}
	if target, _ := payload["target"].(string); target != "checking auth module" {
		t.Errorf("payload target: got %q, want %q", target, "checking auth module")
	}
	if sid, _ := payload["session_id"].(string); sid != "sess-111" {
		t.Errorf("payload session_id: got %q, want %q", sid, "sess-111")
	}
	// ts must be present and positive.
	ts, _ := payload["ts"].(float64)
	if ts <= 0 {
		t.Errorf("payload ts: got %v, want > 0", ts)
	}
	// ts should be within the last minute.
	nowMs := float64(time.Now().UnixMilli())
	if ts < nowMs-60_000 || ts > nowMs+1_000 {
		t.Errorf("payload ts %v is not near current time %v", ts, nowMs)
	}
}
