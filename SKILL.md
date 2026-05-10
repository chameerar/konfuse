---
name: konfuse
description: Merge, list, switch, and delete entries in the local kubeconfig with rename on import and automatic backup
triggers:
  - merge kubeconfig
  - add cluster to kubeconfig
  - import kubeconfig
  - rename context kubeconfig
  - combine kubeconfig files
  - switch kube context
  - delete kube context
  - list kube contexts
---

# konfuse

Merge any kubeconfig file into your existing `~/.kube/config`, switch the active context, and delete contexts cleanly.

## Merge

```bash
# Preview what will change (always do this first)
konfuse merge <file> --dry-run

# Basic merge (asks for confirmation when overwriting an existing kubeconfig)
konfuse merge <file>

# Shortcut: a bare file argument also dispatches to merge
konfuse <file>

# Rename context and cluster on import (recommended)
konfuse merge <file> --rename-context <name> --rename-cluster <name>

# Rename all three (context, cluster, user)
konfuse merge <file> --rename-context <name> --rename-cluster <name> --rename-user <name>

# Non-interactive / CI mode with JSON output
konfuse merge <file> --yes --json

# Target a different kubeconfig
konfuse merge <file> --kubeconfig /path/to/config
```

## List, switch, delete

```bash
# List contexts, clusters, and users (current context marked with *)
konfuse list
konfuse list --json

# Switch the active context (no-op + no backup when already current)
konfuse use prod

# Delete a context and any orphaned cluster/user it referenced
konfuse delete old-staging         # prompts for confirmation
konfuse delete old-staging --yes   # skip the prompt
```

## Top-level

- `konfuse --help` lists every command. `konfuse <command> -h` shows command-specific help.
- `konfuse --version` prints the version. Works at any position (e.g. `konfuse list --version` also prints the version).

## Rules

- Always use `--dry-run` before merging in automated or unfamiliar contexts
- Use `--rename-context` and `--rename-cluster` when the incoming file uses generic names like `kubernetes-admin@cluster.local`
- Only the **first** context/cluster/user in the incoming file is renamed; others pass through unchanged
- `merge` (over an existing file) and `delete` prompt for confirmation in interactive shells. `--yes`, `--json`, or non-TTY stdin auto-skip the prompt. Aborting exits with code 2 and writes nothing.
- A timestamped backup is created before any destructive write. `use` only writes (and only backs up) when the active context actually changes. Restore with `cp ~/.kube/config.backup.<timestamp> ~/.kube/config`.
- `--json` is auto-enabled when stdout is not a TTY

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error (load / parse / write failure) |
| 2 | Usage error or user-aborted prompt |
| 3 | Input file or kubeconfig not found |

## JSON output schemas

`merge`:
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

`list`:
```json
{
  "current_context": "prod",
  "contexts": [
    { "name": "prod", "cluster": "eks-prod", "user": "eks-prod-user" }
  ],
  "clusters": ["eks-prod"],
  "users":    ["eks-prod-user"]
}
```

`delete`:
```json
{
  "target": "/Users/you/.kube/config",
  "backup": "/Users/you/.kube/config.backup.20260328T120000",
  "deleted": { "context": "old-staging", "cluster": "old-cluster", "user": "old-user" }
}
```

`use` (no-op switches return `backup: null` and `used.changed: false`):
```json
{
  "target": "/Users/you/.kube/config",
  "backup": "/Users/you/.kube/config.backup.20260328T120000",
  "used": { "context": "prod", "previous": "staging", "changed": true }
}
```
