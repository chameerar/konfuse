# Changelog

All notable changes to konfuse are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added
- `konfuse use <context-name>` — switch the active context (sets `current-context`); supports `--kubeconfig` and `--json`
- Backup is written only when `use` actually changes the current context (no-op switches leave the file untouched)
- Confirmation prompt before destructive writes on `merge` (when target exists) and `delete`. Auto-skipped in non-TTY contexts (pipes / CI), with `--json`, or with `--yes`. The `--yes` flag is now wired (previously declared but ignored).
- `--yes` is accepted on `use` for scripting symmetry, though `use` never prompts.

### Changed
- `delete` and `use` JSON output now include a `target` field and emit `backup: null` when no backup was created (previously the `backup` key was omitted via `omitempty`).

### Fixed
- `konfuse delete <context-name> --kubeconfig PATH` now respects flags placed after the positional argument (previously the flag was silently ignored)

---

## [0.2.0] - 2026-04-01

### Added
- `konfuse list` — list all contexts, clusters, and users in the kubeconfig; marks the current context with `*`
- `konfuse delete <context-name>` — delete a context and automatically remove its orphaned cluster/user entries
- Both subcommands support `--kubeconfig` and `--json` flags
- Automatic backup before delete operations

---

## [0.1.1] - 2026-03-28

### Added
- `--version` flag — prints the version and exits (`konfuse v0.1.1`)
- Version embedded at build time via `-ldflags "-X main.version=..."` for release binaries

### Fixed
- Empty input file now returns a clear `"Input file is empty"` error with exit code 3 (was a cryptic `"Failed to parse YAML: EOF"` with exit code 1)

### Documented
- `--json` and `--yes` flag help text now explicitly states that non-TTY contexts (pipes, CI) automatically skip prompts and enable JSON output

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
