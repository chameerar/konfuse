"""
konfuse: Merge a new kubeconfig into your existing kubeconfig.

Takes a new kubeconfig YAML file and merges its clusters, users, and contexts
into the existing kubeconfig. Backs up the existing config before any changes.
"""

import argparse
import json
import os
import shutil
import sys
from datetime import datetime

import yaml

DEFAULT_KUBECONFIG = os.path.expanduser("~/.kube/config")

# Exit codes
EXIT_OK = 0
EXIT_ERROR = 1
EXIT_USAGE = 2
EXIT_NOT_FOUND = 3


def _is_tty():
    return sys.stdout.isatty()


def load_yaml(path):
    """Load and parse a YAML file."""
    with open(path, "r") as f:
        return yaml.safe_load(f)


def save_yaml(data, path):
    """Write data to a YAML file, creating parent directories if needed."""
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    with open(path, "w") as f:
        yaml.dump(data, f, default_flow_style=False, sort_keys=False)


def find_by_name(items, name):
    """Find an item in a list by its 'name' field."""
    for item in items:
        if item.get("name") == name:
            return item
    return None


def backup_config(config_path):
    """Create a timestamped backup of the given config file."""
    if not os.path.exists(config_path):
        return None
    timestamp = datetime.now().strftime("%Y%m%dT%H%M%S")
    backup_path = f"{config_path}.backup.{timestamp}"
    shutil.copy2(config_path, backup_path)
    return backup_path


def merge_kubeconfig(
    existing, incoming, rename_context=None, rename_cluster=None, rename_user=None
):
    """Merge incoming kubeconfig entries into existing kubeconfig.

    Args:
        existing:       Parsed existing kubeconfig dict (or None for fresh start).
        incoming:       Parsed incoming kubeconfig dict to merge in.
        rename_context: If set, rename the first incoming context to this value.
        rename_cluster: If set, rename the first incoming cluster to this value.
        rename_user:    If set, rename the first incoming user to this value.

    Returns:
        Tuple of (merged dict, result dict) where result contains lists of
        added/replaced entries per section for structured output.
    """
    result = {
        "clusters": {"added": [], "replaced": []},
        "users": {"added": [], "replaced": []},
        "contexts": {"added": [], "replaced": []},
    }

    if existing is None:
        existing = {
            "apiVersion": "v1",
            "kind": "Config",
            "clusters": [],
            "contexts": [],
            "users": [],
            "current-context": "",
            "preferences": {},
        }

    for key in ("clusters", "contexts", "users"):
        if existing.get(key) is None:
            existing[key] = []
        if incoming.get(key) is None:
            incoming[key] = []

    # Determine original → new names for first entries
    orig_cluster_name = new_cluster_name = None
    orig_context_name = new_context_name = None
    orig_user_name = new_user_name = None

    if incoming["clusters"]:
        orig_cluster_name = incoming["clusters"][0]["name"]
        new_cluster_name = rename_cluster or orig_cluster_name

    if incoming["contexts"]:
        orig_context_name = incoming["contexts"][0]["name"]
        new_context_name = rename_context or orig_context_name

    if incoming["users"]:
        orig_user_name = incoming["users"][0]["name"]
        new_user_name = rename_user or orig_user_name

    # Merge clusters
    for cluster in incoming["clusters"]:
        is_first = cluster["name"] == orig_cluster_name and rename_cluster
        name = new_cluster_name if is_first else cluster["name"]
        entry = {"name": name, "cluster": cluster["cluster"]}
        existing_entry = find_by_name(existing["clusters"], name)
        if existing_entry:
            existing["clusters"].remove(existing_entry)
            result["clusters"]["replaced"].append(name)
        else:
            result["clusters"]["added"].append(name)
        existing["clusters"].append(entry)

    # Merge users
    for user in incoming["users"]:
        is_first = user["name"] == orig_user_name and rename_user
        name = new_user_name if is_first else user["name"]
        entry = {"name": name, "user": user["user"]}
        existing_entry = find_by_name(existing["users"], name)
        if existing_entry:
            existing["users"].remove(existing_entry)
            result["users"]["replaced"].append(name)
        else:
            result["users"]["added"].append(name)
        existing["users"].append(entry)

    # Merge contexts
    for ctx in incoming["contexts"]:
        is_first = ctx["name"] == orig_context_name and rename_context
        name = new_context_name if is_first else ctx["name"]
        context_data = dict(ctx["context"])
        if rename_cluster and context_data.get("cluster") == orig_cluster_name:
            context_data["cluster"] = new_cluster_name
        if rename_user and context_data.get("user") == orig_user_name:
            context_data["user"] = new_user_name
        entry = {"name": name, "context": context_data}
        existing_entry = find_by_name(existing["contexts"], name)
        if existing_entry:
            existing["contexts"].remove(existing_entry)
            result["contexts"]["replaced"].append(name)
        else:
            result["contexts"]["added"].append(name)
        existing["contexts"].append(entry)

    return existing, result


def main():
    parser = argparse.ArgumentParser(
        prog="konfuse",
        description="Merge a new kubeconfig file into your existing kubeconfig.",
        epilog="Examples:\n"
               "  konfuse new-cluster.yaml\n"
               "  konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod\n"
               "  konfuse new-cluster.yaml --dry-run --json\n"
               "  konfuse new-cluster.yaml --kubeconfig /path/to/config\n",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("input", help="Path to the kubeconfig YAML file to merge")
    parser.add_argument("--rename-context", metavar="NAME", help="Rename the first incoming context")
    parser.add_argument("--rename-cluster", metavar="NAME", help="Rename the first incoming cluster")
    parser.add_argument("--rename-user", metavar="NAME", help="Rename the first incoming user")
    parser.add_argument(
        "--kubeconfig",
        default=DEFAULT_KUBECONFIG,
        metavar="PATH",
        help=f"Target kubeconfig to merge into (default: {DEFAULT_KUBECONFIG})",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Preview what would be merged without writing any changes",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        dest="json_output",
        help="Output results as JSON (default when stdout is not a TTY)",
    )
    parser.add_argument(
        "--yes",
        action="store_true",
        help="Skip all confirmation prompts (non-interactive mode)",
    )

    args = parser.parse_args()

    use_json = args.json_output or not _is_tty()

    def emit(data):
        """Print structured JSON result and exit."""
        print(json.dumps(data, indent=2))

    def fail(message, hint=None, code=EXIT_ERROR):
        if use_json:
            payload = {"error": message}
            if hint:
                payload["hint"] = hint
            print(json.dumps(payload), file=sys.stderr)
        else:
            print(f"Error: {message}", file=sys.stderr)
            if hint:
                print(f"Try:   {hint}", file=sys.stderr)
        sys.exit(code)

    # Validate input
    if not os.path.isfile(args.input):
        fail(
            f"Input file not found: {args.input}",
            hint="konfuse <path-to-kubeconfig.yaml>",
            code=EXIT_NOT_FOUND,
        )

    incoming = load_yaml(args.input)
    if not incoming or incoming.get("kind") != "Config":
        fail(
            "Input file is not a valid kubeconfig (missing kind: Config)",
            hint="Ensure the file is a valid kubeconfig YAML",
        )

    # Load existing config
    existing = None
    existing_path_exists = os.path.isfile(args.kubeconfig)
    if existing_path_exists:
        existing = load_yaml(args.kubeconfig)

    # Compute merge (pure — no I/O)
    merged, result = merge_kubeconfig(
        existing, incoming, args.rename_context, args.rename_cluster, args.rename_user
    )

    has_conflicts = any(result[s]["replaced"] for s in result)

    if args.dry_run:
        output = {
            "dry_run": True,
            "target": args.kubeconfig,
            "changes": result,
            "has_conflicts": has_conflicts,
        }
        if use_json:
            emit(output)
        else:
            print("Dry run — no changes will be written\n")
            for section, ops in result.items():
                for name in ops["added"]:
                    print(f"  ✓  Would add {section[:-1]}: {name}")
                for name in ops["replaced"]:
                    print(f"  ⚠  Would replace {section[:-1]}: {name}")
            if has_conflicts:
                print("\n⚠  Conflicts detected. Use --rename-* flags to avoid replacing existing entries.")
        sys.exit(EXIT_OK)

    # Backup + save
    backup_path = None
    if existing_path_exists:
        backup_path = backup_config(args.kubeconfig)
    save_yaml(merged, args.kubeconfig)

    output = {
        "dry_run": False,
        "target": args.kubeconfig,
        "backup": backup_path,
        "changes": result,
        "has_conflicts": has_conflicts,
    }

    if use_json:
        emit(output)
    else:
        if backup_path:
            print(f"💾 Backup saved: {backup_path}")
        print()
        for section, ops in result.items():
            for name in ops["added"]:
                print(f"  ✓  Added {section[:-1]}: {name}")
            for name in ops["replaced"]:
                print(f"  ⚠  Replaced {section[:-1]}: {name}")
        if has_conflicts:
            print(
                "\n⚠  Some entries were replaced. Use --rename-* flags to keep both versions."
            )
        print(f"\n✅ Merged config saved to: {args.kubeconfig}")

    sys.exit(EXIT_OK)


if __name__ == "__main__":
    main()
