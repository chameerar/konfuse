from konfuse import backup_config, merge_kubeconfig

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def make_kubeconfig(contexts=None, clusters=None, users=None):
    return {
        "apiVersion": "v1",
        "kind": "Config",
        "clusters": clusters or [],
        "contexts": contexts or [],
        "users": users or [],
        "current-context": "",
        "preferences": {},
    }


def cluster(name, server="https://example.com"):
    return {"name": name, "cluster": {"server": server}}


def user(name, token="tok"):
    return {"name": name, "user": {"token": token}}


def context(name, cluster_name, user_name="admin"):
    return {"name": name, "context": {"cluster": cluster_name, "user": user_name}}


def merge(*args, **kwargs):
    """Unwrap the (merged, result) tuple for tests that only need the merged dict."""
    merged, _ = merge_kubeconfig(*args, **kwargs)
    return merged


def merge_result(*args, **kwargs):
    """Unwrap the (merged, result) tuple for tests that need the result dict."""
    _, result = merge_kubeconfig(*args, **kwargs)
    return result


# ---------------------------------------------------------------------------
# Merge into empty
# ---------------------------------------------------------------------------

class TestMergeIntoEmpty:
    def test_creates_default_structure(self):
        incoming = make_kubeconfig(clusters=[cluster("c1")], users=[user("u1")],
                                   contexts=[context("ctx1", "c1", "u1")])
        result = merge(None, incoming)
        assert result["apiVersion"] == "v1"
        assert result["kind"] == "Config"

    def test_entries_present(self):
        incoming = make_kubeconfig(clusters=[cluster("c1")], users=[user("u1")],
                                   contexts=[context("ctx1", "c1", "u1")])
        result = merge(None, incoming)
        assert any(e["name"] == "c1" for e in result["clusters"])
        assert any(e["name"] == "u1" for e in result["users"])
        assert any(e["name"] == "ctx1" for e in result["contexts"])


# ---------------------------------------------------------------------------
# No rename
# ---------------------------------------------------------------------------

class TestMergeNoRename:
    def test_names_preserved(self):
        existing = make_kubeconfig(clusters=[cluster("existing-c")])
        incoming = make_kubeconfig(clusters=[cluster("new-c")], users=[user("new-u")],
                                   contexts=[context("new-ctx", "new-c", "new-u")])
        result = merge(existing, incoming)
        names = [e["name"] for e in result["clusters"]]
        assert "existing-c" in names
        assert "new-c" in names

    def test_empty_incoming_sections(self):
        existing = make_kubeconfig(clusters=[cluster("c1")])
        result = merge(existing, make_kubeconfig())
        assert len(result["clusters"]) == 1


# ---------------------------------------------------------------------------
# Rename context
# ---------------------------------------------------------------------------

class TestRenameContext:
    def test_first_context_renamed(self):
        incoming = make_kubeconfig(clusters=[cluster("c1")], users=[user("u1")],
                                   contexts=[context("orig-ctx", "c1", "u1")])
        result = merge(None, incoming, rename_context="prod")
        assert any(e["name"] == "prod" for e in result["contexts"])
        assert not any(e["name"] == "orig-ctx" for e in result["contexts"])

    def test_second_context_not_renamed(self):
        incoming = make_kubeconfig(clusters=[cluster("c1"), cluster("c2")],
                                   users=[user("u1")],
                                   contexts=[context("ctx1", "c1"), context("ctx2", "c2")])
        result = merge(None, incoming, rename_context="prod")
        names = [e["name"] for e in result["contexts"]]
        assert "prod" in names
        assert "ctx2" in names


# ---------------------------------------------------------------------------
# Rename cluster
# ---------------------------------------------------------------------------

class TestRenameCluster:
    def test_first_cluster_renamed(self):
        incoming = make_kubeconfig(clusters=[cluster("orig-c")], users=[user("u1")],
                                   contexts=[context("ctx1", "orig-c")])
        result = merge(None, incoming, rename_cluster="prod-cluster")
        assert any(e["name"] == "prod-cluster" for e in result["clusters"])
        assert not any(e["name"] == "orig-c" for e in result["clusters"])

    def test_cluster_reference_updated_in_context(self):
        incoming = make_kubeconfig(clusters=[cluster("orig-c")], users=[user("u1")],
                                   contexts=[context("ctx1", "orig-c")])
        result = merge(None, incoming, rename_cluster="prod-cluster")
        ctx = next(e for e in result["contexts"] if e["name"] == "ctx1")
        assert ctx["context"]["cluster"] == "prod-cluster"


# ---------------------------------------------------------------------------
# Rename user
# ---------------------------------------------------------------------------

class TestRenameUser:
    def test_first_user_renamed(self):
        incoming = make_kubeconfig(clusters=[cluster("c1")], users=[user("orig-u")],
                                   contexts=[context("ctx1", "c1", "orig-u")])
        result = merge(None, incoming, rename_user="admin")
        assert any(e["name"] == "admin" for e in result["users"])
        assert not any(e["name"] == "orig-u" for e in result["users"])

    def test_user_reference_updated_in_context(self):
        incoming = make_kubeconfig(clusters=[cluster("c1")], users=[user("orig-u")],
                                   contexts=[context("ctx1", "c1", "orig-u")])
        result = merge(None, incoming, rename_user="admin")
        ctx = next(e for e in result["contexts"] if e["name"] == "ctx1")
        assert ctx["context"]["user"] == "admin"

    def test_second_user_not_renamed(self):
        incoming = make_kubeconfig(clusters=[cluster("c1")], users=[user("u1"), user("u2")],
                                   contexts=[context("ctx1", "c1", "u1")])
        result = merge(None, incoming, rename_user="admin")
        names = [e["name"] for e in result["users"]]
        assert "admin" in names
        assert "u2" in names


# ---------------------------------------------------------------------------
# Rename all three
# ---------------------------------------------------------------------------

class TestRenameAll:
    def test_all_renamed_and_references_updated(self):
        incoming = make_kubeconfig(clusters=[cluster("orig-c")], users=[user("orig-u")],
                                   contexts=[context("orig-ctx", "orig-c", "orig-u")])
        result = merge(None, incoming, rename_context="prod",
                       rename_cluster="prod-c", rename_user="prod-u")
        assert any(e["name"] == "prod" for e in result["contexts"])
        assert any(e["name"] == "prod-c" for e in result["clusters"])
        assert any(e["name"] == "prod-u" for e in result["users"])
        ctx = next(e for e in result["contexts"] if e["name"] == "prod")
        assert ctx["context"]["cluster"] == "prod-c"
        assert ctx["context"]["user"] == "prod-u"


# ---------------------------------------------------------------------------
# Conflicts
# ---------------------------------------------------------------------------

class TestConflicts:
    def test_existing_cluster_replaced(self):
        existing = make_kubeconfig(clusters=[cluster("c1", "https://old.example.com")])
        incoming = make_kubeconfig(clusters=[cluster("c1", "https://new.example.com")])
        result = merge(existing, incoming)
        assert len([e for e in result["clusters"] if e["name"] == "c1"]) == 1
        assert result["clusters"][-1]["cluster"]["server"] == "https://new.example.com"

    def test_conflict_recorded_in_result(self):
        existing = make_kubeconfig(clusters=[cluster("c1")])
        incoming = make_kubeconfig(clusters=[cluster("c1")])
        r = merge_result(existing, incoming)
        assert "c1" in r["clusters"]["replaced"]
        assert "c1" not in r["clusters"]["added"]

    def test_new_entry_recorded_in_result(self):
        incoming = make_kubeconfig(clusters=[cluster("new-c")])
        r = merge_result(None, incoming)
        assert "new-c" in r["clusters"]["added"]


# ---------------------------------------------------------------------------
# backup_config
# ---------------------------------------------------------------------------

class TestBackupConfig:
    def test_backup_created(self, tmp_path):
        config = tmp_path / "config"
        config.write_text("original content")
        backup_path = backup_config(str(config))
        assert backup_path is not None
        with open(backup_path) as f:
            assert f.read() == "original content"

    def test_no_backup_if_file_missing(self, tmp_path):
        assert backup_config(str(tmp_path / "nonexistent")) is None

    def test_backup_path_contains_timestamp(self, tmp_path):
        config = tmp_path / "config"
        config.write_text("x")
        backup_path = backup_config(str(config))
        assert ".backup." in backup_path
