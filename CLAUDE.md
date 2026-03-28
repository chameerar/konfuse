# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`konfuse` is a Go CLI tool that merges Kubernetes kubeconfig files with rename-on-import and auto-backup. Single binary, no runtime dependencies.

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
go run . new-cluster.yaml --rename-context prod --rename-cluster eks-prod
```

## Architecture

```
main.go                   # CLI entry point (flag parsing, I/O, output formatting)
internal/merger/
  merger.go               # Pure merge logic — no I/O, all testable
  merger_test.go          # Go tests
```

**`main.go`** handles all I/O: loading/saving YAML, creating backups, formatting human/JSON output, exit codes.

**`internal/merger`** is pure logic (no I/O):
- `MergeKubeconfig(existing, incoming, renameContext, renameCluster, renameUser)` — merges two configs, renames first entries only, updates cross-references, returns `(*KubeConfig, MergeResult)`
- `BackupConfig(path)` — creates a timestamped `.backup.<timestamp>` copy
- `KubeConfig` / `NamedEntry` — YAML-tagged structs; `NamedEntry.Body` uses `yaml:",inline"` to preserve unknown fields

## Key behaviours

- Only the **first** cluster/context/user in the incoming file is renamed; others pass through unchanged
- When `--rename-cluster` is set, the cluster reference inside the first context is also updated
- When `--rename-user` is set, the user reference inside the first context is also updated
- `--json` is auto-enabled when stdout is not a TTY (pipes, CI)
- Exit codes: 0 ok, 1 error, 2 usage error, 3 file not found

## CI

- `.github/workflows/ci.yml` — `go vet` + `go test ./...` on every push/PR
- `.github/workflows/release.yml` — cross-compiled binaries for linux/macos × amd64/arm64 via `GOOS`/`GOARCH`, uploaded to GitHub Releases with SHA256 checksums, then publishes to PyPI via OIDC
