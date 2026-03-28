# Changelog

All notable changes to konfuse are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

---

## [0.1.0] - 2026-03-28

### Added
- Merge any kubeconfig YAML into `~/.kube/config` (or a custom target) in one command
- `--rename-context` — rename the first incoming context on import
- `--rename-cluster` — rename the first incoming cluster on import
- `--rename-user` — rename the first incoming user on import
- `--dry-run` — preview all changes without writing anything
- `--json` — structured JSON output; auto-enabled when stdout is not a TTY (pipes, CI)
- `--yes` — non-interactive / CI mode
- `--kubeconfig` — target a kubeconfig other than `~/.kube/config`
- Automatic timestamped backup before every write (`~/.kube/config.backup.<YYYYMMDDTHHMMSS>`)
- Conflict detection with warnings — incoming entries replace existing ones of the same name
- Internal reference updates: renaming a cluster also updates the cluster reference inside any affected context
- Standalone binaries for Linux (amd64, arm64) and macOS (arm64) — no Python required
- `SKILL.md` for auto-discovery by Claude Code, Cursor, and Gemini CLI
- `AGENTS.md` for auto-discovery by OpenAI Codex
