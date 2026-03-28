package merger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chameerar/konfuse/internal/merger"
)

// ---------------------------------------------------------------------------
// Test helpers (mirror Python's make_kubeconfig / cluster / user / context)
// ---------------------------------------------------------------------------

func makeCluster(name, server string) merger.NamedEntry {
	return merger.NamedEntry{
		Name: name,
		Body: map[string]interface{}{
			"cluster": map[string]interface{}{"server": server},
		},
	}
}

func makeUser(name, token string) merger.NamedEntry {
	return merger.NamedEntry{
		Name: name,
		Body: map[string]interface{}{
			"user": map[string]interface{}{"token": token},
		},
	}
}

func makeContext(name, clusterName, userName string) merger.NamedEntry {
	return merger.NamedEntry{
		Name: name,
		Body: map[string]interface{}{
			"context": map[string]interface{}{
				"cluster": clusterName,
				"user":    userName,
			},
		},
	}
}

func makeKubeConfig(clusters, users, contexts []merger.NamedEntry) *merger.KubeConfig {
	if clusters == nil {
		clusters = []merger.NamedEntry{}
	}
	if users == nil {
		users = []merger.NamedEntry{}
	}
	if contexts == nil {
		contexts = []merger.NamedEntry{}
	}
	return &merger.KubeConfig{
		APIVersion:     "v1",
		Kind:           "Config",
		Clusters:       clusters,
		Users:          users,
		Contexts:       contexts,
		CurrentContext: "",
		Preferences:    map[string]interface{}{},
	}
}

func findEntry(entries []merger.NamedEntry, name string) *merger.NamedEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

func contextField(entry *merger.NamedEntry, field string) string {
	if entry == nil {
		return ""
	}
	ctxData, ok := entry.Body["context"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := ctxData[field].(string)
	return v
}

// ---------------------------------------------------------------------------
// Merge into empty
// ---------------------------------------------------------------------------

func TestMergeIntoEmpty(t *testing.T) {
	incoming := makeKubeConfig(
		[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
		[]merger.NamedEntry{makeUser("u1", "tok")},
		[]merger.NamedEntry{makeContext("ctx1", "c1", "u1")},
	)

	t.Run("creates_default_structure", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "", "")
		if got.APIVersion != "v1" {
			t.Errorf("apiVersion = %q, want v1", got.APIVersion)
		}
		if got.Kind != "Config" {
			t.Errorf("kind = %q, want Config", got.Kind)
		}
	})

	t.Run("entries_present", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "", "")
		if findEntry(got.Clusters, "c1") == nil {
			t.Error("cluster c1 not found")
		}
		if findEntry(got.Users, "u1") == nil {
			t.Error("user u1 not found")
		}
		if findEntry(got.Contexts, "ctx1") == nil {
			t.Error("context ctx1 not found")
		}
	})
}

// ---------------------------------------------------------------------------
// No rename
// ---------------------------------------------------------------------------

func TestMergeNoRename(t *testing.T) {
	t.Run("names_preserved", func(t *testing.T) {
		existing := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("existing-c", "https://example.com")},
			nil, nil,
		)
		incoming := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("new-c", "https://example.com")},
			[]merger.NamedEntry{makeUser("new-u", "tok")},
			[]merger.NamedEntry{makeContext("new-ctx", "new-c", "new-u")},
		)
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if findEntry(got.Clusters, "existing-c") == nil {
			t.Error("existing cluster not found after merge")
		}
		if findEntry(got.Clusters, "new-c") == nil {
			t.Error("new cluster not found after merge")
		}
	})

	t.Run("empty_incoming_sections", func(t *testing.T) {
		existing := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			nil, nil,
		)
		got, _ := merger.MergeKubeconfig(existing, makeKubeConfig(nil, nil, nil), "", "", "")
		if len(got.Clusters) != 1 {
			t.Errorf("cluster count = %d, want 1", len(got.Clusters))
		}
	})
}

// ---------------------------------------------------------------------------
// Rename context
// ---------------------------------------------------------------------------

func TestRenameContext(t *testing.T) {
	incoming := makeKubeConfig(
		[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
		[]merger.NamedEntry{makeUser("u1", "tok")},
		[]merger.NamedEntry{makeContext("orig-ctx", "c1", "u1")},
	)

	t.Run("first_context_renamed", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "prod", "", "")
		if findEntry(got.Contexts, "prod") == nil {
			t.Error("renamed context 'prod' not found")
		}
		if findEntry(got.Contexts, "orig-ctx") != nil {
			t.Error("original context name 'orig-ctx' should not exist")
		}
	})

	t.Run("second_context_not_renamed", func(t *testing.T) {
		incoming2 := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com"), makeCluster("c2", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "admin"), makeContext("ctx2", "c2", "admin")},
		)
		got, _ := merger.MergeKubeconfig(nil, incoming2, "prod", "", "")
		if findEntry(got.Contexts, "prod") == nil {
			t.Error("first context not renamed to 'prod'")
		}
		if findEntry(got.Contexts, "ctx2") == nil {
			t.Error("second context 'ctx2' should remain unchanged")
		}
	})
}

// ---------------------------------------------------------------------------
// Rename cluster
// ---------------------------------------------------------------------------

func TestRenameCluster(t *testing.T) {
	incoming := makeKubeConfig(
		[]merger.NamedEntry{makeCluster("orig-c", "https://example.com")},
		[]merger.NamedEntry{makeUser("u1", "tok")},
		[]merger.NamedEntry{makeContext("ctx1", "orig-c", "u1")},
	)

	t.Run("first_cluster_renamed", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "prod-cluster", "")
		if findEntry(got.Clusters, "prod-cluster") == nil {
			t.Error("renamed cluster 'prod-cluster' not found")
		}
		if findEntry(got.Clusters, "orig-c") != nil {
			t.Error("original cluster name 'orig-c' should not exist")
		}
	})

	t.Run("cluster_reference_updated_in_context", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "prod-cluster", "")
		ctx := findEntry(got.Contexts, "ctx1")
		if got := contextField(ctx, "cluster"); got != "prod-cluster" {
			t.Errorf("context cluster ref = %q, want prod-cluster", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Rename user
// ---------------------------------------------------------------------------

func TestRenameUser(t *testing.T) {
	incoming := makeKubeConfig(
		[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
		[]merger.NamedEntry{makeUser("orig-u", "tok")},
		[]merger.NamedEntry{makeContext("ctx1", "c1", "orig-u")},
	)

	t.Run("first_user_renamed", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "", "admin")
		if findEntry(got.Users, "admin") == nil {
			t.Error("renamed user 'admin' not found")
		}
		if findEntry(got.Users, "orig-u") != nil {
			t.Error("original user name 'orig-u' should not exist")
		}
	})

	t.Run("user_reference_updated_in_context", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "", "admin")
		ctx := findEntry(got.Contexts, "ctx1")
		if got := contextField(ctx, "user"); got != "admin" {
			t.Errorf("context user ref = %q, want admin", got)
		}
	})

	t.Run("second_user_not_renamed", func(t *testing.T) {
		incoming2 := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok"), makeUser("u2", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1")},
		)
		got, _ := merger.MergeKubeconfig(nil, incoming2, "", "", "admin")
		if findEntry(got.Users, "admin") == nil {
			t.Error("first user not renamed to 'admin'")
		}
		if findEntry(got.Users, "u2") == nil {
			t.Error("second user 'u2' should remain unchanged")
		}
	})
}

// ---------------------------------------------------------------------------
// Rename all three
// ---------------------------------------------------------------------------

func TestRenameAll(t *testing.T) {
	incoming := makeKubeConfig(
		[]merger.NamedEntry{makeCluster("orig-c", "https://example.com")},
		[]merger.NamedEntry{makeUser("orig-u", "tok")},
		[]merger.NamedEntry{makeContext("orig-ctx", "orig-c", "orig-u")},
	)

	t.Run("all_renamed_and_references_updated", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "prod", "prod-c", "prod-u")
		if findEntry(got.Contexts, "prod") == nil {
			t.Error("context not renamed to 'prod'")
		}
		if findEntry(got.Clusters, "prod-c") == nil {
			t.Error("cluster not renamed to 'prod-c'")
		}
		if findEntry(got.Users, "prod-u") == nil {
			t.Error("user not renamed to 'prod-u'")
		}
		ctx := findEntry(got.Contexts, "prod")
		if got := contextField(ctx, "cluster"); got != "prod-c" {
			t.Errorf("context cluster ref = %q, want prod-c", got)
		}
		if got := contextField(ctx, "user"); got != "prod-u" {
			t.Errorf("context user ref = %q, want prod-u", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Conflicts
// ---------------------------------------------------------------------------

func TestConflicts(t *testing.T) {
	t.Run("existing_cluster_replaced", func(t *testing.T) {
		existing := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://old.example.com")},
			nil, nil,
		)
		incoming := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://new.example.com")},
			nil, nil,
		)
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		count := 0
		for _, c := range got.Clusters {
			if c.Name == "c1" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 cluster named c1, got %d", count)
		}
		// The replaced entry should be at the end (last appended).
		last := got.Clusters[len(got.Clusters)-1]
		clusterData, _ := last.Body["cluster"].(map[string]interface{})
		server, _ := clusterData["server"].(string)
		if server != "https://new.example.com" {
			t.Errorf("server = %q, want https://new.example.com", server)
		}
	})

	t.Run("conflict_recorded_in_result", func(t *testing.T) {
		existing := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			nil, nil,
		)
		incoming := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			nil, nil,
		)
		_, result := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if !contains(result.Clusters.Replaced, "c1") {
			t.Error("c1 should be in clusters.replaced")
		}
		if contains(result.Clusters.Added, "c1") {
			t.Error("c1 should not be in clusters.added")
		}
	})

	t.Run("new_entry_recorded_in_result", func(t *testing.T) {
		incoming := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("new-c", "https://example.com")},
			nil, nil,
		)
		_, result := merger.MergeKubeconfig(nil, incoming, "", "", "")
		if !contains(result.Clusters.Added, "new-c") {
			t.Error("new-c should be in clusters.added")
		}
	})
}

// ---------------------------------------------------------------------------
// BackupConfig
// ---------------------------------------------------------------------------

func TestBackupConfig(t *testing.T) {
	t.Run("backup_created", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		if err := os.WriteFile(configPath, []byte("original content"), 0600); err != nil {
			t.Fatal(err)
		}
		backupPath, err := merger.BackupConfig(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if backupPath == "" {
			t.Fatal("expected non-empty backup path")
		}
		data, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "original content" {
			t.Errorf("backup content = %q, want %q", data, "original content")
		}
	})

	t.Run("no_backup_if_file_missing", func(t *testing.T) {
		dir := t.TempDir()
		backupPath, err := merger.BackupConfig(filepath.Join(dir, "nonexistent"))
		if err != nil {
			t.Fatal(err)
		}
		if backupPath != "" {
			t.Errorf("expected empty backup path, got %q", backupPath)
		}
	})

	t.Run("backup_path_contains_timestamp", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		if err := os.WriteFile(configPath, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		backupPath, err := merger.BackupConfig(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(backupPath, ".backup.") {
			t.Errorf("backup path %q does not contain '.backup.'", backupPath)
		}
	})
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
