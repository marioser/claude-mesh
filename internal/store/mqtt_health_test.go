package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"claude-mesh/internal/store"
)

// newRedisStore creates a *store.RedisStore (concrete) backed by miniredis.
// Used to access WriteMQTTHealth / ReadMQTTHealth which are not on Store interface.
func newRedisStore(t *testing.T) *store.RedisStore {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return store.NewRedisStore(client, store.DefaultConfig())
}

// TestWriteReadMQTTHealth verifies the round-trip: WriteMQTTHealth persists the
// health struct and ReadMQTTHealth returns it with the expected field values.
func TestWriteReadMQTTHealth(t *testing.T) {
	s := newRedisStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	fields := store.MQTTHealthFields{
		Connected:   true,
		Subscribed:  true,
		MsgCount:    42,
		LastMsgMs:   now - 1000,
		StartedAtMs: now - 5000,
		UpdatedAtMs: now,
	}

	if err := s.WriteMQTTHealth(ctx, fields); err != nil {
		t.Fatalf("WriteMQTTHealth: %v", err)
	}

	got, found, err := s.ReadMQTTHealth(ctx)
	if err != nil {
		t.Fatalf("ReadMQTTHealth: %v", err)
	}
	if !found {
		t.Fatal("ReadMQTTHealth: health hash not found after write")
	}
	if !got.Connected {
		t.Error("Connected: want true, got false")
	}
	if !got.Subscribed {
		t.Error("Subscribed: want true, got false")
	}
	if got.MsgCount != 42 {
		t.Errorf("MsgCount: got %d, want 42", got.MsgCount)
	}
}

// TestReadMQTTHealthMissing verifies that ReadMQTTHealth returns found=false when
// the key has never been written (simulates daemon not running or key expired).
func TestReadMQTTHealthMissing(t *testing.T) {
	s := newRedisStore(t)
	ctx := context.Background()

	_, found, err := s.ReadMQTTHealth(ctx)
	if err != nil {
		t.Fatalf("ReadMQTTHealth on empty store: %v", err)
	}
	if found {
		t.Error("expected found=false when key is absent, got found=true")
	}
}

// TestWriteMQTTHealthNotConnected verifies that connected=false is correctly stored
// and read back (the "daemon running but MQTT down" scenario).
func TestWriteMQTTHealthNotConnected(t *testing.T) {
	s := newRedisStore(t)
	ctx := context.Background()

	fields := store.MQTTHealthFields{
		Connected:  false,
		Subscribed: false,
		MsgCount:   0,
	}
	if err := s.WriteMQTTHealth(ctx, fields); err != nil {
		t.Fatalf("WriteMQTTHealth: %v", err)
	}

	got, found, err := s.ReadMQTTHealth(ctx)
	if err != nil {
		t.Fatalf("ReadMQTTHealth: %v", err)
	}
	if !found {
		t.Fatal("expected found=true, got false")
	}
	if got.Connected {
		t.Error("Connected: want false, got true")
	}
}
