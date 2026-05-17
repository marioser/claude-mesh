package bridge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/marioser/claude-mesh/internal/bridge"
	"github.com/marioser/claude-mesh/internal/events"
	"github.com/marioser/claude-mesh/internal/mqtt"
	"github.com/marioser/claude-mesh/internal/store"
)

// fakeSubscriber records subscriptions and allows triggering messages.
type fakeSubscriber struct {
	mu       sync.Mutex
	handlers []mqtt.MessageHandler

	// failFirst is the number of Subscribe calls that return an error before succeeding.
	failFirst int
	callCount int
	failErr   error
}

func (f *fakeSubscriber) Subscribe(_ context.Context, handler mqtt.MessageHandler) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	if f.failFirst > 0 && f.callCount <= f.failFirst {
		return f.failErr
	}
	f.handlers = append(f.handlers, handler)
	return nil
}

// Send delivers a message to all registered handlers.
func (f *fakeSubscriber) Send(topic string, payload []byte) {
	f.mu.Lock()
	handlers := make([]mqtt.MessageHandler, len(f.handlers))
	copy(handlers, f.handlers)
	f.mu.Unlock()
	for _, h := range handlers {
		h(topic, payload)
	}
}

// newTestStore creates a miniredis-backed Store for bridge tests.
func newTestStore(t *testing.T) store.Store {
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

// nowMs returns current timestamp as float64 milliseconds.
func nowMs() float64 {
	return float64(time.Now().UnixMilli())
}

// TestSessionOpenEventStored verifies that a session-open MQTT message results
// in the session being stored in Redis via OpenSession.
func TestSessionOpenEventStored(t *testing.T) {
	fakeSub := &fakeSubscriber{}
	s := newTestStore(t)
	b := bridge.New(fakeSub, s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)

	// Give the bridge time to subscribe.
	time.Sleep(20 * time.Millisecond)

	ev := events.SessionOpen{
		Ts:        nowMs(),
		SessionID: "bridge-sess-1",
		Cwd:       "/bridge-test",
		Host:      "host1",
		PID:       99,
	}
	payload, _ := json.Marshal(ev)
	fakeSub.Send("claude/mesh/session/bridge-sess-1/open", payload)

	// Wait for async processing.
	time.Sleep(100 * time.Millisecond)

	view, err := s.GetSession(context.Background(), "bridge-sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if view == nil {
		t.Fatal("session not stored after session-open event")
	}
	if view.Cwd != "/bridge-test" {
		t.Errorf("Cwd: got %q, want %q", view.Cwd, "/bridge-test")
	}
}

// TestActivityEventRecorded verifies that an activity MQTT message results in
// PushActivity being called and the event appearing in the global ring.
func TestActivityEventRecorded(t *testing.T) {
	fakeSub := &fakeSubscriber{}
	s := newTestStore(t)
	b := bridge.New(fakeSub, s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Open a session first.
	openEv := events.SessionOpen{Ts: nowMs(), SessionID: "act-sess", Cwd: "/p", Host: "h", PID: 1}
	openPayload, _ := json.Marshal(openEv)
	fakeSub.Send("claude/mesh/session/act-sess/open", openPayload)
	time.Sleep(50 * time.Millisecond)

	act := events.Activity{
		Ts:        nowMs(),
		SessionID: "act-sess",
		Tool:      "Edit",
		Target:    "main.go",
		Cwd:       "/p",
	}
	actPayload, _ := json.Marshal(act)
	fakeSub.Send("claude/mesh/session/act-sess/activity", actPayload)
	time.Sleep(100 * time.Millisecond)

	global, err := s.RecentActivity(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("RecentActivity global: %v", err)
	}
	if len(global) == 0 {
		t.Error("no activity events in global ring after activity event")
	}
	if global[0].Tool != "Edit" {
		t.Errorf("Tool: got %q, want %q", global[0].Tool, "Edit")
	}
}

// TestSessionCloseEventHandled verifies that a session-close MQTT message results
// in the session being removed from the active ZSET.
func TestSessionCloseEventHandled(t *testing.T) {
	fakeSub := &fakeSubscriber{}
	s := newTestStore(t)
	b := bridge.New(fakeSub, s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Open a session first.
	openEv := events.SessionOpen{Ts: nowMs(), SessionID: "close-sess", Cwd: "/p", Host: "h", PID: 1}
	openPayload, _ := json.Marshal(openEv)
	fakeSub.Send("claude/mesh/session/close-sess/open", openPayload)
	time.Sleep(50 * time.Millisecond)

	// Close the session.
	closeEv := events.SessionClose{Ts: nowMs(), SessionID: "close-sess", Reason: "stop"}
	closePayload, _ := json.Marshal(closeEv)
	fakeSub.Send("claude/mesh/session/close-sess/close", closePayload)
	time.Sleep(100 * time.Millisecond)

	sessions, err := s.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	for _, sess := range sessions {
		if sess.ID == "close-sess" {
			t.Error("session still in active ZSET after session-close event")
		}
	}
}

// TestMalformedPayloadNoPanic verifies that a malformed JSON payload does not
// cause a panic or Store write.
func TestMalformedPayloadNoPanic(t *testing.T) {
	fakeSub := &fakeSubscriber{}
	s := newTestStore(t)
	b := bridge.New(fakeSub, s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Send malformed payload — bridge must not panic.
	fakeSub.Send("claude/mesh/session/bad-sess/open", []byte("NOT JSON {{{"))
	time.Sleep(100 * time.Millisecond)

	// Session must NOT have been written.
	view, err := s.GetSession(context.Background(), "bad-sess")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if view != nil {
		t.Error("session written to Redis despite malformed JSON payload")
	}
}

// TestBridgeBootRetryOnSubscribeFailure verifies that bridge.Run retries Subscribe
// with exponential backoff when the initial subscribe returns an error, and eventually
// processes messages once subscribe succeeds (Bug 2 fix).
func TestBridgeBootRetryOnSubscribeFailure(t *testing.T) {
	errors := fmt.Errorf("subscribe temporarily unavailable")
	fakeSub := &fakeSubscriber{
		failFirst: 2,     // fail the first 2 Subscribe calls, succeed on 3rd
		failErr:   errors,
	}
	s := newTestStore(t)

	// Use a short sweep interval; also bridge.Run's retry backoff starts at 1s by default.
	// We cannot override the retry interval via the public API, so we accept that this
	// test takes ~3s (1s + 2s backoff). To keep tests fast we use t.Parallel() and a
	// generous deadline.
	b := bridge.New(fakeSub, s, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		b.Run(ctx)
		close(done)
	}()

	// Wait until subscribe succeeds (fakeSub has a handler registered).
	deadline := time.After(8 * time.Second)
	for {
		fakeSub.mu.Lock()
		ready := len(fakeSub.handlers) > 0
		fakeSub.mu.Unlock()
		if ready {
			break
		}
		select {
		case <-deadline:
			t.Fatal("bridge never subscribed successfully within 8s")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Verify the bridge retried: callCount should be 3 (2 failures + 1 success).
	fakeSub.mu.Lock()
	count := fakeSub.callCount
	fakeSub.mu.Unlock()
	if count < 3 {
		t.Errorf("Subscribe call count: got %d, want >= 3 (2 failures + 1 success)", count)
	}

	// Now send a message and verify it is processed.
	openEv := events.SessionOpen{Ts: nowMs(), SessionID: "retry-sess", Cwd: "/p", Host: "h", PID: 1}
	payload, _ := json.Marshal(openEv)
	fakeSub.Send("claude/mesh/session/retry-sess/open", payload)
	time.Sleep(200 * time.Millisecond)

	view, err := s.GetSession(context.Background(), "retry-sess")
	if err != nil {
		t.Fatalf("GetSession after retry: %v", err)
	}
	if view == nil {
		t.Error("session not stored after bridge recovered from subscribe failure")
	}

	cancel()
	<-done
}

// TestSweepTickerFires verifies that the bridge's sweep ticker goroutine evicts
// stale sessions from the ZSET when it fires.
func TestSweepTickerFires(t *testing.T) {
	fakeSub := &fakeSubscriber{}
	s := newTestStore(t)

	// Use a very short sweep interval (50ms) and a short session TTL (1s) so the
	// test doesn't have to wait for the production default to elapse.
	b := bridge.NewWithConfig(fakeSub, s, nil, 50*time.Millisecond, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go b.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Open a session with a stale timestamp older than the configured TTL.
	staleTs := float64(time.Now().Add(-5 * time.Second).UnixMilli())
	openEv := events.SessionOpen{Ts: staleTs, SessionID: "stale-sweep", Cwd: "/", Host: "h", PID: 1}
	openPayload, _ := json.Marshal(openEv)
	fakeSub.Send("claude/mesh/session/stale-sweep/open", openPayload)
	time.Sleep(50 * time.Millisecond)

	// Wait long enough for at least one sweep tick.
	time.Sleep(200 * time.Millisecond)

	sessions, err := s.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	for _, sess := range sessions {
		if sess.ID == "stale-sweep" {
			t.Error("stale session still in ZSET after sweep tick")
		}
	}
}
