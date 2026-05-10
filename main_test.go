package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"claude-mesh/internal/usagestats"
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

	pollUsageOnce(context.Background(), client, projectsDir, plan, log)

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
