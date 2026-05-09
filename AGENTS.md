# AGENTS.md

This file provides guidance to AI coding agents (OpenAI Codex, etc.) when working with code in this repository.

## Overview

`konfuse` is a Go CLI tool for kubeconfig management: merging files (with rename-on-import and auto-backup), listing entries, switching the active context, and deleting contexts cleanly. Single binary, no runtime dependencies. Requires Go 1.22+ to build.

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

# Vet
go vet ./...

# Run the tool
go run . new-cluster.yaml --rename-context prod --rename-cluster eks-prod
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

**`internal/merger`** is pure — all tests call its functions directly without mocking. CLI integration tests live in `main_test.go` and exercise the `runXE` entry points with in-memory readers/writers.

## Subcommands

| Command | Purpose | Notes |
|---|---|---|
| `konfuse <file>` (default) | Merge `<file>` into the target kubeconfig | Prompts before overwriting; skip with `--yes` / `--json` / non-TTY |
| `konfuse list` | List contexts, clusters, users | Read-only |
| `konfuse use <ctx>` | Switch the active context | No-op (no backup, no write) when already on `<ctx>` |
| `konfuse delete <ctx>` | Delete a context and any orphaned cluster/user | Prompts before writing; skip with `--yes` / `--json` / non-TTY |

## Key flags

| Flag | Behaviour |
|---|---|
| `--dry-run` | (merge) Compute and show changes without writing |
| `--json` | Structured JSON output (auto-enabled when stdout is not a TTY); also auto-skips confirmation prompts |
| `--yes` | Skip confirmation prompts. Honored on merge and delete; accepted (no-op) on use for scripting symmetry |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--rename-context` | (merge) Rename the first incoming context |
| `--rename-cluster` | (merge) Rename the first incoming cluster (also updates context's cluster ref) |
| `--rename-user` | (merge) Rename the first incoming user (also updates context's user ref) |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error (load / parse / write failure) |
| 2 | Usage error or user-aborted prompt |
| 3 | Input file or kubeconfig not found |
