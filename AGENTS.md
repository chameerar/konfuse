# AGENTS.md

This file provides guidance to AI coding agents (OpenAI Codex, etc.) when working with code in this repository.

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

**`internal/merger.MergeKubeconfig`** is a pure function — all tests call it directly without any mocking.

## Key flags

| Flag | Behaviour |
|---|---|
| `--dry-run` | Compute and show changes without writing |
| `--json` | Structured JSON output (auto-enabled when stdout is not a TTY) |
| `--yes` | Skip prompts (non-interactive / CI mode) |
| `--rename-context` | Rename the first incoming context |
| `--rename-cluster` | Rename the first incoming cluster (also updates context's cluster ref) |
| `--rename-user` | Rename the first incoming user (also updates context's user ref) |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error |
| 2 | Usage / argument error |
| 3 | Input file not found |
