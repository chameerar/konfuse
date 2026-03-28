from konfuse import backup_config, merge_kubeconfig

# ---------------------------------------------------------------------------
# Fixtures
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


# ---------------------------------------------------------------------------
# merge_kubeconfig tests
# ---------------------------------------------------------------------------

class TestMergeIntoEmpty:
    def test_creates_default_structure(self):
        incoming = make_kubeconfig(
            clusters=[cluster("c1")],
            users=[user("u1")],
            contexts=[context("ctx1", "c1", "u1")],
        )
        result = merge_kubeconfig(None, incoming)
        assert result["apiVersion"] == "v1"
        assert result["kind"] == "Config"

    def test_entries_present(self):
        incoming = make_kubeconfig(
            clusters=[cluster("c1")],
            users=[user("u1")],
            contexts=[context("ctx1", "c1", "u1")],
        )
        result = merge_kubeconfig(None, incoming)
        assert any(e["name"] == "c1" for e in result["clusters"])
        assert any(e["name"] == "u1" for e in result["users"])
        assert any(e["name"] == "ctx1" for e in result["contexts"])


class TestMergeNoRename:
    def test_names_preserved(self):
        existing = make_kubeconfig(clusters=[cluster("existing-c")])
        incoming = make_kubeconfig(
            clusters=[cluster("new-c")],
            users=[user("new-u")],
            contexts=[context("new-ctx", "new-c", "new-u")],
        )
        result = merge_kubeconfig(existing, incoming)
        names = [e["name"] for e in result["clusters"]]
        assert "existing-c" in names
        assert "new-c" in names

    def test_incoming_empty_sections(self):
        existing = make_kubeconfig(clusters=[cluster("c1")])
        incoming = make_kubeconfig()  # no clusters/users/contexts
        result = merge_kubeconfig(existing, incoming)
        assert len(result["clusters"]) == 1


class TestRenameContext:
    def test_first_context_renamed(self):
        incoming = make_kubeconfig(
            clusters=[cluster("c1")],
            users=[user("u1")],
            contexts=[context("orig-ctx", "c1", "u1")],
        )
        result = merge_kubeconfig(None, incoming, rename_context="prod")
        assert any(e["name"] == "prod" for e in result["contexts"])
        assert not any(e["name"] == "orig-ctx" for e in result["contexts"])

    def test_second_context_not_renamed(self):
        incoming = make_kubeconfig(
            clusters=[cluster("c1"), cluster("c2")],
            users=[user("u1")],
            contexts=[context("ctx1", "c1"), context("ctx2", "c2")],
        )
        result = merge_kubeconfig(None, incoming, rename_context="prod")
        names = [e["name"] for e in result["contexts"]]
        assert "prod" in names
        assert "ctx2" in names


class TestRenameCluster:
    def test_first_cluster_renamed(self):
        incoming = make_kubeconfig(
            clusters=[cluster("orig-c")],
            users=[user("u1")],
            contexts=[context("ctx1", "orig-c")],
        )
        result = merge_kubeconfig(None, incoming, rename_cluster="prod-cluster")
        assert any(e["name"] == "prod-cluster" for e in result["clusters"])
        assert not any(e["name"] == "orig-c" for e in result["clusters"])

    def test_cluster_reference_updated_in_context(self):
        incoming = make_kubeconfig(
            clusters=[cluster("orig-c")],
            users=[user("u1")],
            contexts=[context("ctx1", "orig-c")],
        )
        result = merge_kubeconfig(None, incoming, rename_cluster="prod-cluster")
        ctx = next(e for e in result["contexts"] if e["name"] == "ctx1")
        assert ctx["context"]["cluster"] == "prod-cluster"


class TestRenameBoth:
    def test_both_renamed_and_reference_updated(self):
        incoming = make_kubeconfig(
            clusters=[cluster("orig-c")],
            users=[user("u1")],
            contexts=[context("orig-ctx", "orig-c")],
        )
        result = merge_kubeconfig(None, incoming, rename_context="prod", rename_cluster="prod-c")
        assert any(e["name"] == "prod" for e in result["contexts"])
        assert any(e["name"] == "prod-c" for e in result["clusters"])
        ctx = next(e for e in result["contexts"] if e["name"] == "prod")
        assert ctx["context"]["cluster"] == "prod-c"


class TestConflicts:
    def test_existing_cluster_replaced_with_warning(self, capsys):
        existing = make_kubeconfig(clusters=[cluster("c1", "https://old.example.com")])
        incoming = make_kubeconfig(clusters=[cluster("c1", "https://new.example.com")])
        result = merge_kubeconfig(existing, incoming)
        out = capsys.readouterr().out
        assert "⚠" in out
        assert len([e for e in result["clusters"] if e["name"] == "c1"]) == 1
        assert result["clusters"][-1]["cluster"]["server"] == "https://new.example.com"

    def test_existing_context_replaced(self, capsys):
        existing = make_kubeconfig(contexts=[context("ctx1", "c1")])
        incoming = make_kubeconfig(contexts=[context("ctx1", "c2")])
        result = merge_kubeconfig(existing, incoming)
        assert len([e for e in result["contexts"] if e["name"] == "ctx1"]) == 1
        assert result["contexts"][-1]["context"]["cluster"] == "c2"


class TestInvalidInput:
    def test_missing_kind_config(self):
        # merge_kubeconfig itself does not validate kind — that's main()'s job.
        # But it should not crash on a minimal dict.
        incoming = {"apiVersion": "v1", "kind": "Config"}
        result = merge_kubeconfig(None, incoming)
        assert result["kind"] == "Config"


# ---------------------------------------------------------------------------
# backup_config tests
# ---------------------------------------------------------------------------

class TestBackupConfig:
    def test_backup_created(self, tmp_path):
        config = tmp_path / "config"
        config.write_text("original content")
        backup_path = backup_config(str(config))
        assert backup_path is not None
        assert backup_path.startswith(str(config))
        with open(backup_path) as f:
            assert f.read() == "original content"

    def test_no_backup_if_file_missing(self, tmp_path):
        result = backup_config(str(tmp_path / "nonexistent"))
        assert result is None

    def test_backup_path_contains_timestamp(self, tmp_path):
        config = tmp_path / "config"
        config.write_text("x")
        backup_path = backup_config(str(config))
        assert ".backup." in backup_path
