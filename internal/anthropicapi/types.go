// Package anthropicapi provides a client for the claude.ai internal usage API.
//
// Authentication uses the browser session cookie from claude.ai. When the cookie
// expires, FetchUsage returns ErrAuthFailed and the caller should fall back to
// the local JSONL parsing method.
package anthropicapi

import (
	"errors"
	"time"
)

// ErrAuthFailed is returned when the API responds with HTTP 401 — cookie expired or invalid.
var ErrAuthFailed = errors.New("anthropic api: auth failed (cookie expired or invalid)")

// ErrNotFound is returned when the API responds with HTTP 404 — wrong org_id.
var ErrNotFound = errors.New("anthropic api: org not found")

// Period represents one rolling usage window returned by the API.
type Period struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"` // RFC3339
}

// Usage holds the full response payload from the Anthropic usage endpoint.
type Usage struct {
	FiveHour       Period  `json:"five_hour"`
	SevenDay       Period  `json:"seven_day"`
	SevenDayOpus   *Period `json:"seven_day_opus"`   // null in API → nil pointer
	SevenDaySonnet *Period `json:"seven_day_sonnet"` // null in API → nil pointer
}
