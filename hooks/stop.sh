#!/usr/bin/env bash
# Claude Mesh — Stop hook
# Publishes a session-close event. MUST check stop_hook_active to prevent infinite loop.
# Always exits 0 — never blocks Claude Code.
# Produces NO stdout output.

set -euo pipefail

LOG_FILE="${HOME}/Library/Logs/claude-mesh-hooks.log"
BRIDGE_BIN="claude-mesh-bridge"

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] stop: $*" >> "${LOG_FILE}" 2>&1
}

INPUT=$(cat)

# LOOP GUARD: if stop_hook_active is true, exit immediately.
# Use jq if available, else grep fallback (no jq dependency required).
if command -v jq > /dev/null 2>&1; then
    STOP_ACTIVE=$(echo "${INPUT}" | jq -r '.stop_hook_active // false' 2>/dev/null || echo "false")
else
    if echo "${INPUT}" | grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
        STOP_ACTIVE="true"
    else
        STOP_ACTIVE="false"
    fi
fi

if [ "${STOP_ACTIVE}" = "true" ]; then
    exit 0
fi

# Guard: binary must exist.
if ! command -v "${BRIDGE_BIN}" > /dev/null 2>&1; then
    exit 0
fi

SESSION_ID=$(echo "${INPUT}" | jq -r '.session_id // ""' 2>/dev/null || true)
TS=$(python3 -c "import time; print(f'{time.time()*1000:.3f}')" 2>/dev/null || date +%s000)

if [ -z "${SESSION_ID}" ]; then
    exit 0
fi

PAYLOAD=$(jq -n \
    --arg ts "${TS}" \
    --arg session_id "${SESSION_ID}" \
    --arg reason "stop" \
    '{ts: ($ts | tonumber), session_id: $session_id, reason: $reason}' \
    2>/dev/null)

if [ -z "${PAYLOAD}" ]; then
    exit 0
fi

# Publish with timeout — prefer gtimeout (brew coreutils), then timeout, then no-timeout fallback.
if command -v gtimeout > /dev/null 2>&1; then
    echo "${PAYLOAD}" | gtimeout 2s "${BRIDGE_BIN}" publish session-close >> "${LOG_FILE}" 2>&1 || log "publish timeout or failed"
elif command -v timeout > /dev/null 2>&1; then
    echo "${PAYLOAD}" | timeout 2s "${BRIDGE_BIN}" publish session-close >> "${LOG_FILE}" 2>&1 || log "publish timeout or failed"
else
    echo "${PAYLOAD}" | perl -e 'alarm(2); exec @ARGV' "${BRIDGE_BIN}" publish session-close >> "${LOG_FILE}" 2>&1 || log "publish failed"
fi

# Smoke log: proof of execution for debugging session lifecycle.
log "session closed: session_id=${SESSION_ID}"

exit 0
