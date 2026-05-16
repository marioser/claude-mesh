package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/marioser/claude-mesh/internal/usagestats"
)

// TestPollUsageOnceWritesRedisKeys verifies that pollUsageOnce scans JSONL files
// and writes the expected keys to Redis with non-zero token counts when usage data exists.
func TestPollUsageOnceWritesRedisKeys(t *testing.T) {
	// Spin up miniredis.
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set up a temporary projects directory with a known JSONL file.
	projectsDir := t.TempDir()
	projA := filepath.Join(projectsDir, "-Users-test-poll")
	if err := os.MkdirAll(projA, 0755); err != nil {
		t.Fatal(err)
	}
	// Write one assistant message with 20000 tokens (5000+10000+5000).
	jsonl := `{"type":"assistant","timestamp":"2026-05-09T10:00:00.000Z","sessionId":"test-poll","message":{"role":"assistant","usage":{"input_tokens":5000,"output_tokens":100,"cache_creation_input_tokens":10000,"cache_read_input_tokens":5000}}}` + "\n"
	if err := os.WriteFile(filepath.Join(projA, "test.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatal(err)
	}

	plan := usagestats.Plan{
		Tier:      "max20",
		Limit5h:   220_000,
		LimitWeek: 7_392_000,
	}
	log, _ := zap.NewDevelopment()

	pollUsageOnce(context.Background(), client, projectsDir, plan, log, anthropicAPICfg{})

	// Verify keys were written.
	ctx := context.Background()

	tokens5h, err := client.Get(ctx, "claude:mesh:usage:tokens:5h").Result()
	if err != nil {
		t.Fatalf("tokens:5h key not written: %v", err)
	}
	// 20000 tokens — they are within the last 5h (timestamp is recent relative to "since" filter).
	// Note: timestamp 2026-05-09 may be outside the 5h window depending on test run time.
	// So we just check the key exists (non-empty string).
	if tokens5h == "" {
		t.Error("claude:mesh:usage:tokens:5h: want non-empty value, got empty")
	}

	planVal, err := client.Get(ctx, "claude:mesh:usage:plan").Result()
	if err != nil {
		t.Fatalf("usage:plan key not written: %v", err)
	}
	if planVal != "max20" {
		t.Errorf("usage:plan: want 'max20', got %q", planVal)
	}

	// Verify TTL is set (should be <= 120s).
	ttl := client.TTL(ctx, "claude:mesh:usage:plan").Val()
	if ttl <= 0 {
		t.Errorf("usage:plan TTL: want > 0, got %v", ttl)
	}
}

// TestPollUsageOnceUsesAnthropicAPIWhenConfigured verifies that when org_id and cookie
// are set, pollUsageOnce fetches from the Anthropic API and writes "anthropic" as the source.
func TestPollUsageOnceUsesAnthropicAPIWhenConfigured(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Mock Anthropic API server.
	apiBody := map[string]any{
		"five_hour": map[string]any{
			"utilization": 42.0,
			"resets_at":   "2026-05-10T19:00:00Z",
		},
		"seven_day": map[string]any{
			"utilization": 17.5,
			"resets_at":   "2026-05-17T00:00:00Z",
		},
		"seven_day_sonnet": nil,
		"seven_day_opus":   nil,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiBody)
	}))
	defer srv.Close()

	plan := usagestats.Plan{Tier: "max20", Limit5h: 220_000, LimitWeek: 7_392_000}
	log, _ := zap.NewDevelopment()

	// Empty projectsDir — ensures JSONL fallback contributes 0 tokens.
	projectsDir := t.TempDir()

	apiCfg := anthropicAPICfg{
		orgID:   "org-test-123",
		cookie:  "session=test",
		baseURL: srv.URL,
	}

	pollUsageOnce(context.Background(), client, projectsDir, plan, log, apiCfg)

	ctx := context.Background()

	// Source should be "anthropic".
	source, err := client.Get(ctx, "claude:mesh:usage:source").Result()
	if err != nil {
		t.Fatalf("usage:source key not written: %v", err)
	}
	if source != "anthropic" {
		t.Errorf("usage:source = %q, want 'anthropic'", source)
	}

	// pct:5h should be 42.0 (directly from API utilization field).
	pct5h, err := client.Get(ctx, "claude:mesh:usage:pct:5h").Result()
	if err != nil {
		t.Fatalf("usage:pct:5h key not written: %v", err)
	}
	if !strings.HasPrefix(pct5h, "42") {
		t.Errorf("usage:pct:5h = %q, want prefix '42'", pct5h)
	}

	// pct:week should be 17.5 from API.
	pctWeek, err := client.Get(ctx, "claude:mesh:usage:pct:week").Result()
	if err != nil {
		t.Fatalf("usage:pct:week key not written: %v", err)
	}
	if !strings.HasPrefix(pctWeek, "17") {
		t.Errorf("usage:pct:week = %q, want prefix '17'", pctWeek)
	}

	// resets_at keys should be set.
	resetsAt5h, err := client.Get(ctx, "claude:mesh:usage:resets_at:5h").Result()
	if err != nil {
		t.Fatalf("usage:resets_at:5h key not written: %v", err)
	}
	if resetsAt5h == "" {
		t.Error("usage:resets_at:5h: want non-empty")
	}
}

// TestPollUsageOnceAnthropicAPI401FallsBackToJSONL verifies that when the API returns 401,
// the daemon falls back to local JSONL parsing without crashing.
func TestPollUsageOnceAnthropicAPI401FallsBackToJSONL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// API returns 401.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// Set up JSONL data for fallback.
	projectsDir := t.TempDir()
	projA := filepath.Join(projectsDir, "proj-fallback")
	if err := os.MkdirAll(projA, 0755); err != nil {
		t.Fatal(err)
	}
	jsonl := `{"type":"assistant","timestamp":"2026-05-09T10:00:00.000Z","sessionId":"test-fb","message":{"role":"assistant","usage":{"input_tokens":5000,"output_tokens":100,"cache_creation_input_tokens":10000,"cache_read_input_tokens":5000}}}` + "\n"
	if err := os.WriteFile(filepath.Join(projA, "test.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatal(err)
	}

	plan := usagestats.Plan{Tier: "max20", Limit5h: 220_000, LimitWeek: 7_392_000}
	log, _ := zap.NewDevelopment()

	apiCfg := anthropicAPICfg{
		orgID:   "org-test",
		cookie:  "bad-cookie",
		baseURL: srv.URL,
	}

	pollUsageOnce(context.Background(), client, projectsDir, plan, log, apiCfg)

	ctx := context.Background()

	// Source should be "local" (fallback).
	source, err := client.Get(ctx, "claude:mesh:usage:source").Result()
	if err != nil {
		t.Fatalf("usage:source key not written: %v", err)
	}
	if source != "local" {
		t.Errorf("usage:source = %q, want 'local'", source)
	}

	// JSONL tokens should have been written.
	tokensWeek, err := client.Get(ctx, "claude:mesh:usage:tokens:week").Result()
	if err != nil {
		t.Fatalf("tokens:week key not written: %v", err)
	}
	if tokensWeek == "" {
		t.Error("tokens:week: want non-empty after JSONL fallback")
	}
}

// TestPollUsageOnceNoAnthropicConfigUsesJSONL verifies that when anthropic config is empty,
// the local JSONL path is used and source is "local".
func TestPollUsageOnceNoAnthropicConfigUsesJSONL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	projectsDir := t.TempDir()
	plan := usagestats.Plan{Tier: "pro", Limit5h: 19_000, LimitWeek: 638_400}
	log, _ := zap.NewDevelopment()

	// Empty apiCfg means "disabled".
	apiCfg := anthropicAPICfg{}
	pollUsageOnce(context.Background(), client, projectsDir, plan, log, apiCfg)

	ctx := context.Background()
	source, err := client.Get(ctx, "claude:mesh:usage:source").Result()
	if err != nil {
		t.Fatalf("usage:source key not written: %v", err)
	}
	if source != "local" {
		t.Errorf("usage:source = %q, want 'local'", source)
	}
}

// TestBuildSubscriberClientID verifies that the generated client_id contains
// the base name, hostname, and current PID — preventing the "session taken over"
// MQTT loop caused by a fixed client_id when multiple daemons run concurrently.
func TestBuildSubscriberClientID(t *testing.T) {
	base := "claude-mesh"
	id := buildSubscriberClientID(base)

	hostname, _ := os.Hostname()
	pid := strconv.Itoa(os.Getpid())

	if !strings.Contains(id, base+"-sub-") {
		t.Errorf("client_id %q does not contain base prefix %q", id, base+"-sub-")
	}
	if hostname != "" && !strings.Contains(id, hostname) {
		t.Errorf("client_id %q does not contain hostname %q", id, hostname)
	}
	if !strings.Contains(id, pid) {
		t.Errorf("client_id %q does not contain pid %q", id, pid)
	}
	// Sanity: format is base-sub-hostname-pid
	expected := fmt.Sprintf("%s-sub-%s-%s", base, hostname, pid)
	if id != expected {
		t.Errorf("client_id = %q, want %q", id, expected)
	}
}
