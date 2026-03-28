# AGENTS.md

This file provides guidance to AI coding agents (OpenAI Codex, etc.) when working with code in this repository.

## Overview

`konfuse` is a single-file Python CLI tool (`konfuse.py`) that merges Kubernetes kubeconfig files with rename-on-import and auto-backup.

## Development Setup

```bash
pip install -e ".[dev]"
```

## Commands

```bash
# Run tests
pytest

# Run a single test class
pytest tests/test_merge.py::TestRenameUser

# Run with coverage
pytest --cov=konfuse --cov-report=term-missing

# Lint
ruff check .

# Run the tool
konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod
```

## Architecture

All logic is in `konfuse.py`. Execution flow:

1. **CLI** (`main()` via argparse) — validates input, parses flags
2. **Load** (`load_yaml`) — parses incoming and existing kubeconfig YAML
3. **Backup** (`backup_config`) — creates a timestamped copy before any writes
4. **Merge** (`merge_kubeconfig`) — pure function (dict-in, dict-out): merges clusters/users/contexts, applies optional renames to the first entry of each section, updates cross-references, returns `(merged_dict, result_dict)`
5. **Save** (`save_yaml`) — writes the result; skipped in `--dry-run` mode

## Key flags

| Flag | Behaviour |
|---|---|
| `--dry-run` | Compute and show changes without writing |
| `--json` | Output structured JSON (auto-enabled when stdout is not a TTY) |
| `--yes` | Skip prompts (non-interactive / CI mode) |
| `--rename-context` | Rename the first incoming context |
| `--rename-cluster` | Rename the first incoming cluster |
| `--rename-user` | Rename the first incoming user |

## CI

- `.github/workflows/ci.yml` — lint + test matrix on every push/PR
- `.github/workflows/release.yml` — standalone binaries + PyPI publish on `v*` tags
