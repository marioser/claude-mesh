#!/usr/bin/env bash
# Claude Mesh — SessionStart hook
# Reads stdin JSON from Claude Code, extracts session info, and publishes a session-open event.
# Produces NO stdout output (context injection must come from other hooks like NeuroStack).
# All output is redirected to the log file.

set -euo pipefail

# Claude Code launches hooks with a stripped environment — shell exports in
# ~/.zshenv / ~/.bashrc / ~/.profile do NOT reach this subprocess. Load the
# bridge connection config (CLAUDE_MESH_MQTT_*, CLAUDE_MESH_REDIS_*) from a
# user-controlled file so the bridge CLI can reach a non-localhost broker.
# Convention: ${XDG_CONFIG_HOME:-$HOME/.config}/claude-mesh/hook-env
# See examples/hook-env in the repo for the expected format.
# shellcheck disable=SC1090
_cm_env_file="${XDG_CONFIG_HOME:-$HOME/.config}/claude-mesh/hook-env"
if [ -f "${_cm_env_file}" ]; then
    set -a
    # shellcheck disable=SC1090
    source "${_cm_env_file}" 2>/dev/null || true
    set +a
fi

LOG_FILE="${HOME}/Library/Logs/claude-mesh-hooks.log"
BRIDGE_BIN="claude-mesh-bridge"

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] session-start: $*" >> "${LOG_FILE}" 2>&1
}

# Guard: if binary is not found, log and exit 0 (never block Claude Code).
if ! command -v "${BRIDGE_BIN}" > /dev/null 2>&1; then
    log "binary not found: ${BRIDGE_BIN} — skipping"
    exit 0
fi

# Ensure daemon is running (background, no-wait so we don't block startup).
"${BRIDGE_BIN}" --ensure-running >> "${LOG_FILE}" 2>&1 &

# Read stdin JSON from Claude Code.
INPUT=$(cat)

# Extract required fields using jq (macOS dev boxes have jq via brew).
SESSION_ID=$(echo "${INPUT}" | jq -r '.session_id // ""' 2>/dev/null || true)
CWD=$(echo "${INPUT}" | jq -r '.cwd // ""' 2>/dev/null || true)
TRANSCRIPT=$(echo "${INPUT}" | jq -r '.transcript_path // ""' 2>/dev/null || true)
GIT_BRANCH=$(git -C "${CWD}" branch --show-current 2>/dev/null || echo "")
HOST=$(hostname -s 2>/dev/null || echo "unknown")
PID=$$
TS=$(python3 -c "import time; print(f'{time.time()*1000:.3f}')" 2>/dev/null || date +%s000)

if [ -z "${SESSION_ID}" ]; then
    log "missing session_id in stdin — skipping"
    exit 0
fi

# Build JSON payload and publish with a 3s timeout.
PAYLOAD=$(jq -n \
    --arg ts "${TS}" \
    --arg session_id "${SESSION_ID}" \
    --arg cwd "${CWD}" \
    --arg transcript_path "${TRANSCRIPT}" \
    --arg git_branch "${GIT_BRANCH}" \
    --arg host "${HOST}" \
    --argjson pid "${PID}" \
    '{ts: ($ts | tonumber), session_id: $session_id, cwd: $cwd, transcript_path: $transcript_path, git_branch: $git_branch, host: $host, pid: $pid}' \
    2>/dev/null)

if [ -z "${PAYLOAD}" ]; then
    log "failed to build payload — skipping"
    exit 0
fi

# Publish with timeout — exit 0 regardless of result (never block Claude Code).
# Prefer gtimeout (brew coreutils on macOS), then timeout (Linux/some macOS), then no-timeout fallback.
# Note: the publish command is fast (<1s on local broker) so the no-timeout fallback is safe.
_publish_with_timeout() {
    local payload="$1"
    if command -v gtimeout > /dev/null 2>&1; then
        echo "${payload}" | gtimeout 3s "${BRIDGE_BIN}" publish session-open >> "${LOG_FILE}" 2>&1
    elif command -v timeout > /dev/null 2>&1; then
        echo "${payload}" | timeout 3s "${BRIDGE_BIN}" publish session-open >> "${LOG_FILE}" 2>&1
    else
        # No timeout available — run directly (publish is fast on local broker).
        echo "${payload}" | "${BRIDGE_BIN}" publish session-open >> "${LOG_FILE}" 2>&1
    fi
}

_publish_with_timeout "${PAYLOAD}" || log "publish failed or timed out"

# Smoke log: proof of execution for debugging session lifecycle.
log "session opened: session_id=${SESSION_ID} cwd=${CWD} git_branch=${GIT_BRANCH}"

exit 0
