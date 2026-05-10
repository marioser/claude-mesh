#!/usr/bin/env bash
# Claude Mesh — PreToolUse hook
# Reads stdin JSON and publishes an activity event for Edit, Write, MultiEdit, Bash tools.
# Must NOT block or deny tool execution — always exits 0.
# Produces NO stdout output.

set -euo pipefail

LOG_FILE="${HOME}/Library/Logs/claude-mesh-hooks.log"
BRIDGE_BIN="claude-mesh-bridge"
TIMEOUT_SECS=1

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] pre-tool-use: $*" >> "${LOG_FILE}" 2>&1
}

# Guard: binary must exist.
if ! command -v "${BRIDGE_BIN}" > /dev/null 2>&1; then
    exit 0
fi

INPUT=$(cat)

TOOL_NAME=$(echo "${INPUT}" | jq -r '.tool_name // ""' 2>/dev/null || true)
SESSION_ID=$(echo "${INPUT}" | jq -r '.session_id // ""' 2>/dev/null || true)
CWD=$(echo "${INPUT}" | jq -r '.cwd // ""' 2>/dev/null || true)

# Only publish for file-touching tools.
case "${TOOL_NAME}" in
    Edit|Write|MultiEdit|Bash) ;;
    *) exit 0 ;;
esac

# Extract target: file_path for Edit/Write/MultiEdit, truncated command for Bash.
if [ "${TOOL_NAME}" = "Bash" ]; then
    TARGET=$(echo "${INPUT}" | jq -r '.tool_input.command // ""' 2>/dev/null | head -c 200 || true)
else
    TARGET=$(echo "${INPUT}" | jq -r '.tool_input.file_path // ""' 2>/dev/null || true)
fi

TS=$(python3 -c "import time; print(f'{time.time()*1000:.3f}')" 2>/dev/null || date +%s000)

PAYLOAD=$(jq -n \
    --arg ts "${TS}" \
    --arg session_id "${SESSION_ID}" \
    --arg tool "${TOOL_NAME}" \
    --arg target "${TARGET}" \
    --arg cwd "${CWD}" \
    '{ts: ($ts | tonumber), session_id: $session_id, tool: $tool, target: $target, cwd: $cwd}' \
    2>/dev/null)

if [ -z "${PAYLOAD}" ]; then
    exit 0
fi

# Publish with hard timeout — kill subprocess on timeout.
if command -v timeout > /dev/null 2>&1; then
    echo "${PAYLOAD}" | timeout "${TIMEOUT_SECS}s" "${BRIDGE_BIN}" publish activity >> "${LOG_FILE}" 2>&1 || log "publish timeout or failed"
else
    echo "${PAYLOAD}" | perl -e 'alarm('"${TIMEOUT_SECS}"'); exec @ARGV' "${BRIDGE_BIN}" publish activity >> "${LOG_FILE}" 2>&1 || log "publish failed"
fi

exit 0
