#!/usr/bin/env bash
# Claude Mesh — UserPromptSubmit hook
# Publishes an activity event on every user prompt so idle sessions
# (those that are not invoking Edit/Write/MultiEdit/Bash within the
# sweep cutoff) do not get evicted from the active-sessions ZSET.
# Must NOT block or alter the user's prompt — always exits 0 with no stdout.

set -euo pipefail

LOG_FILE="${HOME}/Library/Logs/claude-mesh-hooks.log"
BRIDGE_BIN="claude-mesh-bridge"
TIMEOUT_SECS=1.5
TIMEOUT_INT=2

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] user-prompt-submit: $*" >> "${LOG_FILE}" 2>&1
}

# Guard: binary must exist.
if ! command -v "${BRIDGE_BIN}" > /dev/null 2>&1; then
    exit 0
fi

INPUT=$(cat)

SESSION_ID=$(echo "${INPUT}" | jq -r '.session_id // ""' 2>/dev/null || true)
CWD=$(echo "${INPUT}" | jq -r '.cwd // ""' 2>/dev/null || true)

if [ -z "${SESSION_ID}" ]; then
    log "missing session_id in stdin — skipping"
    exit 0
fi

TS=$(python3 -c "import time; print(f'{time.time()*1000:.3f}')" 2>/dev/null || date +%s000)

PAYLOAD=$(jq -n \
    --arg ts "${TS}" \
    --arg session_id "${SESSION_ID}" \
    --arg cwd "${CWD}" \
    '{ts: ($ts | tonumber), session_id: $session_id, tool: "UserPromptSubmit", target: "", cwd: $cwd}' \
    2>/dev/null)

if [ -z "${PAYLOAD}" ]; then
    exit 0
fi

if command -v gtimeout > /dev/null 2>&1; then
    echo "${PAYLOAD}" | gtimeout "${TIMEOUT_SECS}s" "${BRIDGE_BIN}" publish activity >> "${LOG_FILE}" 2>&1 || log "publish timeout or failed"
elif command -v timeout > /dev/null 2>&1; then
    echo "${PAYLOAD}" | timeout "${TIMEOUT_SECS}s" "${BRIDGE_BIN}" publish activity >> "${LOG_FILE}" 2>&1 || log "publish timeout or failed"
else
    echo "${PAYLOAD}" | perl -e 'alarm('"${TIMEOUT_INT}"'); exec @ARGV' "${BRIDGE_BIN}" publish activity >> "${LOG_FILE}" 2>&1 || log "publish failed"
fi

exit 0
