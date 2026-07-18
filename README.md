# konfuse

> Merge any kubeconfig in one command. Rename on import. Never lose your existing config.

[![CI](https://github.com/chameerar/konfuse/actions/workflows/ci.yml/badge.svg)](https://github.com/chameerar/konfuse/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/chameerar/konfuse)](https://github.com/chameerar/konfuse/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Kubeconfigs are confusing enough. `konfuse` makes merging them less so.

Got a new cluster config from your ops team? Spinning up another EKS environment? `konfuse` merges it into your existing `~/.kube/config` in one command — with a friendly name, and a backup in case anything goes wrong.

![konfuse in action](docs/demo.gif)

## Why konfuse?

> Comparison covers the merge feature. See [Managing contexts](#managing-contexts) for the `list` / `use` / `delete` subcommands.

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

### Install with Go

Requires Go 1.26 or newer.

```bash
go install github.com/chameerar/konfuse@latest
```

### Build from source

```bash
git clone https://github.com/chameerar/konfuse.git
cd konfuse
go build -o konfuse .
```

### Uninstall

If you installed via binary download:

```bash
rm ~/.local/bin/konfuse
```

If you installed via `go install`:

```bash
rm $(go env GOPATH)/bin/konfuse
```

## Usage

```bash
# Preview what will change (no writes)
konfuse merge new-cluster.yaml --dry-run

# Merge into ~/.kube/config
konfuse merge new-cluster.yaml

# Rename context, cluster, and user on import
konfuse merge new-cluster.yaml --rename-context prod --rename-cluster eks-prod --rename-user eks-admin

# Machine-readable output (also auto-enabled in pipes/CI)
konfuse merge new-cluster.yaml --json

# Target a different kubeconfig
konfuse merge new-cluster.yaml --kubeconfig ~/.kube/work-config
```

Run `konfuse --help` for the full command list, or `konfuse <command> -h` for command-specific help. `konfuse --version` prints the version (works after any subcommand too).

### Merge options

| Option | Description |
|---|---|
| `input` (positional) | Path to the kubeconfig YAML to merge |
| `--rename-context NAME` | Rename the first incoming context |
| `--rename-cluster NAME` | Rename the first incoming cluster |
| `--rename-user NAME` | Rename the first incoming user |
| `--dry-run` | Preview changes without writing anything |
| `--json` | Output results as JSON (auto-enabled when stdout is not a TTY) |
| `--yes` | Skip the confirmation prompt before overwriting an existing kubeconfig |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |

## Managing contexts

```bash
# List contexts, clusters, and users (current context marked with *)
konfuse list

# Switch the active context
konfuse use prod

# Delete a context (also removes its cluster/user if no longer referenced)
konfuse delete old-staging
```

`delete` always creates a timestamped backup before writing and prompts for confirmation in interactive shells. `use` only creates a backup (and only writes) when the active context actually changes; no-op switches leave the file untouched. `list` is read-only.

### List options

| Option | Description |
|---|---|
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--json` | Output as JSON (auto-enabled when stdout is not a TTY) |

### Delete options

| Option | Description |
|---|---|
| `<context-name>` (positional) | Context to delete |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--json` | Output as JSON (auto-enabled when stdout is not a TTY) |
| `--yes` | Skip the confirmation prompt |

### Use options

| Option | Description |
|---|---|
| `<context-name>` (positional) | Context to switch to |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--json` | Output as JSON (auto-enabled when stdout is not a TTY) |

## Confirmation prompts

`merge` (when overwriting an existing kubeconfig) and `delete` ask for confirmation before writing. The prompt is auto-skipped when:

- `--yes` is set,
- `--json` is set, or
- stdin is not a TTY (pipes, CI).

Aborting at the prompt (any input other than `y` / `yes`) exits with code 2 and writes nothing.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error (load / parse / write failure) |
| 2 | Usage error or user-aborted prompt |
| 3 | Input file or kubeconfig not found |

## Example: EKS config with a friendly name

You receive `eks-staging.yaml` with context named `arn:aws:eks:us-east-1:123456789:cluster/staging`. Run:

```bash
konfuse merge eks-staging.yaml --rename-context staging --rename-cluster eks-staging
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

## Contributing

Contributions are welcome. konfuse is a single Go binary with no runtime dependencies (Go 1.26+ to build).

**Project layout:**

- `main.go` — CLI entry point: flag parsing, I/O, backups, output formatting
- `internal/merger/` — pure merge/list/use/delete logic (no I/O, fully unit-tested)

**Develop:**

```bash
git clone https://github.com/chameerar/konfuse.git
cd konfuse
go mod tidy

go build -o konfuse .        # build
go test ./...                # run tests
go vet ./...                 # vet
```

**Before opening a PR:**

- Run `go test ./...` and `go vet ./...` — both must pass (CI runs them on every push).
- Add or update tests for behavior changes. Keep merge logic in `internal/merger` (I/O-free and testable); keep I/O and formatting in `main.go`.
- Keep the `--json` output stable — it's a scripting/CI contract.
- Note user-facing changes in `CHANGELOG.md` under an "Unreleased" heading.

Have an idea or found a bug? [Open an issue](https://github.com/chameerar/konfuse/issues) to discuss before large changes.

## License

MIT
