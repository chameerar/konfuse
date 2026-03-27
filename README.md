# kube-ctx-merge

A simple CLI tool to merge new kubeconfig files into your existing kubeconfig — without the headache.

If you've ever had to manually copy-paste clusters, contexts, and users between kubeconfig files, this tool is for you. It safely merges a new kubeconfig YAML into your existing one, with optional renaming and automatic backups.

## Features

- **One-command merge** — pass in a kubeconfig YAML and you're done
- **Automatic backup** — your existing config is backed up with a timestamp before any changes
- **Rename on import** — optionally rename the incoming context and/or cluster
- **Smart conflict handling** — warns you if an existing entry is being replaced
- **Configurable target** — defaults to `~/.kube/config` but can point anywhere

## Project Structure

```
kube-ctx-merge/
├── kube_ctx_merge.py   # Source code
├── README.md
├── LICENSE
└── .gitignore
```

## Requirements

- Python 3.6+
- [PyYAML](https://pypi.org/project/PyYAML/) (`pip install pyyaml`)

## Installation

### Option 1: Clone and run directly

```bash
git clone https://github.com/<your-username>/kube-ctx-merge.git
cd kube-ctx-merge

# Run directly
python3 kube_ctx_merge.py new-cluster.yaml
```

### Option 2: Create an alias

```bash
# Add to ~/.zshrc or ~/.bashrc
alias kube-ctx-merge="python3 /path/to/kube-ctx-merge/kube_ctx_merge.py"
```

### Option 3: Symlink to a directory on your PATH

```bash
ln -s /path/to/kube-ctx-merge/kube_ctx_merge.py /usr/local/bin/kube-ctx-merge
chmod +x /usr/local/bin/kube-ctx-merge
```

## Usage

```bash
# Basic: merge a new kubeconfig into ~/.kube/config
python3 kube_ctx_merge.py new-cluster.yaml

# Rename the incoming context and cluster during merge
python3 kube_ctx_merge.py new-cluster.yaml --rename-context my-prod --rename-cluster prod-cluster

# Target a different kubeconfig file
python3 kube_ctx_merge.py new-cluster.yaml --kubeconfig /path/to/custom/config
```

### Options

| Option | Description |
|---|---|
| `input` (positional, required) | Path to the new kubeconfig YAML file to merge |
| `--rename-context NAME` | Rename the incoming context to `NAME` |
| `--rename-cluster NAME` | Rename the incoming cluster to `NAME` |
| `--kubeconfig PATH` | Path to the target kubeconfig (default: `~/.kube/config`) |

## Examples

### Merge an EKS config with a friendly name

```bash
kube-ctx-merge eks-staging.yaml \
  --rename-context staging \
  --rename-cluster eks-staging
```

**Before:**

```
$ kubectl config get-contexts
CURRENT   NAME       CLUSTER        AUTHINFO
*         minikube   minikube       minikube
```

**After:**

```
$ kubectl config get-contexts
CURRENT   NAME       CLUSTER        AUTHINFO
*         minikube   minikube       minikube
          staging    eks-staging    arn:aws:eks:...
```

### Merge without renaming

```bash
kube-ctx-merge rancher-export.yaml
```

The context, cluster, and user names from the incoming file are preserved as-is.

## How It Works

1. **Validates** the input file is a valid kubeconfig (`kind: Config`)
2. **Backs up** the existing config to `~/.kube/config.backup.<YYYYMMDDTHHMMSS>`
3. **Merges** clusters, users, and contexts into the existing config
4. If `--rename-context` or `--rename-cluster` is used, renames the **first** context/cluster in the incoming file and updates internal references (e.g., the cluster reference inside the context)
5. **Saves** the merged result

If a cluster, user, or context with the same name already exists, it is **replaced** and a warning is printed.

## Backups

Every run creates a timestamped backup before modifying anything:

```
~/.kube/config.backup.20260327T103000
~/.kube/config.backup.20260327T114500
```

You can restore any backup by copying it back:

```bash
cp ~/.kube/config.backup.20260327T103000 ~/.kube/config
```

## License

MIT
