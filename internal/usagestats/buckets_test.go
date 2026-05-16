package usagestats_test

import (
	"testing"
	"time"

	"github.com/marioser/claude-mesh/internal/usagestats"
)

// makeTime creates a time.Time from a unix offset in seconds relative to a base.
func makeTime(baseUnix int64, offsetSecs int) time.Time {
	return time.Unix(baseUnix+int64(offsetSecs), 0)
}

// base is an arbitrary fixed time for test reproducibility.
var baseUnix = int64(1_700_000_000)

// TestFiveHourTokensBasic verifies that tokens within the last 5h are summed.
// Entry at now-4h (within window) should be counted; entry at now-6h should not.
func TestFiveHourTokensBasic(t *testing.T) {
	now := time.Unix(baseUnix, 0)
	entries := []usagestats.Entry{
		{Timestamp: makeTime(baseUnix, -6*3600), Tokens: 10_000}, // 6h ago — OUTSIDE
		{Timestamp: makeTime(baseUnix, -4*3600), Tokens: 20_000}, // 4h ago — inside
		{Timestamp: makeTime(baseUnix, -1*3600), Tokens: 30_000}, // 1h ago — inside
	}

	got := usagestats.FiveHourTokens(entries, now)

	if got != 50_000 {
		t.Errorf("FiveHourTokens: want 50_000 (20k+30k), got %d", got)
	}
}

// TestFiveHourTokensAllOutside verifies zero result when all entries are older than 5h.
func TestFiveHourTokensAllOutside(t *testing.T) {
	now := time.Unix(baseUnix, 0)
	entries := []usagestats.Entry{
		{Timestamp: makeTime(baseUnix, -6*3600), Tokens: 10_000},
		{Timestamp: makeTime(baseUnix, -10*3600), Tokens: 5_000},
	}

	got := usagestats.FiveHourTokens(entries, now)

	if got != 0 {
		t.Errorf("FiveHourTokens: want 0 when all outside window, got %d", got)
	}
}

// TestFiveHourTokensExactBoundary verifies that an entry exactly at now-5h is included.
func TestFiveHourTokensExactBoundary(t *testing.T) {
	now := time.Unix(baseUnix, 0)
	entries := []usagestats.Entry{
		{Timestamp: makeTime(baseUnix, -5*3600), Tokens: 7_000}, // exactly at boundary
	}

	got := usagestats.FiveHourTokens(entries, now)

	if got != 7_000 {
		t.Errorf("FiveHourTokens boundary: want 7_000, got %d", got)
	}
}

// TestFiveHourTokensEmpty verifies zero for empty entries slice.
func TestFiveHourTokensEmpty(t *testing.T) {
	now := time.Unix(baseUnix, 0)
	got := usagestats.FiveHourTokens(nil, now)
	if got != 0 {
		t.Errorf("FiveHourTokens empty: want 0, got %d", got)
	}
}

// TestWeekTokensBasic verifies that tokens within 7 days are summed.
// Entry at now-8d should be excluded; entries within 7d should be summed.
func TestWeekTokensBasic(t *testing.T) {
	now := time.Unix(baseUnix, 0)
	entries := []usagestats.Entry{
		{Timestamp: makeTime(baseUnix, -8*24*3600), Tokens: 100_000}, // 8 days ago — OUTSIDE
		{Timestamp: makeTime(baseUnix, -6*24*3600), Tokens: 50_000},  // 6 days ago — inside
		{Timestamp: makeTime(baseUnix, -1*24*3600), Tokens: 70_000},  // 1 day ago — inside
	}

	got := usagestats.WeekTokens(entries, now)

	if got != 120_000 {
		t.Errorf("WeekTokens: want 120_000 (50k+70k), got %d", got)
	}
}

// TestWeekTokensAllInside verifies all entries inside 7d window are summed.
func TestWeekTokensAllInside(t *testing.T) {
	now := time.Unix(baseUnix, 0)
	entries := []usagestats.Entry{
		{Timestamp: makeTime(baseUnix, -1*3600), Tokens: 10_000},
		{Timestamp: makeTime(baseUnix, -2*3600), Tokens: 20_000},
		{Timestamp: makeTime(baseUnix, -3*3600), Tokens: 30_000},
	}

	got := usagestats.WeekTokens(entries, now)

	if got != 60_000 {
		t.Errorf("WeekTokens all inside: want 60_000, got %d", got)
	}
}
