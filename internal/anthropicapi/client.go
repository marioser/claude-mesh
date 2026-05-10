package anthropicapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// pythonClient implements HTTPDoer by shelling out to python3 + curl_cffi.
// This is the only reliable way to bypass Cloudflare's TLS fingerprint check
// on the claude.ai usage endpoint without deep TLS stack engineering in Go.
//
// Each call to Do() executes a short Python script via exec.Command, passing the
// target URL and Cookie through environment variables. The script returns a JSON
// envelope: {"status": <int>, "body": "<string>", "headers": {}, "error": "<string>"}.
type pythonClient struct {
	pythonPath string
	script     string
}

// defaultPythonScript is the curl_cffi script executed for each Do() call.
// URL and Cookie are injected via CLAUDE_MESH_TARGET_URL and CLAUDE_MESH_TARGET_COOKIE
// environment variables so they never appear in process argument lists.
const defaultPythonScript = `
import json, sys, os
from curl_cffi import requests
url = os.environ.get('CLAUDE_MESH_TARGET_URL', '')
cookie = os.environ.get('CLAUDE_MESH_TARGET_COOKIE', '')
headers_env = os.environ.get('CLAUDE_MESH_TARGET_HEADERS', '{}')
try:
    extra_headers = json.loads(headers_env)
except Exception:
    extra_headers = {}
headers = {
    'accept': '*/*',
    'accept-language': 'es-419,es-US;q=0.9,es;q=0.8,en;q=0.7',
    'anthropic-client-platform': 'web_claude_ai',
    'anthropic-client-version': '1.0.0',
    'sec-ch-ua': '"Google Chrome";v="120", "Not.A/Brand";v="8", "Chromium";v="120"',
    'sec-ch-ua-mobile': '?0',
    'sec-ch-ua-platform': '"macOS"',
    'sec-fetch-dest': 'empty',
    'sec-fetch-mode': 'cors',
    'sec-fetch-site': 'same-origin',
    'referer': 'https://claude.ai/settings/usage',
    'cookie': cookie,
}
headers.update(extra_headers)
try:
    r = requests.get(url, headers=headers, impersonate='chrome120', timeout=15)
    out = {'status': r.status_code, 'headers': dict(r.headers), 'body': r.text}
    print(json.dumps(out))
except Exception as e:
    print(json.dumps({'status': 0, 'error': str(e)}))
    sys.exit(1)
`

// NewPythonClient creates an HTTPDoer backed by python3 + curl_cffi.
// It verifies python3 is in PATH and auto-installs curl_cffi if missing.
// Returns an error if python3 is not available.
func NewPythonClient() (HTTPDoer, error) {
	return newPythonClient(defaultPythonScript)
}

// NewPythonClientWithScript creates an HTTPDoer that runs the given Python script
// instead of the default curl_cffi script. Used in tests to inject deterministic
// fake responses without needing real curl_cffi.
func NewPythonClientWithScript(script string) (HTTPDoer, error) {
	return newPythonClient(script)
}

func newPythonClient(script string) (*pythonClient, error) {
	p, err := exec.LookPath("python3")
	if err != nil {
		return nil, fmt.Errorf("python3 not found in PATH: %w", err)
	}

	pc := &pythonClient{pythonPath: p, script: script}

	// Only ensure curl_cffi when using the default script (production path).
	if script == defaultPythonScript {
		if err := pc.ensureCurlCffi(); err != nil {
			return nil, err
		}
	}

	return pc, nil
}

// ensureCurlCffi checks if curl_cffi is importable and auto-installs it if not.
func (p *pythonClient) ensureCurlCffi() error {
	checkCmd := exec.Command(p.pythonPath, "-c", "import curl_cffi")
	if err := checkCmd.Run(); err == nil {
		return nil // already installed
	}

	installCmd := exec.Command(p.pythonPath, "-m", "pip", "install", "--user", "--quiet", "curl_cffi")
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("anthropic api: install curl_cffi: %w\noutput: %s", err, out)
	}
	return nil
}

// Do implements HTTPDoer. It executes the Python script as a subprocess,
// passing the request URL and Cookie header via environment variables.
// The script must print a single JSON line: {"status": N, "body": "...", "headers": {}}
// or {"status": 0, "error": "..."} on failure.
func (p *pythonClient) Do(req *http.Request) (*http.Response, error) {
	cookie := req.Header.Get("Cookie")

	// Serialize non-cookie headers to JSON so the script can merge them.
	extraHeaders := make(map[string]string)
	for k, vals := range req.Header {
		lk := strings.ToLower(k)
		if lk == "cookie" {
			continue // passed separately
		}
		if len(vals) > 0 {
			extraHeaders[k] = vals[0]
		}
	}
	headersJSON, err := json.Marshal(extraHeaders)
	if err != nil {
		headersJSON = []byte("{}")
	}

	cmd := exec.CommandContext(req.Context(), p.pythonPath, "-c", p.script)
	cmd.Env = append(os.Environ(),
		"CLAUDE_MESH_TARGET_URL="+req.URL.String(),
		"CLAUDE_MESH_TARGET_COOKIE="+cookie,
		"CLAUDE_MESH_TARGET_HEADERS="+string(headersJSON),
	)

	out, err := cmd.Output()
	if err != nil {
		// cmd.Output() returns *exec.ExitError on non-zero exit.
		// The script may have already printed a JSON error to stdout before
		// calling sys.exit(1). Try to parse it first.
		if len(out) > 0 {
			var result pythonResult
			if jsonErr := json.Unmarshal(out, &result); jsonErr == nil && result.Error != "" {
				return nil, fmt.Errorf("anthropic api: curl_cffi: %s", result.Error)
			}
		}
		return nil, fmt.Errorf("anthropic api: python subprocess: %w", err)
	}

	var result pythonResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("anthropic api: parse python output: %w", err)
	}

	if result.Status == 0 && result.Error != "" {
		return nil, fmt.Errorf("anthropic api: curl_cffi: %s", result.Error)
	}

	return &http.Response{
		StatusCode: result.Status,
		Status:     fmt.Sprintf("%d", result.Status),
		Body:       io.NopCloser(strings.NewReader(result.Body)),
		Header:     make(http.Header),
	}, nil
}

// pythonResult is the JSON envelope printed by the Python script.
type pythonResult struct {
	Status  int               `json:"status"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers"`
	Error   string            `json:"error"`
}
