package anthropicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	defaultBaseURL = "https://claude.ai"
)

// HTTPDoer is a minimal interface satisfied by *http.Client and by the TLS-impersonating
// client returned by NewTLSClient. Using an interface keeps FetchUsage testable with a
// plain httptest.Server without needing the real bogdanfinn stack.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// FetchUsage calls the Anthropic usage endpoint and returns parsed usage data.
//
// The baseURL parameter is the scheme+host (e.g. "https://claude.ai"). In tests,
// pass the httptest.Server URL. In production, pass "" to use the default.
//
// The client parameter is either a *http.Client (tests) or a TLS-impersonating
// client created with NewTLSClient (production). Both satisfy HTTPDoer.
//
// Errors:
//   - ErrAuthFailed for 401 responses (cookie expired or invalid)
//   - ErrNotFound for 404 responses (wrong org_id)
//   - other errors for network or JSON parsing failures
func FetchUsage(ctx context.Context, client HTTPDoer, orgID, cookie string, baseURL string) (Usage, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	url := fmt.Sprintf("%s/api/organizations/%s/usage", baseURL, orgID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Usage{}, fmt.Errorf("anthropic api: build request: %w", err)
	}

	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-language", "es-419,es-US;q=0.9,es;q=0.8,en;q=0.7")
	req.Header.Set("anthropic-client-platform", "web_claude_ai")
	req.Header.Set("anthropic-client-version", "1.0.0")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="120", "Not.A/Brand";v="8", "Chromium";v="120"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "macOS")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("referer", "https://claude.ai/settings/usage")
	req.Header.Set("Cookie", cookie)

	resp, err := client.Do(req)
	if err != nil {
		return Usage{}, fmt.Errorf("anthropic api: http request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to parsing
	case http.StatusUnauthorized:
		return Usage{}, ErrAuthFailed
	case http.StatusNotFound:
		return Usage{}, ErrNotFound
	default:
		return Usage{}, fmt.Errorf("anthropic api: unexpected status %d", resp.StatusCode)
	}

	var u Usage
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return Usage{}, fmt.Errorf("anthropic api: parse response: %w", err)
	}

	return u, nil
}
