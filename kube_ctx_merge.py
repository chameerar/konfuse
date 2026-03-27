"""
kube-ctx-merge: Merge a new kubeconfig into your existing kubeconfig.

Takes a new kubeconfig YAML file and merges its clusters, users, and contexts
into the existing kubeconfig. Backs up the existing config before any changes.
"""

import argparse
import os
import sys
import shutil
from datetime import datetime

import yaml


DEFAULT_KUBECONFIG = os.path.expanduser("~/.kube/config")


def load_yaml(path):
    """Load and parse a YAML file."""
    with open(path, "r") as f:
        return yaml.safe_load(f)


def save_yaml(data, path):
    """Write data to a YAML file, creating parent directories if needed."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
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


def merge_kubeconfig(existing, incoming, rename_context=None, rename_cluster=None):
    """Merge incoming kubeconfig entries into existing kubeconfig.

    Args:
        existing:       Parsed existing kubeconfig dict (or None for fresh start).
        incoming:       Parsed incoming kubeconfig dict to merge in.
        rename_context: If set, rename the first incoming context to this value.
        rename_cluster: If set, rename the first incoming cluster to this value.

    Returns:
        The merged kubeconfig dict.
    """
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

    # Determine original names from incoming config
    orig_cluster_name = None
    new_cluster_name = None
    orig_context_name = None
    new_context_name = None

    if incoming["clusters"]:
        orig_cluster_name = incoming["clusters"][0]["name"]
        new_cluster_name = rename_cluster if rename_cluster else orig_cluster_name

    if incoming["contexts"]:
        orig_context_name = incoming["contexts"][0]["name"]
        new_context_name = rename_context if rename_context else orig_context_name

    # Merge clusters
    for cluster in incoming["clusters"]:
        name = new_cluster_name if cluster["name"] == orig_cluster_name and rename_cluster else cluster["name"]
        entry = {"name": name, "cluster": cluster["cluster"]}
        existing_entry = find_by_name(existing["clusters"], name)
        if existing_entry:
            existing["clusters"].remove(existing_entry)
            print(f"  ⚠  Replacing existing cluster: {name}")
        else:
            print(f"  ✓  Adding cluster: {name}")
        existing["clusters"].append(entry)

    # Merge users
    for user in incoming["users"]:
        name = user["name"]
        entry = {"name": name, "user": user["user"]}
        existing_entry = find_by_name(existing["users"], name)
        if existing_entry:
            existing["users"].remove(existing_entry)
            print(f"  ⚠  Replacing existing user: {name}")
        else:
            print(f"  ✓  Adding user: {name}")
        existing["users"].append(entry)

    # Merge contexts (apply renames for both context name and cluster reference)
    for ctx in incoming["contexts"]:
        name = new_context_name if ctx["name"] == orig_context_name and rename_context else ctx["name"]
        context_data = dict(ctx["context"])
        # Update cluster reference if cluster was renamed
        if rename_cluster and context_data.get("cluster") == orig_cluster_name:
            context_data["cluster"] = new_cluster_name
        entry = {"name": name, "context": context_data}
        existing_entry = find_by_name(existing["contexts"], name)
        if existing_entry:
            existing["contexts"].remove(existing_entry)
            print(f"  ⚠  Replacing existing context: {name}")
        else:
            print(f"  ✓  Adding context: {name}")
        existing["contexts"].append(entry)

    return existing


def main():
    parser = argparse.ArgumentParser(
        prog="kube-ctx-merge",
        description="Merge a new kubeconfig file into your existing kubeconfig.",
        epilog="Examples:\n"
               "  kube-ctx-merge new-cluster.yaml\n"
               "  kube-ctx-merge new-cluster.yaml --rename-context my-prod --rename-cluster prod-cluster\n"
               "  kube-ctx-merge new-cluster.yaml --kubeconfig /path/to/config\n",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "input",
        help="Path to the new kubeconfig YAML file to merge",
    )
    parser.add_argument(
        "--rename-context",
        metavar="NAME",
        help="Rename the incoming context to NAME",
    )
    parser.add_argument(
        "--rename-cluster",
        metavar="NAME",
        help="Rename the incoming cluster to NAME",
    )
    parser.add_argument(
        "--kubeconfig",
        default=DEFAULT_KUBECONFIG,
        metavar="PATH",
        help=f"Path to the existing kubeconfig (default: {DEFAULT_KUBECONFIG})",
    )

    args = parser.parse_args()

    # Validate input file
    if not os.path.isfile(args.input):
        print(f"Error: Input file not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    print(f"📂 Loading incoming config: {args.input}")
    incoming = load_yaml(args.input)
    if not incoming or incoming.get("kind") != "Config":
        print("Error: Input file does not look like a valid kubeconfig (missing kind: Config)", file=sys.stderr)
        sys.exit(1)

    # Load existing config (or start fresh)
    existing = None
    if os.path.isfile(args.kubeconfig):
        print(f"📂 Loading existing config: {args.kubeconfig}")
        existing = load_yaml(args.kubeconfig)

        # Backup
        backup_path = backup_config(args.kubeconfig)
        print(f"💾 Backup saved: {backup_path}")
    else:
        print(f"📂 No existing config at {args.kubeconfig}, creating new one")

    # Merge
    print("\nMerging...")
    merged = merge_kubeconfig(existing, incoming, args.rename_context, args.rename_cluster)

    # Save
    save_yaml(merged, args.kubeconfig)
    print(f"\n✅ Merged config saved to: {args.kubeconfig}")


if __name__ == "__main__":
    main()
