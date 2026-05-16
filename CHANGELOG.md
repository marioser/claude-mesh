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
