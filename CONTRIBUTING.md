# Contributing

Thanks for considering a contribution to Claude Mesh. This document covers the
workflow, conventions, and what we look for in pull requests.

## Quick start

```bash
git clone https://github.com/marioser/claude-mesh.git
cd claude-mesh
go test ./...
```

If tests pass, you have a working dev environment.

## Workflow

1. **Open an issue first** for anything beyond a typo or a one-line fix. We
   want to align on scope before you spend time on code.
2. **Fork and branch.** Branch names: `fix/<short-desc>`, `feat/<short-desc>`,
   `docs/<short-desc>`.
3. **Write a focused PR.** Keep changes under ~400 lines when possible. If
   the change is bigger, split it into reviewable chunks.
4. **Tests are mandatory.** Every behavior change needs at least one test.
   Use `miniredis` for Redis interactions, fake clients for MQTT.
5. **Lint clean.** Run `go vet ./...` and (when available)
   `golangci-lint run`.
6. **Conventional commits.** Format: `type(scope): summary`. Types: `feat`,
   `fix`, `docs`, `chore`, `refactor`, `test`. Do NOT add `Co-Authored-By` or
   AI attribution lines.

## What we look for

- **Tests that exercise behavior, not implementation.** Asserting on output
  shape beats asserting on internal struct field counts.
- **No silent failures.** Errors should be returned, logged, or both —
  never swallowed with `_ = err`.
- **No new global state.** If you need shared state, pass it as a dependency.
- **Doc updates with code.** A new env var means a row in the README table.

## Architecture notes

The codebase has three components:

- `cmd/claude-mesh-bridge` and root `main.go` — the bridge daemon
- `cmd/claude-mesh-mcp` — the MCP server
- `internal/` — everything else, split by domain (mqtt, store, bridge, etc.)

The bridge talks to MQTT (paho) and Redis (go-redis). MCP handlers read from
the store, never directly from MQTT. Hooks publish to MQTT, never to Redis
directly.

## Reporting bugs

Open an issue with:

- What you expected vs. what happened
- Output of `claude-mesh-bridge status --json`
- Relevant lines from `~/Library/Logs/claude-mesh-bridge.log` (or your
  `CLAUDE_MESH_LOG_DIR`)
- OS and Go version

## License

By contributing, you agree your contributions are licensed under the
[MIT License](LICENSE).
