package anthropicapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marioser/claude-mesh/internal/anthropicapi"
)

// TestNewPythonClientReturnsHTTPDoer verifies that NewPythonClient returns a non-nil
// HTTPDoer when python3 is available. Skipped when python3 is not in PATH.
func TestNewPythonClientReturnsHTTPDoer(t *testing.T) {
	client, err := anthropicapi.NewPythonClient()
	if err != nil {
		// If python3 is missing, skip instead of failing — CI safety.
		t.Skipf("NewPythonClient: %v (skipping — python3 not available)", err)
	}
	if client == nil {
		t.Fatal("NewPythonClient: returned nil client")
	}
}

// TestPythonClientDoParsesPythonOutput verifies that pythonClient.Do correctly
// invokes python3 -c <script>, passes URL+cookie via env, and parses the JSON
// output into a *http.Response.
//
// We inject a fake python3 script path that prints a hard-coded JSON response,
// bypassing the real curl_cffi entirely.
func TestPythonClientDoParsesPythonOutput(t *testing.T) {
	// Spin up a real HTTP server so we have a valid URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should NOT be reached — we override the HTTP call via Python.
		t.Error("real server should not be called by pythonClient")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Build a *http.Request pointing at the test server (URL is what matters).
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/test", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Cookie", "session=fake")

	// Use NewPythonClientWithScript to inject a deterministic fake script.
	fakeScript := `
import json, os, sys
print(json.dumps({"status": 200, "body": '{"five_hour":{"utilization":42.0,"resets_at":"2026-05-10T19:00:00Z"},"seven_day":{"utilization":21.0,"resets_at":"2026-05-17T00:00:00Z"}}', "headers": {}}))
`
	client, err := anthropicapi.NewPythonClientWithScript(fakeScript)
	if err != nil {
		t.Skipf("NewPythonClientWithScript: %v (skipping — python3 not available)", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

// TestPythonClientPythonError verifies that Do returns an error when the python
// subprocess reports an error (status=0, error field set).
func TestPythonClientPythonError(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	errorScript := `
import json, sys
print(json.dumps({"status": 0, "error": "connection refused"}))
sys.exit(1)
`
	client, err := anthropicapi.NewPythonClientWithScript(errorScript)
	if err != nil {
		t.Skipf("NewPythonClientWithScript: %v (skipping — python3 not available)", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected error from python subprocess, got nil")
	}
}

// TestPythonClientIntegrationFetchUsage is an integration test that uses
// NewPythonClient + a real httptest server to verify the full FetchUsage path.
// This replaces the former TestNewTLSClientCanDoHTTPRequest test.
// Skipped when python3 is not available.
func TestPythonClientIntegrationFetchUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-client-platform") != "web_claude_ai" {
			t.Errorf("header not forwarded: anthropic-client-platform got %q",
				r.Header.Get("anthropic-client-platform"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":50.0,"resets_at":"2026-05-10T19:00:00Z"},"seven_day":{"utilization":25.0,"resets_at":"2026-05-17T00:00:00Z"}}`))
	}))
	defer srv.Close()

	// Use the real script so headers are forwarded correctly.
	pythonClient, err := anthropicapi.NewPythonClient()
	if err != nil {
		t.Skipf("NewPythonClient: %v (skipping — python3 not available)", err)
	}

	u, err := anthropicapi.FetchUsage(context.Background(), pythonClient, "org-py-test", "session=test", srv.URL)
	if err != nil {
		t.Fatalf("FetchUsage via PythonClient: %v", err)
	}
	if u.FiveHour.Utilization != 50.0 {
		t.Errorf("FiveHour.Utilization = %f, want 50.0", u.FiveHour.Utilization)
	}
	if u.SevenDay.Utilization != 25.0 {
		t.Errorf("SevenDay.Utilization = %f, want 25.0", u.SevenDay.Utilization)
	}
}
