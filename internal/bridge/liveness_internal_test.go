package bridge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/mqtt"
	"github.com/marioser/claude-mesh/internal/store"
)

// silentSub is a minimal Subscriber that never delivers messages — bridge
// liveness tests do not need MQTT traffic.
type silentSub struct{}

func (silentSub) Subscribe(_ context.Context, _ mqtt.MessageHandler) error { return nil }

func newLivenessTestStore(t *testing.T) store.Store {
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

// withProcessChecker swaps the package-level liveness check for the test and
// restores the original on cleanup.
func withProcessChecker(t *testing.T, fn func(pid int) bool) {
	t.Helper()
	prev := processChecker
	processChecker = fn
	t.Cleanup(func() { processChecker = prev })
}

func openSession(t *testing.T, s store.Store, sid, host string, pid int, lastSeen time.Time) {
	t.Helper()
	ts := float64(lastSeen.UnixMilli())
	ev := events.SessionOpen{Ts: ts, SessionID: sid, Cwd: "/", Host: host, PID: pid}
	if err := s.OpenSession(context.Background(), ev); err != nil {
		t.Fatalf("OpenSession %q: %v", sid, err)
	}
	// OpenSession stamps last_seen with ev.Ts, so the ZSET score is already stale
	// when lastSeen is in the past.
}

// TestRefreshLiveSessionsKeepsLocalAliveSession verifies that a session whose
// host matches the bridge's hostname and whose PID is reported alive gets its
// ZSET score refreshed during the sweep tick, even when the prior score is
// well past the configured TTL.
func TestRefreshLiveSessionsKeepsLocalAliveSession(t *testing.T) {
	withProcessChecker(t, func(_ int) bool { return true })

	s := newLivenessTestStore(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}

	// TTL: 1s. Open the session 10s in the past so its score is far below cutoff.
	b := NewWithConfig(silentSub{}, s, nil, 50*time.Millisecond, 1*time.Second)

	openSession(t, s, "live-local", host, os.Getpid(), time.Now().Add(-10*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)

	// Allow at least 4 sweep ticks (50ms each) so refresh has a chance to run.
	time.Sleep(250 * time.Millisecond)

	sessions, err := s.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	var found bool
	for _, sess := range sessions {
		if sess.ID == "live-local" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected live local session to be retained by liveness refresh, but it was evicted")
	}
}

// TestRefreshLiveSessionsSkipsRemoteHost verifies that a session whose host
// does NOT match the bridge's hostname is never probed by the liveness check
// and is evicted purely by TTL.
func TestRefreshLiveSessionsSkipsRemoteHost(t *testing.T) {
	probed := false
	withProcessChecker(t, func(_ int) bool {
		probed = true
		return true
	})

	s := newLivenessTestStore(t)
	b := NewWithConfig(silentSub{}, s, nil, 50*time.Millisecond, 1*time.Second)

	openSession(t, s, "remote-sess", "definitely-not-this-host", 1, time.Now().Add(-10*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)

	time.Sleep(250 * time.Millisecond)

	if probed {
		t.Error("liveness check probed a remote-host session — host filter is broken")
	}

	sessions, err := s.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	for _, sess := range sessions {
		if sess.ID == "remote-sess" {
			t.Error("remote-host session not evicted by TTL")
		}
	}
}

// TestRefreshLiveSessionsEvictsDeadLocalProcess verifies that a session whose
// host matches but whose PID is reported dead is left alone by refresh and
// therefore evicted by TTL on the next sweep.
func TestRefreshLiveSessionsEvictsDeadLocalProcess(t *testing.T) {
	withProcessChecker(t, func(_ int) bool { return false })

	s := newLivenessTestStore(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}

	b := NewWithConfig(silentSub{}, s, nil, 50*time.Millisecond, 1*time.Second)

	openSession(t, s, "dead-local", host, 999999, time.Now().Add(-10*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)

	time.Sleep(250 * time.Millisecond)

	sessions, err := s.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	for _, sess := range sessions {
		if sess.ID == "dead-local" {
			t.Error("dead-local session was retained despite processChecker returning false")
		}
	}
}

// TestIsProcessAliveOurselves cross-checks the real isProcessAlive implementation
// against the running test process. This guards against regressions in the
// platform-specific liveness implementation.
func TestIsProcessAliveOurselves(t *testing.T) {
	if !isProcessAlive(os.Getpid()) {
		t.Error("isProcessAlive returned false for the running test process")
	}
	if isProcessAlive(0) {
		t.Error("isProcessAlive returned true for pid 0")
	}
	if isProcessAlive(-1) {
		t.Error("isProcessAlive returned true for negative pid")
	}
}
