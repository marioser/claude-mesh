# Claude Mesh — MIOBOX Observability Bridge (SMBX-177)

Claude Mesh enables real-time cross-session awareness for Claude Code. It publishes session lifecycle and tool-use events over MQTT, persists them in Redis, and exposes 4 MCP tools so any running Claude session can query the shared mesh state.

---

## Install

### Prerequisites

- mosquitto (MQTT broker): `brew install mosquitto`
- redis: `brew install redis`
- Both running on localhost (default ports 1883 / 6379)

### Build and install

```bash
cd scripts/claude-mesh
make build
./dist/claude-mesh-bridge install
```

The installer:
1. Writes the launchd plist and loads the daemon (`launchctl load`)
2. Patches `~/.claude/settings.json` with 3 hook entries (idempotent)
3. Patches `~/.claude/.mcp.json` with the `claude-mesh` MCP server entry (idempotent)

---

## Uninstall

```bash
./dist/claude-mesh-bridge uninstall
```

---

## Dev Loop

```bash
make test          # unit tests (miniredis-backed, no external deps)
make test-race     # unit tests with race detector
make test-integration  # requires mosquitto + redis running
make test-e2e      # shell E2E script (requires build)
make build         # compile for host
make crossbuild    # compile for darwin-arm64, darwin-amd64, linux-amd64
```

---

## Topics

| Topic | Publisher | QoS | Description |
|-------|-----------|-----|-------------|
| `claude/mesh/session/{sid}/open` | session-start hook | 1 | Session opened |
| `claude/mesh/session/{sid}/activity` | pre-tool-use hook | 1 | Tool use event |
| `claude/mesh/session/{sid}/close` | stop hook | 1 | Session closed |

---

## Redis Keys

| Key | Type | TTL | Description |
|-----|------|-----|-------------|
| `claude:mesh:session:{sid}` | Hash | 90s | Session metadata |
| `claude:mesh:sessions:active` | ZSET | none | Active session IDs (score = last_seen_ms) |
| `claude:mesh:activity:{sid}` | List | 600s | Per-session activity ring (newest first, max 50) |
| `claude:mesh:activity:global` | List | 1800s | Global activity ring (newest first, max 200) |

---

## MCP Tools

| Tool | Description |
|------|-------------|
| `mesh_status` | Active session count and summaries |
| `mesh_check_conflict` | Sessions that recently touched a given file path |
| `mesh_recent_activity` | Cross-session activity from global ring buffer |
| `mesh_announce` | Publish a manual intent event |

---

## Troubleshooting

**Daemon not starting**: Check `~/Library/Logs/claude-mesh-bridge.log`. Verify mosquitto and redis are running.

**Hooks not firing**: Verify entries in `~/.claude/settings.json`. Run `claude-mesh-bridge status` to check daemon/Redis/MQTT health.

**Log paths**:
- Daemon: `~/Library/Logs/claude-mesh-bridge.log`
- Hooks: `~/Library/Logs/claude-mesh-hooks.log`
