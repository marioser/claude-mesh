# Changelog

All notable changes to Claude Mesh are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
with the pre-1.0 caveat that minor version bumps may include breaking changes
until the project leaves beta.

## [Unreleased]

### Added

- `LICENSE` (MIT), `CONTRIBUTING.md`, and a public-facing `README.md`.
- `.gitignore` covering `dist/` build artifacts.
- `Store.TouchOrCreateSession` — activity-friendly upsert that registers a
  session in the active ZSET on its first activity event, seeding identifying
  metadata (cwd, opened_at) only when the session Hash is absent (HSetNX
  semantics). Lets resumed sessions and sessions whose Hash expired during
  the close-grace window reappear automatically.
- Hooks now source `${XDG_CONFIG_HOME:-$HOME/.config}/claude-mesh/hook-env`
  if it exists, so users running against a non-localhost MQTT broker / Redis
  no longer need to fight Claude Code's stripped hook environment. An
  example file is shipped at `examples/hook-env`.

### Fixed

- Session lifecycle: `mesh_active_sessions` now reflects long-running and
  resumed sessions correctly. Previously the `stop.sh` hook published a
  `session-close` at the end of every agent turn (Claude Code fires the `Stop`
  event after every turn, not just at session termination), causing the bridge
  to evict active sessions immediately. Two changes ship together:
  - `hooks/stop.sh` is now a no-op; sessions expire naturally via the
    bridge `SessionTTL` sweep when activity stops.
  - The bridge `activity` handler now uses `TouchOrCreateSession` instead of
    `TouchSession`, so a session that was closed (or never opened via
    `session-start`, e.g. `claude --resume`) is re-registered on its next
    activity event.
- Hook env loading: Claude Code launches hooks with a stripped environment,
  so `CLAUDE_MESH_*` exports from `~/.zshenv` / `~/.bashrc` never reached
  the bridge CLI invoked by the hook. The CLI silently fell back to
  `localhost:1883` / `localhost:6379` and published nothing, leaving
  `mesh_active_sessions` empty for any non-localhost deployment.
  Hooks now load config from `${XDG_CONFIG_HOME:-$HOME/.config}/claude-mesh/hook-env`.

### Changed

- Module path: `claude-mesh` → `github.com/marioser/claude-mesh`.
- launchd label: `com.miobox.claude-mesh-bridge` → `io.github.marioser.claude-mesh-bridge`.
- Repository extracted from MIOBOX monorepo into its own OSS project. Git
  history preserved via `git subtree split`.

### Removed

- Tracked binaries under `dist/` (now gitignored, built on demand).

## [Pre-extraction]

The commit history before this CHANGELOG was maintained inside the MIOBOX
monorepo under `scripts/claude-mesh/`. Highlights:

- **Bridge resilience** — MQTT subscriber health surfaced through `status`
  command, `OnConnectHandler` wired for automatic re-subscribe on reconnect,
  active sessions rehydrated on daemon restart.
- **Anthropic usage poll** — optional, env-gated tracking of plan usage via
  Anthropic's internal API, with JSONL parser fallback.
- **MCP server** — five tools exposed to Claude Code: `mesh_status`,
  `mesh_active_sessions`, `mesh_recent_activity`, `mesh_check_conflict`,
  `mesh_announce`.
- **Status line** — compact one-line summary for Claude Code's `statusLine`.

[Unreleased]: https://github.com/marioser/claude-mesh/compare/v0.1.0...HEAD
