package anthropicapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"claude-mesh/internal/anthropicapi"
)

// responseJSON builds a usage response JSON for the mock server.
func responseJSON(t *testing.T, fiveHourPct, sevenDayPct float64, resetsAt string) string {
	t.Helper()
	return `{
		"five_hour":   {"utilization":` + jsonFloat(fiveHourPct) + `,"resets_at":"` + resetsAt + `"},
		"seven_day":   {"utilization":` + jsonFloat(sevenDayPct) + `,"resets_at":"` + resetsAt + `"},
		"seven_day_sonnet": {"utilization":3.0,"resets_at":"` + resetsAt + `"},
		"seven_day_opus": null,
		"extra_usage": {"is_enabled":false}
	}`
}

func jsonFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// TestFetchUsageHappyPath verifies that a 200 response is fully parsed.
func TestFetchUsageHappyPath(t *testing.T) {
	resetsAt := "2026-05-10T19:00:00Z"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify required headers.
		if r.Header.Get("anthropic-client-platform") != "web_claude_ai" {
			t.Errorf("missing anthropic-client-platform header, got %q", r.Header.Get("anthropic-client-platform"))
		}
		if r.Header.Get("anthropic-client-version") != "1.0.0" {
			t.Errorf("missing anthropic-client-version header")
		}
		if r.Header.Get("Cookie") == "" {
			t.Errorf("Cookie header not set")
		}
		if r.Header.Get("accept") != "*/*" {
			t.Errorf("missing accept header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseJSON(t, 23.0, 12.0, resetsAt)))
	}))
	defer srv.Close()

	client := srv.Client()
	u, err := anthropicapi.FetchUsage(context.Background(), client, "org-123", "session=abc123", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.FiveHour.Utilization != 23.0 {
		t.Errorf("FiveHour.Utilization = %f, want 23.0", u.FiveHour.Utilization)
	}
	if u.SevenDay.Utilization != 12.0 {
		t.Errorf("SevenDay.Utilization = %f, want 12.0", u.SevenDay.Utilization)
	}
	expected, _ := time.Parse(time.RFC3339, resetsAt)
	if !u.FiveHour.ResetsAt.Equal(expected) {
		t.Errorf("FiveHour.ResetsAt = %v, want %v", u.FiveHour.ResetsAt, expected)
	}
	// SevenDaySonnet should be populated (pointer to Period).
	if u.SevenDaySonnet == nil {
		t.Error("SevenDaySonnet should not be nil")
	} else if u.SevenDaySonnet.Utilization != 3.0 {
		t.Errorf("SevenDaySonnet.Utilization = %f, want 3.0", u.SevenDaySonnet.Utilization)
	}
	// SevenDayOpus is null in the response — should be nil pointer.
	if u.SevenDayOpus != nil {
		t.Errorf("SevenDayOpus should be nil (was null in response)")
	}
}

// TestFetchUsage401ReturnsErrAuthFailed verifies that a 401 response maps to ErrAuthFailed.
func TestFetchUsage401ReturnsErrAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := anthropicapi.FetchUsage(context.Background(), srv.Client(), "org-123", "bad-cookie", srv.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != anthropicapi.ErrAuthFailed {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

// TestFetchUsage404ReturnsErrNotFound verifies that a 404 response maps to ErrNotFound.
func TestFetchUsage404ReturnsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := anthropicapi.FetchUsage(context.Background(), srv.Client(), "org-123", "cookie", srv.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != anthropicapi.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestFetchUsageMalformedJSONReturnsParseError verifies bad JSON returns an error.
func TestFetchUsageMalformedJSONReturnsParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	_, err := anthropicapi.FetchUsage(context.Background(), srv.Client(), "org-123", "cookie", srv.URL)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	// Must NOT be one of the sentinel errors.
	if err == anthropicapi.ErrAuthFailed || err == anthropicapi.ErrNotFound {
		t.Errorf("expected generic parse error, got sentinel %v", err)
	}
}

// TestFetchUsageContextCancelledReturnsError verifies that a cancelled context propagates.
func TestFetchUsageContextCancelledReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate slow response: sleep longer than the ctx deadline.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := anthropicapi.FetchUsage(ctx, srv.Client(), "org-123", "cookie", srv.URL)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestFetchUsageURLConstruction verifies the request URL contains the orgID path segment.
func TestFetchUsageURLConstruction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/organizations/my-org-456/usage" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseJSON(t, 10.0, 5.0, "2026-05-10T00:00:00Z")))
	}))
	defer srv.Close()

	_, err := anthropicapi.FetchUsage(context.Background(), srv.Client(), "my-org-456", "c=1", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
