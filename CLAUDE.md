# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`konfuse` is a single-file Python CLI tool that merges Kubernetes kubeconfig files. Unique combination: merge + rename-on-import + timestamped auto-backup.

## Development Setup

```bash
pip install -e ".[dev]"
```

## Commands

```bash
# Run tests
pytest

# Run a single test
pytest tests/test_merge.py::TestRenameCluster

# Run with coverage
pytest --cov=konfuse --cov-report=term-missing

# Lint
ruff check .

# Run the tool
konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod
```

## Architecture

All logic lives in `konfuse.py` (~200 lines). Execution flow:

1. **CLI** (`main()` via argparse) — validates input, reads arguments
2. **Load** (`load_yaml`) — parses both the incoming and existing kubeconfig YAML
3. **Backup** (`backup_config`) — creates a timestamped `.bak` copy before any writes
4. **Merge** (`merge_kubeconfig`) — merges clusters, users, and contexts; warns on name conflicts; applies optional renames only to the **first** cluster/context in the incoming file, updating internal cross-references
5. **Save** (`save_yaml`) — writes the merged result

`merge_kubeconfig` is pure (dict-in, dict-out, no I/O) — all tests call it directly without mocking.

## CI

- `.github/workflows/ci.yml` — runs lint + tests on Python 3.8/3.10/3.12 × ubuntu/macos on every push and PR
- `.github/workflows/release.yml` — on `v*` tags: builds standalone binaries via PyInstaller (linux-amd64, linux-arm64, macos-amd64, macos-arm64), uploads them as GitHub Release assets with SHA256 checksums, then publishes to PyPI
