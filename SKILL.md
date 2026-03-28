---
name: konfuse
description: Merge a kubeconfig file into the local kubeconfig with optional rename on import and automatic backup
triggers:
  - merge kubeconfig
  - add cluster to kubeconfig
  - import kubeconfig
  - rename context kubeconfig
  - combine kubeconfig files
---

# konfuse

Merge any kubeconfig file into your existing `~/.kube/config` in one command.

## Usage

```bash
# Preview what will change (always do this first)
konfuse <file> --dry-run

# Basic merge
konfuse <file>

# Rename context and cluster on import (recommended)
konfuse <file> --rename-context <name> --rename-cluster <name>

# Rename all three (context, cluster, user)
konfuse <file> --rename-context <name> --rename-cluster <name> --rename-user <name>

# Non-interactive / CI mode with JSON output
konfuse <file> --yes --json

# Target a different kubeconfig
konfuse <file> --kubeconfig /path/to/config
```

## Rules

- Always use `--dry-run` before merging in automated or unfamiliar contexts
- Use `--rename-context` and `--rename-cluster` when the incoming file uses generic names like `kubernetes-admin@cluster.local`
- Use `--json` to get machine-readable output; use `--yes` to suppress all prompts
- Only the **first** context/cluster/user in the incoming file is renamed; others pass through unchanged
- A timestamped backup is automatically created before any write — restore with `cp ~/.kube/config.backup.<timestamp> ~/.kube/config`

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error |
| 2 | Usage / argument error |
| 3 | Input file not found |

## JSON output schema

```json
{
  "dry_run": false,
  "target": "/Users/you/.kube/config",
  "backup": "/Users/you/.kube/config.backup.20260328T120000",
  "changes": {
    "clusters": { "added": ["eks-prod"], "replaced": [] },
    "users":    { "added": ["eks-prod-user"], "replaced": [] },
    "contexts": { "added": ["prod"], "replaced": [] }
  },
  "has_conflicts": false
}
```
