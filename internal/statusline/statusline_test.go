package statusline_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"claude-mesh/internal/contextusage"
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

// zeroUsage is a helper for "no transcript info" scenario.
func zeroUsage() contextusage.Usage {
	return contextusage.Usage{}
}

// usage75 returns a Usage at 75% (150k/200k).
func usage75() contextusage.Usage {
	return contextusage.Usage{
		Tokens:  150000,
		Limit:   200000,
		Percent: 75.0,
		Method:  "usage",
		Source:  "transcript",
	}
}

// usage45 returns a Usage at 45% (90k/200k).
func usage45() contextusage.Usage {
	return contextusage.Usage{
		Tokens:  90000,
		Limit:   200000,
		Percent: 45.0,
		Method:  "usage",
		Source:  "transcript",
	}
}

// usage85 returns a Usage at 85% (170k/200k).
func usage85() contextusage.Usage {
	return contextusage.Usage{
		Tokens:  170000,
		Limit:   200000,
		Percent: 85.0,
		Method:  "usage",
		Source:  "transcript",
	}
}

// usage96 returns a Usage at 96% (192k/200k) — critical threshold.
func usage96() contextusage.Usage {
	return contextusage.Usage{
		Tokens:  192000,
		Limit:   200000,
		Percent: 96.0,
		Method:  "usage",
		Source:  "transcript",
	}
}

// TestRenderHappyPath verifies the full line with sessions, recent events, and context.
func TestRenderHappyPath(t *testing.T) {
	s := &fakeStore{
		sessionCount: 3,
		activities:   recentActivities(12),
	}
	in := statusline.Input{
		SessionID:      "sid-test",
		TranscriptPath: "",
		Cwd:            "/test",
	}
	line := statusline.Render(context.Background(), s, "feature/SMBX-177-claude-mesh", in, usage75())

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
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

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
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if !strings.Contains(line, "1 sesión") {
		t.Errorf("Render singular: want '1 sesión' in %q", line)
	}
	// Must NOT contain "1 sesiones"
	if strings.Contains(line, "1 sesiones") {
		t.Errorf("Render singular: must not contain '1 sesiones' in %q", line)
	}
}

// TestRenderDaemonDown verifies fallback when store returns an error (no context info).
func TestRenderDaemonDown(t *testing.T) {
	s := &fakeStore{
		healthErr: errors.New("connection refused"),
	}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

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

// TestRenderDaemonDownWithContext verifies context block is shown even when daemon is down.
func TestRenderDaemonDownWithContext(t *testing.T) {
	s := &fakeStore{
		healthErr: errors.New("connection refused"),
	}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, usage85())

	if !strings.Contains(line, "🧠") {
		t.Errorf("Render daemon down + context: want '🧠' in %q", line)
	}
	if !strings.Contains(line, "daemon down") {
		t.Errorf("Render daemon down + context: want 'daemon down' in %q", line)
	}
	if !strings.Contains(line, "🔴") {
		t.Errorf("Render daemon down + context: want '🔴' for 85%% in %q", line)
	}
}

// TestRenderBranchEmpty verifies branch falls back to "-" when empty.
func TestRenderBranchEmpty(t *testing.T) {
	s := &fakeStore{
		sessionCount: 2,
		activities:   recentActivities(3),
	}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "", in, zeroUsage())

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
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

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

	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(ctx, s, "develop", in, zeroUsage())

	// With canceled context, must return fallback line gracefully.
	if line == "" {
		t.Error("Render canceled context: expected non-empty fallback line, got empty string")
	}
	// Should contain the branch and fallback daemon state.
	if !strings.Contains(line, "🌳") {
		t.Errorf("Render canceled context: want '🌳' in fallback line %q", line)
	}
}

// TestRenderBranchWithChanges verifies "(N)" suffix when changes > 0.
func TestRenderBranchWithChanges(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: recentActivities(2)}
	in := statusline.Input{
		Cwd:     "/test",
		Changes: 5,
	}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if !strings.Contains(line, "(5)") {
		t.Errorf("Render with changes: want '(5)' in %q", line)
	}
}

// TestRenderBranchNoChanges verifies no "(0)" suffix when changes == 0.
func TestRenderBranchNoChanges(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{
		Cwd:     "/test",
		Changes: 0,
	}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if strings.Contains(line, "(0)") {
		t.Errorf("Render no changes: must not contain '(0)' in %q", line)
	}
}

// TestRenderContextGreen verifies 🟢 icon when usage < 60%.
func TestRenderContextGreen(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, usage45())

	if !strings.Contains(line, "🟢") {
		t.Errorf("Context green: want '🟢' in %q", line)
	}
	if !strings.Contains(line, "🧠") {
		t.Errorf("Context green: want '🧠' in %q", line)
	}
}

// TestRenderContextYellow verifies 🟡 icon when usage >= 60% and < 80%.
func TestRenderContextYellow(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, usage75())

	if !strings.Contains(line, "🟡") {
		t.Errorf("Context yellow: want '🟡' in %q", line)
	}
}

// TestRenderContextRed verifies 🔴 icon when usage >= 80% and < 95%.
func TestRenderContextRed(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, usage85())

	if !strings.Contains(line, "🔴") {
		t.Errorf("Context red: want '🔴' in %q", line)
	}
}

// TestRenderContextCritical verifies 🚨 icon when usage >= 95%.
func TestRenderContextCritical(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, usage96())

	if !strings.Contains(line, "🚨") {
		t.Errorf("Context critical: want '🚨' in %q", line)
	}
}

// TestRenderContextSkippedWhenZero verifies no 🧠 block when Usage.Tokens == 0.
func TestRenderContextSkippedWhenZero(t *testing.T) {
	s := &fakeStore{sessionCount: 2, activities: recentActivities(3)}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if strings.Contains(line, "🧠") {
		t.Errorf("Zero usage: must not contain '🧠' when Tokens=0, got %q", line)
	}
}

// TestRenderTokenFormatK verifies token formatting rounds to nearest k.
// 94312 → "94k", 123456 → "123k", 1234 → "1k".
func TestRenderTokenFormatK(t *testing.T) {
	cases := []struct {
		tokens  int
		wantStr string
	}{
		{94312, "94k"},
		{123456, "123k"},
		{1234, "1k"},
		{150000, "150k"},
	}
	s := &fakeStore{sessionCount: 1, activities: nil}

	for _, tc := range cases {
		u := contextusage.Usage{
			Tokens:  tc.tokens,
			Limit:   200000,
			Percent: float64(tc.tokens) / 200000.0 * 100.0,
			Method:  "usage",
			Source:  "transcript",
		}
		in := statusline.Input{Cwd: "/test"}
		line := statusline.Render(context.Background(), s, "develop", in, u)
		if !strings.Contains(line, tc.wantStr) {
			t.Errorf("Token format %d: want %q in %q", tc.tokens, tc.wantStr, line)
		}
	}
}

// TestRender1MLimit verifies the "1M" label in the context block when limit is 1_000_000.
// This is the bug regression test: 42% (426k/1M) instead of 100% (200k/200k).
func TestRender1MLimit(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	// Simulate 426k tokens with a 1M context window: ~42.6% → green icon.
	u := contextusage.Usage{
		Tokens:  426000,
		Limit:   1_000_000,
		Percent: 42.6,
		Method:  "usage",
		Source:  "transcript",
	}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, u)

	// Must show "1M" as the limit label.
	if !strings.Contains(line, "1M") {
		t.Errorf("1M limit: want '1M' in context block, got %q", line)
	}
	// Must NOT show "200k" as the limit.
	if strings.Contains(line, "200k") {
		t.Errorf("1M limit: must not show '200k' limit, got %q", line)
	}
	// At 42.6% → green icon 🟢.
	if !strings.Contains(line, "🟢") {
		t.Errorf("1M limit at 42%%: want '🟢' icon, got %q", line)
	}
}

// TestRender200kLimit verifies "200k" label when limit is 200_000 (standard models).
func TestRender200kLimit(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	u := contextusage.Usage{
		Tokens:  190000,
		Limit:   200_000,
		Percent: 95.0,
		Method:  "usage",
		Source:  "transcript",
	}
	in := statusline.Input{Cwd: "/test"}
	line := statusline.Render(context.Background(), s, "develop", in, u)

	if !strings.Contains(line, "200k") {
		t.Errorf("200k limit: want '200k' in context block, got %q", line)
	}
	// Must NOT show "1M".
	if strings.Contains(line, "1M") {
		t.Errorf("200k limit: must not contain '1M', got %q", line)
	}
}

// TestRenderWith1MModel verifies the 🤖 block appears with the correct short name for a 1M model.
func TestRenderWith1MModel(t *testing.T) {
	s := &fakeStore{sessionCount: 2, activities: recentActivities(5)}
	u := contextusage.Usage{
		Tokens:  426000,
		Limit:   1_000_000,
		Percent: 42.6,
		Method:  "usage",
		Source:  "transcript",
	}
	in := statusline.Input{
		Cwd: "/test",
		Model: statusline.Model{
			ID:          "claude-opus-4-7[1m]",
			DisplayName: "Opus 4.7 (1M context)",
		},
	}
	line := statusline.Render(context.Background(), s, "develop", in, u)

	if !strings.Contains(line, "🤖") {
		t.Errorf("Render 1M model: want '🤖' in %q", line)
	}
	if !strings.Contains(line, "Opus 4.7 (1M)") {
		t.Errorf("Render 1M model: want 'Opus 4.7 (1M)' in %q", line)
	}
}

// TestRenderWith200kModel verifies the 🤖 block shows the correct short name for a 200k model.
func TestRenderWith200kModel(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{
		Cwd: "/test",
		Model: statusline.Model{
			ID:          "claude-sonnet-4-6",
			DisplayName: "Claude Sonnet 4.6",
		},
	}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if !strings.Contains(line, "🤖") {
		t.Errorf("Render 200k model: want '🤖' in %q", line)
	}
	if !strings.Contains(line, "Sonnet 4.6") {
		t.Errorf("Render 200k model: want 'Sonnet 4.6' in %q", line)
	}
}

// TestRenderWithoutModel verifies no 🤖 block when Model is empty.
func TestRenderWithoutModel(t *testing.T) {
	s := &fakeStore{sessionCount: 1, activities: nil}
	in := statusline.Input{Cwd: "/test"} // zero Model{}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if strings.Contains(line, "🤖") {
		t.Errorf("Render no model: must not contain '🤖', got %q", line)
	}
}

// TestRenderDaemonDownWithModel verifies the 🤖 block appears even when daemon is down.
func TestRenderDaemonDownWithModel(t *testing.T) {
	s := &fakeStore{healthErr: errors.New("connection refused")}
	in := statusline.Input{
		Cwd: "/test",
		Model: statusline.Model{
			ID:          "claude-haiku-4-5-20251001",
			DisplayName: "Haiku 4.5",
		},
	}
	line := statusline.Render(context.Background(), s, "develop", in, zeroUsage())

	if !strings.Contains(line, "🤖") {
		t.Errorf("Render daemon down + model: want '🤖' in %q", line)
	}
	if !strings.Contains(line, "Haiku 4.5") {
		t.Errorf("Render daemon down + model: want 'Haiku 4.5' in %q", line)
	}
	if !strings.Contains(line, "daemon down") {
		t.Errorf("Render daemon down + model: want 'daemon down' in %q", line)
	}
}
