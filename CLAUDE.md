# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`konfuse` is a Go CLI for kubeconfig management: merging files (with rename-on-import and auto-backup), listing entries, switching the active context, and deleting contexts cleanly. Single binary, no runtime dependencies. Requires Go 1.22+ to build.

## Development Setup

```bash
go mod tidy
```

## Commands

```bash
# Build
go build -o konfuse .

# Run tests
go test ./...

# Run a specific test
go test ./internal/merger/ -run TestRenameUser

# Run tests with verbose output
go test -v ./...

# Vet
go vet ./...

# Run the tool
go run . merge new-cluster.yaml --rename-context prod --rename-cluster eks-prod
go run . list --json
go run . use prod
go run . delete old-staging --yes
```

## Architecture

```
main.go                   # CLI entry point (flag parsing, I/O, output formatting)
internal/merger/
  merger.go               # Pure merge logic — no I/O, all testable
  merger_test.go          # Go tests
```

**`main.go`** handles all I/O: subcommand dispatch, flag parsing, loading/saving YAML, creating backups, formatting human/JSON output, confirmation prompts, exit codes. Each subcommand is split into a thin `runX(...)` wrapper that calls `runXE(args, kubeconfig, stdin, stdout, stderr) int` so CLI behavior is unit-testable end-to-end.

**`internal/merger`** is pure logic (no I/O):
- `MergeKubeconfig(existing, incoming, renameContext, renameCluster, renameUser)` — merges two configs, renames first entries only, updates cross-references, returns `(*KubeConfig, MergeResult)`
- `ListEntries(cfg)` / `DeleteContext(cfg, name)` / `UseContext(cfg, name)` — readers and modifiers used by the corresponding subcommands
- `BackupConfig(path)` — creates a timestamped `.backup.<timestamp>` copy
- `KubeConfig` / `NamedEntry` — YAML-tagged structs; `NamedEntry.Body` uses `yaml:",inline"` to preserve unknown fields

## Key behaviours

- Only the **first** cluster/context/user in the incoming file is renamed; others pass through unchanged
- When `--rename-cluster` is set, the cluster reference inside the first context is also updated
- When `--rename-user` is set, the user reference inside the first context is also updated
- `konfuse use <context>` switches `current-context`; backup is only written when the value actually changes (no-op switches leave the file untouched)
- `merge` (over an existing file) and `delete` prompt for confirmation in interactive shells. `--yes`, `--json`, and non-TTY stdin all auto-skip the prompt. `use` never prompts and does not accept `--yes`.
- `merge` is an explicit subcommand (`konfuse merge <file>`); there is no bare-file shortcut. An unrecognized first argument is a usage error (exit 2); when it looks like a path, the hint points at `konfuse merge <file>`.
- `--version` and `-h`/`--help` are top-level concerns handled in `main()` before subcommand dispatch, so they work regardless of where they appear in argv.
- `--json` is auto-enabled when stdout is not a TTY (pipes, CI)
- Exit codes: 0 ok, 1 error (load/parse/write), 2 usage error or user-aborted prompt, 3 file not found

## CI

- `.github/workflows/ci.yml` — `go vet` + `go test ./...` on every push/PR
- `.github/workflows/release.yml` — cross-compiled binaries for linux/macos × amd64/arm64 via `GOOS`/`GOARCH`, uploaded to GitHub Releases with SHA256 checksums, then publishes to PyPI via OIDC
