package statusline_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"claude-mesh/internal/events"
	"claude-mesh/internal/statusline"
	"claude-mesh/internal/store"
)

// fakeStore implements store.Store for statusline tests.
type fakeStore struct {
	sessionCount int
	activities   []events.Activity
	healthErr    error
}

func (f *fakeStore) OpenSession(_ context.Context, _ events.SessionOpen) error { return nil }
func (f *fakeStore) TouchSession(_ context.Context, _ string, _ float64) error { return nil }
func (f *fakeStore) CloseSession(_ context.Context, _ string) error            { return nil }
func (f *fakeStore) GetSession(_ context.Context, _ string) (*store.SessionView, error) {
	return nil, nil
}
func (f *fakeStore) ListActiveSessions(_ context.Context) ([]store.SessionView, error) {
	if f.healthErr != nil {
		return nil, f.healthErr
	}
	sessions := make([]store.SessionView, f.sessionCount)
	for i := range sessions {
		sessions[i] = store.SessionView{ID: "sid-" + string(rune('0'+i))}
	}
	return sessions, nil
}
func (f *fakeStore) PushActivity(_ context.Context, _ events.Activity) error { return nil }
func (f *fakeStore) RecentActivity(_ context.Context, limit int, sid string) ([]events.Activity, error) {
	if f.healthErr != nil {
		return nil, f.healthErr
	}
	if sid != "" {
		return nil, nil // statusline only uses global ring
	}
	return f.activities, nil
}
func (f *fakeStore) SweepExpired(_ context.Context, _ float64) (int, error) { return 0, nil }
func (f *fakeStore) HealthCheck(_ context.Context) error                     { return f.healthErr }

// recentActivities builds a slice of Activity events with timestamps within 5 minutes.
func recentActivities(n int) []events.Activity {
	acts := make([]events.Activity, n)
	nowMs := float64(time.Now().UnixMilli())
	for i := range acts {
		acts[i] = events.Activity{
			SessionID: "sid-0",
			Tool:      "Edit",
			Ts:        nowMs - float64(i*1000), // within 5m window
		}
	}
	return acts
}

// staleActivities builds activities older than 5 minutes.
func staleActivities(n int) []events.Activity {
	acts := make([]events.Activity, n)
	sixMinAgoMs := float64(time.Now().UnixMilli()) - 360_000
	for i := range acts {
		acts[i] = events.Activity{
			SessionID: "sid-0",
			Tool:      "Edit",
			Ts:        sixMinAgoMs - float64(i*1000),
		}
	}
	return acts
}

// TestRenderHappyPath verifies the full line with sessions and recent events.
func TestRenderHappyPath(t *testing.T) {
	s := &fakeStore{
		sessionCount: 3,
		activities:   recentActivities(12),
	}
	line := statusline.Render(context.Background(), s, "feature/SMBX-177-claude-mesh")

	wantParts := []string{
		"🌳", "feature/SMBX-177-claude-mesh",
		"🔵", "3 sesiones",
		"📡", "12 eventos/5m",
		"✅", "daemon",
	}
	for _, part := range wantParts {
		if !strings.Contains(line, part) {
			t.Errorf("Render happy path: missing %q in %q", part, line)
		}
	}
}

// TestRenderEmpty verifies 0 sessions and 0 events.
func TestRenderEmpty(t *testing.T) {
	s := &fakeStore{
		sessionCount: 0,
		activities:   nil,
	}
	line := statusline.Render(context.Background(), s, "develop")

	if !strings.Contains(line, "0 sesiones") {
		t.Errorf("Render empty: want '0 sesiones' in %q", line)
	}
	if !strings.Contains(line, "0 eventos/5m") {
		t.Errorf("Render empty: want '0 eventos/5m' in %q", line)
	}
	if !strings.Contains(line, "✅") {
		t.Errorf("Render empty: want '✅' (daemon healthy) in %q", line)
	}
}

// TestRenderSingular verifies the singular "sesión" form for exactly 1 session.
func TestRenderSingular(t *testing.T) {
	s := &fakeStore{
		sessionCount: 1,
		activities:   recentActivities(5),
	}
	line := statusline.Render(context.Background(), s, "develop")

	if !strings.Contains(line, "1 sesión") {
		t.Errorf("Render singular: want '1 sesión' in %q", line)
	}
	// Must NOT contain "1 sesiones"
	if strings.Contains(line, "1 sesiones") {
		t.Errorf("Render singular: must not contain '1 sesiones' in %q", line)
	}
}

// TestRenderDaemonDown verifies fallback when store returns an error.
func TestRenderDaemonDown(t *testing.T) {
	s := &fakeStore{
		healthErr: errors.New("connection refused"),
	}
	line := statusline.Render(context.Background(), s, "develop")

	// Must contain fallback indicators.
	if !strings.Contains(line, "⚪") {
		t.Errorf("Render daemon down: want '⚪' in %q", line)
	}
	if !strings.Contains(line, "daemon down") {
		t.Errorf("Render daemon down: want 'daemon down' in %q", line)
	}
	// Must NOT contain healthy session count.
	if strings.Contains(line, "sesion") {
		t.Errorf("Render daemon down: must not contain session count in %q", line)
	}
}

// TestRenderBranchEmpty verifies branch falls back to "-" when empty.
func TestRenderBranchEmpty(t *testing.T) {
	s := &fakeStore{
		sessionCount: 2,
		activities:   recentActivities(3),
	}
	line := statusline.Render(context.Background(), s, "")

	if !strings.Contains(line, "🌳 -") {
		t.Errorf("Render no branch: want '🌳 -' in %q", line)
	}
}

// TestRenderOnlyRecentEvents verifies only events within 5min window are counted.
func TestRenderOnlyRecentEvents(t *testing.T) {
	recent := recentActivities(4)
	stale := staleActivities(10)
	all := append(recent, stale...)

	s := &fakeStore{
		sessionCount: 1,
		activities:   all,
	}
	line := statusline.Render(context.Background(), s, "develop")

	// Only the 4 recent events should be counted.
	if !strings.Contains(line, "4 eventos/5m") {
		t.Errorf("Render stale filter: want '4 eventos/5m' in %q", line)
	}
}

// TestRenderCanceledContext verifies fallback when context is already canceled (timeout).
func TestRenderCanceledContext(t *testing.T) {
	s := &fakeStore{sessionCount: 2, activities: recentActivities(5)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	line := statusline.Render(ctx, s, "develop")

	// With canceled context, must return fallback line gracefully.
	if line == "" {
		t.Error("Render canceled context: expected non-empty fallback line, got empty string")
	}
	// Should contain the branch and fallback daemon state.
	if !strings.Contains(line, "🌳") {
		t.Errorf("Render canceled context: want '🌳' in fallback line %q", line)
	}
}
