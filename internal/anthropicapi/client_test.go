package anthropicapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"claude-mesh/internal/anthropicapi"
)

// TestNewTLSClientCreatesHTTPDoer verifies that NewTLSClient returns a non-nil
// HTTPDoer and does not error on creation.
func TestNewTLSClientCreatesHTTPDoer(t *testing.T) {
	client, err := anthropicapi.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("NewTLSClient: returned nil client")
	}
}

// TestNewTLSClientCanDoHTTPRequest verifies the adapter can perform a real HTTP
// request against a local httptest server (no TLS fingerprint checks apply there).
// This validates the request/response conversion pipeline end-to-end.
func TestNewTLSClientCanDoHTTPRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify our custom headers survive the adapter conversion.
		if r.Header.Get("anthropic-client-platform") != "web_claude_ai" {
			t.Errorf("header not forwarded: anthropic-client-platform got %q", r.Header.Get("anthropic-client-platform"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":50.0,"resets_at":"2026-05-10T19:00:00Z"},"seven_day":{"utilization":25.0,"resets_at":"2026-05-17T00:00:00Z"}}`))
	}))
	defer srv.Close()

	// Use a plain *http.Client for this test (targets the httptest server directly).
	// The TLS client can also hit plain HTTP servers.
	tlsClient, err := anthropicapi.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	u, err := anthropicapi.FetchUsage(context.Background(), tlsClient, "org-tls-test", "session=test", srv.URL)
	if err != nil {
		t.Fatalf("FetchUsage via TLS client: %v", err)
	}
	if u.FiveHour.Utilization != 50.0 {
		t.Errorf("FiveHour.Utilization = %f, want 50.0", u.FiveHour.Utilization)
	}
	if u.SevenDay.Utilization != 25.0 {
		t.Errorf("SevenDay.Utilization = %f, want 25.0", u.SevenDay.Utilization)
	}
}
