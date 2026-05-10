package anthropicapi

import (
	"fmt"
	"io"
	"net/http"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// tlsClientAdapter wraps a bogdanfinn TLS-impersonation client and adapts it to
// the HTTPDoer interface which uses standard net/http types.
//
// The adapter converts *net/http.Request → *fhttp.Request (bogdanfinn fork) before
// calling the underlying impersonation client, then converts the *fhttp.Response
// back to a standard *net/http.Response. Header maps are compatible struct-for-struct
// since both use map[string][]string.
type tlsClientAdapter struct {
	inner tlsclient.HttpClient
}

// NewTLSClient creates an HTTPDoer backed by a TLS-impersonating client that mimics
// the Chrome 120 TLS fingerprint. Use this in production to bypass Cloudflare's
// TLS-fingerprint check on the claude.ai usage endpoint.
//
// The returned client satisfies HTTPDoer and can be passed directly to FetchUsage.
func NewTLSClient() (HTTPDoer, error) {
	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(30),
		tlsclient.WithClientProfile(profiles.Chrome_120),
		tlsclient.WithNotFollowRedirects(),
	}

	inner, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("anthropic api: create TLS client: %w", err)
	}

	return &tlsClientAdapter{inner: inner}, nil
}

// Do implements HTTPDoer. It converts the standard *http.Request to a *fhttp.Request,
// sends it through the TLS-impersonating client, then converts the response back to
// a standard *http.Response so callers can use the standard library types.
func (a *tlsClientAdapter) Do(req *http.Request) (*http.Response, error) {
	// Build the fhttp request from the standard request.
	fReq, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic api: convert request: %w", err)
	}

	// Copy headers.
	for k, vals := range req.Header {
		for _, v := range vals {
			fReq.Header.Add(k, v)
		}
	}

	fResp, err := a.inner.Do(fReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic api: tls request: %w", err)
	}

	// Convert fhttp.Response → net/http.Response.
	// Both share the same underlying types for headers (map[string][]string),
	// status code, and body (io.ReadCloser), so the conversion is straightforward.
	stdResp := &http.Response{
		Status:     fResp.Status,
		StatusCode: fResp.StatusCode,
		Header:     http.Header(fResp.Header),
		Body:       fResp.Body,
		// ContentLength is available on both.
		ContentLength: fResp.ContentLength,
	}

	// Ensure body is never nil (matches net/http contract).
	if stdResp.Body == nil {
		stdResp.Body = io.NopCloser(nil)
	}

	return stdResp, nil
}
