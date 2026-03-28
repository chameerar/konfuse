# konfuse

> Merge any kubeconfig in one command. Rename on import. Never lose your existing config.

[![CI](https://github.com/chameerar/konfuse/actions/workflows/ci.yml/badge.svg)](https://github.com/chameerar/konfuse/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/chameerar/konfuse)](https://github.com/chameerar/konfuse/releases/latest)

Kubeconfigs are confusing enough. `konfuse` makes merging them less so.

Got a new cluster config from your ops team? Spinning up another EKS environment? `konfuse` merges it into your existing `~/.kube/config` in one command — with a friendly name, and a backup in case anything goes wrong.

## Why konfuse?

| Feature | konfuse | kubecm | kubectx | konfig |
|---|:---:|:---:|:---:|:---:|
| Merge kubeconfigs | ✓ | ✓ | ✗ | ✓ |
| Rename context on import | ✓ | ✗ | ✗ | ✗ |
| Rename cluster on import | ✓ | ✗ | ✗ | ✗ |
| Rename user on import | ✓ | ✗ | ✗ | ✗ |
| Auto timestamped backup | ✓ | ✗ | ✗ | ✗ |
| --dry-run / preview | ✓ | ✗ | ✗ | ✗ |
| --json structured output | ✓ | ✗ | ✗ | ✗ |
| Single binary, no runtime deps | ✓ | ✓ | ✓ | ✗ |
| kubectl plugin (Krew) | soon | ✓ | ✓ | ✓ |

## Installation

### Download binary (recommended)

```bash
mkdir -p ~/.local/bin

# macOS (Apple Silicon)
curl -L https://github.com/chameerar/konfuse/releases/latest/download/konfuse-macos-arm64 \
  -o ~/.local/bin/konfuse && chmod +x ~/.local/bin/konfuse

# macOS (Intel)
curl -L https://github.com/chameerar/konfuse/releases/latest/download/konfuse-macos-amd64 \
  -o ~/.local/bin/konfuse && chmod +x ~/.local/bin/konfuse

# Linux (amd64)
curl -L https://github.com/chameerar/konfuse/releases/latest/download/konfuse-linux-amd64 \
  -o ~/.local/bin/konfuse && chmod +x ~/.local/bin/konfuse

# Linux (arm64)
curl -L https://github.com/chameerar/konfuse/releases/latest/download/konfuse-linux-arm64 \
  -o ~/.local/bin/konfuse && chmod +x ~/.local/bin/konfuse
```

Make sure `~/.local/bin` is on your PATH (add to `~/.zshrc` or `~/.bashrc` if needed):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Build from source

```bash
go install github.com/chameerar/konfuse@latest
```

## Usage

```bash
# Preview what will change (no writes)
konfuse new-cluster.yaml --dry-run

# Merge into ~/.kube/config
konfuse new-cluster.yaml

# Rename context, cluster, and user on import
konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod --rename-user eks-admin

# Machine-readable output (also auto-enabled in pipes/CI)
konfuse new-cluster.yaml --json

# Target a different kubeconfig
konfuse new-cluster.yaml --kubeconfig ~/.kube/work-config
```

### Options

| Option | Description |
|---|---|
| `input` (positional) | Path to the kubeconfig YAML to merge |
| `--rename-context NAME` | Rename the first incoming context |
| `--rename-cluster NAME` | Rename the first incoming cluster |
| `--rename-user NAME` | Rename the first incoming user |
| `--dry-run` | Preview changes without writing anything |
| `--json` | Output results as JSON (auto-enabled when stdout is not a TTY) |
| `--yes` | Non-interactive mode — skip all prompts |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |

## Example: EKS config with a friendly name

You receive `eks-staging.yaml` with context named `arn:aws:eks:us-east-1:123456789:cluster/staging`. Run:

```bash
konfuse eks-staging.yaml --rename-context staging --rename-cluster eks-staging
```

**Before:**
```
$ kubectl config get-contexts
CURRENT   NAME       CLUSTER    AUTHINFO
*         minikube   minikube   minikube
```

**After:**
```
$ kubectl config get-contexts
CURRENT   NAME       CLUSTER       AUTHINFO
*         minikube   minikube      minikube
          staging    eks-staging   arn:aws:eks:...
```

## How it works

1. Validates the input file is a valid kubeconfig (`kind: Config`)
2. Backs up your existing config to `~/.kube/config.backup.<YYYYMMDDTHHMMSS>`
3. Merges clusters, users, and contexts — renaming the first entry if flags are set
4. Updates internal cluster/user references when `--rename-*` flags are used
5. Saves the merged result

Conflicts (same name already exists) are handled non-fatally: the incoming entry replaces the existing one with a warning.

## Restore a backup

```bash
cp ~/.kube/config.backup.20260328T120000 ~/.kube/config
```

## License

MIT
