# konfuse

> Merge any kubeconfig in one command. Rename on import. Never lose your existing config.

[![CI](https://github.com/chameerar/konfuse/actions/workflows/ci.yml/badge.svg)](https://github.com/chameerar/konfuse/actions/workflows/ci.yml)
[![PyPI version](https://img.shields.io/pypi/v/konfuse)](https://pypi.org/project/konfuse/)
[![Python versions](https://img.shields.io/pypi/pyversions/konfuse)](https://pypi.org/project/konfuse/)
[![codecov](https://codecov.io/gh/chameerar/konfuse/branch/main/graph/badge.svg)](https://codecov.io/gh/chameerar/konfuse)

Kubeconfigs are confusing enough. `konfuse` makes merging them less so.

Got a new cluster config from your ops team? Spinning up another EKS environment? `konfuse` merges it into your existing `~/.kube/config` in one command — with a friendly name, and a backup in case anything goes wrong.

## Why konfuse?

| Feature | konfuse | kubecm | kubectx | konfig |
|---|:---:|:---:|:---:|:---:|
| Merge kubeconfigs | ✓ | ✓ | ✗ | ✓ |
| Rename context on import | ✓ | ✗ | ✗ | ✗ |
| Rename cluster on import | ✓ | ✗ | ✗ | ✗ |
| Auto timestamped backup | ✓ | ✗ | ✗ | ✗ |
| kubectl plugin (Krew) | soon | ✓ | ✓ | ✓ |

## Installation

```bash
# Recommended
pipx install konfuse

# Or with pip
pip install konfuse
```

## Usage

```bash
# Merge into ~/.kube/config
konfuse new-cluster.yaml

# Rename the context and cluster on import
konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod

# Target a different kubeconfig
konfuse new-cluster.yaml --kubeconfig ~/.kube/work-config
```

### Options

| Option | Description |
|---|---|
| `input` (positional) | Path to the kubeconfig YAML to merge |
| `--rename-context NAME` | Rename the incoming context to `NAME` |
| `--rename-cluster NAME` | Rename the incoming cluster to `NAME` |
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
4. Updates internal cluster references when `--rename-cluster` is used
5. Saves the merged result

Conflicts (same name already exists) are handled non-fatally: the incoming entry replaces the existing one with a warning.

## Restore a backup

```bash
cp ~/.kube/config.backup.20260327T103000 ~/.kube/config
```

## Requirements

- Python 3.8+
- PyYAML (installed automatically)

## License

MIT
