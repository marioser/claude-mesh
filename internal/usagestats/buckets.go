package usagestats

import "time"

const (
	fiveHours = 5 * time.Hour
	sevenDays = 7 * 24 * time.Hour
)

// Entry represents a single token-usage record extracted from a JSONL transcript.
type Entry struct {
	SessionID string
	Timestamp time.Time
	Tokens    int // input + output + cache_read + cache_creation
}

// FiveHourTokens returns the total tokens consumed in the rolling 5-hour window
// ending at `now`. Entries at exactly now-5h are included (>=).
func FiveHourTokens(entries []Entry, now time.Time) int {
	cutoff := now.Add(-fiveHours)
	total := 0
	for _, e := range entries {
		if !e.Timestamp.Before(cutoff) {
			total += e.Tokens
		}
	}
	return total
}

// WeekTokens returns the total tokens consumed in the rolling 7-day window
// ending at `now`. Entries at exactly now-7d are included (>=).
func WeekTokens(entries []Entry, now time.Time) int {
	cutoff := now.Add(-sevenDays)
	total := 0
	for _, e := range entries {
		if !e.Timestamp.Before(cutoff) {
			total += e.Tokens
		}
	}
	return total
}
