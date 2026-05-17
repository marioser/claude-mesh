#!/usr/bin/env bash
# Claude Mesh — Stop hook (no-op since v0.x.y)
#
# Claude Code fires the `Stop` event at the end of EVERY agent turn,
# not just at session termination. Publishing session-close on Stop
# caused the bridge to evict active sessions after each turn, leaving
# mesh_active_sessions permanently empty.
#
# Sessions now expire naturally via the bridge's SessionTTL sweep when
# no activity events arrive for the configured window. Activity events
# (PreToolUse) refresh the session via TouchOrCreateSession in the bridge.
#
# This hook is kept (instead of being removed) so that existing installs
# whose ~/.claude/settings.json still references stop.sh continue to work
# silently — no orphan hook errors, no manual cleanup required.
#
# If you need a hard close (e.g. process exit), publish manually:
#   echo '{"ts":...,"session_id":"...","reason":"exit"}' \
#     | claude-mesh-bridge publish session-close

set -euo pipefail

LOG_FILE="${HOME}/Library/Logs/claude-mesh-hooks.log"

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] stop: $*" >> "${LOG_FILE}" 2>&1
}

# Loop guard parity with previous version: if Claude re-fires the Stop
# hook within the same turn (stop_hook_active=true), exit immediately.
INPUT=$(cat || true)
if command -v jq > /dev/null 2>&1; then
    STOP_ACTIVE=$(echo "${INPUT}" | jq -r '.stop_hook_active // false' 2>/dev/null || echo "false")
elif echo "${INPUT}" | grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true' 2>/dev/null; then
    STOP_ACTIVE="true"
else
    STOP_ACTIVE="false"
fi
if [ "${STOP_ACTIVE}" = "true" ]; then
    exit 0
fi

SESSION_ID=$(echo "${INPUT}" | jq -r '.session_id // ""' 2>/dev/null || true)
log "stop received (no-op, session kept alive by TTL): session_id=${SESSION_ID}"

exit 0
