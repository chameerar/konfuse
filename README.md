# konfuse

> Merge any kubeconfig in one command. Rename on import. Never lose your existing config.

[![CI](https://github.com/chameerar/konfuse/actions/workflows/ci.yml/badge.svg)](https://github.com/chameerar/konfuse/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/chameerar/konfuse)](https://github.com/chameerar/konfuse/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**konfuse is a small, dependency-free CLI for taming your kubeconfig** — merge new cluster configs, list what you have, switch the active context, and delete contexts cleanly. One static binary, runs standalone or as a `kubectl` plugin.

Its signature move: when your ops team hands you a new cluster config (or you spin up another EKS environment), konfuse merges it into your existing `~/.kube/config` in a single command — giving the incoming context a friendly name and taking a timestamped backup first, so a bad merge is never a lost config.

![konfuse in action](docs/demo.gif)

## Why konfuse?

Plenty of tools switch contexts. What's missing is a safe, scriptable way to **bring a new config in** — konfuse is built around that gap.

| Feature | konfuse | kubecm | kubectx | konfig |
|---|:---:|:---:|:---:|:---:|
| Merge kubeconfigs | ✓ | ✓ | ✗ | ✓ |
| Rename context on import | ✓ | ✗ | ✗ | ✗ |
| Rename cluster on import | ✓ | ✗ | ✗ | ✗ |
| Rename user on import | ✓ | ✗ | ✗ | ✗ |
| Auto timestamped backup | ✓ | ✗ | ✗ | ✗ |
| `--dry-run` / preview | ✓ | ✗ | ✗ | ✗ |
| `--json` structured output | ✓ | ✗ | ✗ | ✗ |
| List / switch / delete contexts | ✓ | ✓ | ✓ | ✗ |
| Single binary, no runtime deps | ✓ | ✓ | ✓ | ✗ |

The rename-on-import and auto-backup columns are konfuse's alone — no other tool combines them.

## Installation

### Krew (recommended)

If you have [Krew](https://krew.sigs.k8s.io/) (the `kubectl` plugin manager) installed:

```bash
kubectl krew install konfuse
```

konfuse then runs as a `kubectl` plugin — the examples in this README use the standalone `konfuse` binary, so just prefix them with `kubectl`:

```bash
kubectl konfuse merge new-cluster.yaml --rename-context prod
kubectl konfuse list
```

### Download a binary

No Go toolchain required. Downloads to `~/.local/bin` (no sudo):

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

Make sure `~/.local/bin` is on your `PATH` (add to `~/.zshrc` or `~/.bashrc` if needed):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Go install

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

```bash
kubectl krew uninstall konfuse   # if installed via Krew
rm ~/.local/bin/konfuse          # if installed via binary download
rm $(go env GOPATH)/bin/konfuse  # if installed via go install
```

## Commands at a glance

```bash
konfuse merge <file>     # merge a kubeconfig into ~/.kube/config (rename + backup)
konfuse list             # list contexts, clusters, and users
konfuse use <context>    # switch the active context
konfuse delete <context> # delete a context and any orphaned cluster/user
```

Run `konfuse --help` for the full command list, `konfuse <command> -h` for command-specific help, and `konfuse --version` for the version (both work at any position in the arguments).

## Merge

The flagship command. Validates the incoming file is a real kubeconfig, backs up your current config, then merges in the new clusters, users, and contexts.

```bash
# Preview what will change — writes nothing
konfuse merge new-cluster.yaml --dry-run

# Merge into ~/.kube/config
konfuse merge new-cluster.yaml

# Rename the incoming context, cluster, and user on the way in
konfuse merge new-cluster.yaml --rename-context prod --rename-cluster eks-prod --rename-user eks-admin

# Target a different kubeconfig
konfuse merge new-cluster.yaml --kubeconfig ~/.kube/work-config
```

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

Only the **first** incoming cluster/context/user is renamed; any others pass through unchanged, with their internal references kept intact.

### How a merge works

1. Validates the input file is a valid kubeconfig (`kind: Config`).
2. Backs up your existing config to `~/.kube/config.backup.<YYYYMMDDTHHMMSS>`.
3. Merges clusters, users, and contexts — renaming the first entry if `--rename-*` flags are set.
4. Updates the internal cluster/user references inside the renamed context.
5. Writes the merged result.

Name conflicts (an incoming entry whose name already exists) are non-fatal: the incoming entry replaces the existing one and konfuse warns you. Pass `--rename-*` to keep both versions instead.

### Example: EKS config with a friendly name

You receive `eks-staging.yaml` with a context named `arn:aws:eks:us-east-1:123456789:cluster/staging`. Give it a name you'll actually remember:

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

### Restore a backup

Every write leaves a timestamped backup next to your config. Rolling back is a copy:

```bash
cp ~/.kube/config.backup.20260328T120000 ~/.kube/config
```

## Managing contexts

Beyond merging, konfuse handles the day-to-day of a growing kubeconfig:

```bash
# List contexts, clusters, and users (the current context is marked)
konfuse list

# Switch the active context
konfuse use prod

# Delete a context (also removes its cluster/user if nothing else references them)
konfuse delete old-staging
```

- `list` is read-only.
- `use` only writes (and only backs up) when the active context actually changes; a no-op switch leaves the file untouched.
- `delete` always backs up before writing and prompts for confirmation in interactive shells.

### list options

| Option | Description |
|---|---|
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--json` | Output as JSON (auto-enabled when stdout is not a TTY) |

### use options

| Option | Description |
|---|---|
| `<context-name>` (positional) | Context to switch to |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--json` | Output as JSON (auto-enabled when stdout is not a TTY) |

### delete options

| Option | Description |
|---|---|
| `<context-name>` (positional) | Context to delete |
| `--kubeconfig PATH` | Target kubeconfig (default: `~/.kube/config`) |
| `--json` | Output as JSON (auto-enabled when stdout is not a TTY) |
| `--yes` | Skip the confirmation prompt |

## Scripting & AI agents

konfuse is built to be driven by scripts, CI, and AI coding agents, not just typed by hand:

- **`--json` structured output** on every command — and it's auto-enabled whenever stdout isn't a TTY (pipes, CI), so you rarely need the flag. The JSON shape is a stable contract.
- **`--dry-run`** returns the full set of planned changes (as JSON too) without touching disk — safe to inspect before committing.
- **`--yes`** and non-interactive detection skip confirmation prompts unattended.
- **Meaningful exit codes** (see below) let callers branch on the outcome.
- **[`SKILL.md`](SKILL.md)** (Claude Code, Cursor, Gemini CLI) and **[`AGENTS.md`](AGENTS.md)** (OpenAI Codex) ship in the repo so agents know how to invoke konfuse correctly.

## Confirmation prompts

`merge` (when overwriting an existing kubeconfig) and `delete` ask for confirmation before writing. The prompt is auto-skipped when:

- `--yes` is set,
- `--json` is set, or
- stdin is not a TTY (pipes, CI).

Aborting at the prompt (any input other than `y` / `yes`) exits with code 2 and writes nothing. `use` never prompts.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error (load / parse / write failure) |
| 2 | Usage error or user-aborted prompt |
| 3 | Input file or kubeconfig not found |

## Contributing

Contributions are welcome. konfuse is a single Go binary with no runtime dependencies (Go 1.26+ to build).

**Project layout:**

- `main.go` — CLI entry point: flag parsing, I/O, backups, output formatting
- `internal/merger/` — pure merge / list / use / delete logic (no I/O, fully unit-tested)

**Develop:**

```bash
git clone https://github.com/chameerar/konfuse.git
cd konfuse
go mod tidy

go build -o konfuse .   # build
go test ./...           # run tests
go vet ./...            # vet
```

**Before opening a PR:**

- Run `go test ./...` and `go vet ./...` — both must pass (CI runs them on every push).
- Add or update tests for behavior changes. Keep merge logic in `internal/merger` (I/O-free and testable); keep I/O and formatting in `main.go`.
- Keep the `--json` output stable — it's a scripting/CI contract.
- Note user-facing changes in `CHANGELOG.md` under the `## [Unreleased]` heading.

Have an idea or found a bug? [Open an issue](https://github.com/chameerar/konfuse/issues) to discuss before large changes.

## License

MIT
