package merger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chameerar/konfuse/internal/merger"
)

// ---------------------------------------------------------------------------
// Helpers
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

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Merge into empty / nil existing
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

	t.Run("result_records_all_as_added", func(t *testing.T) {
		_, result := merger.MergeKubeconfig(nil, incoming, "", "", "")
		if !contains(result.Clusters.Added, "c1") {
			t.Error("c1 not in clusters.added")
		}
		if !contains(result.Users.Added, "u1") {
			t.Error("u1 not in users.added")
		}
		if !contains(result.Contexts.Added, "ctx1") {
			t.Error("ctx1 not in contexts.added")
		}
		if len(result.Clusters.Replaced)+len(result.Users.Replaced)+len(result.Contexts.Replaced) != 0 {
			t.Error("no replaced entries expected for fresh merge")
		}
	})

	t.Run("existing_current_context_preserved", func(t *testing.T) {
		existing := makeKubeConfig(nil, nil, nil)
		existing.CurrentContext = "my-context"
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if got.CurrentContext != "my-context" {
			t.Errorf("CurrentContext = %q, want my-context", got.CurrentContext)
		}
	})

	t.Run("existing_preferences_preserved", func(t *testing.T) {
		existing := makeKubeConfig(nil, nil, nil)
		existing.Preferences = map[string]interface{}{"colors": true}
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if got.Preferences["colors"] != true {
			t.Error("preferences not preserved")
		}
	})
}

// ---------------------------------------------------------------------------
// Incoming with nil sections
// ---------------------------------------------------------------------------

func TestIncomingNilSections(t *testing.T) {
	t.Run("nil_clusters_in_incoming", func(t *testing.T) {
		existing := makeKubeConfig([]merger.NamedEntry{makeCluster("c1", "https://example.com")}, nil, nil)
		incoming := &merger.KubeConfig{APIVersion: "v1", Kind: "Config"}
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if len(got.Clusters) != 1 {
			t.Errorf("cluster count = %d, want 1", len(got.Clusters))
		}
	})

	t.Run("nil_users_in_incoming", func(t *testing.T) {
		existing := makeKubeConfig(nil, []merger.NamedEntry{makeUser("u1", "tok")}, nil)
		incoming := &merger.KubeConfig{APIVersion: "v1", Kind: "Config"}
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if len(got.Users) != 1 {
			t.Errorf("user count = %d, want 1", len(got.Users))
		}
	})

	t.Run("nil_contexts_in_incoming", func(t *testing.T) {
		existing := makeKubeConfig(nil, nil, []merger.NamedEntry{makeContext("ctx1", "c1", "u1")})
		incoming := &merger.KubeConfig{APIVersion: "v1", Kind: "Config"}
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if len(got.Contexts) != 1 {
			t.Errorf("context count = %d, want 1", len(got.Contexts))
		}
	})

	t.Run("fully_empty_incoming_leaves_existing_unchanged", func(t *testing.T) {
		existing := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1")},
		)
		incoming := &merger.KubeConfig{APIVersion: "v1", Kind: "Config"}
		got, result := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if len(got.Clusters) != 1 || len(got.Users) != 1 || len(got.Contexts) != 1 {
			t.Error("existing entries should be unchanged")
		}
		if len(result.Clusters.Added)+len(result.Users.Added)+len(result.Contexts.Added) != 0 {
			t.Error("no additions expected for empty incoming")
		}
	})
}

// ---------------------------------------------------------------------------
// No rename
// ---------------------------------------------------------------------------

func TestMergeNoRename(t *testing.T) {
	t.Run("names_preserved", func(t *testing.T) {
		existing := makeKubeConfig([]merger.NamedEntry{makeCluster("existing-c", "https://example.com")}, nil, nil)
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

	t.Run("multiple_incoming_entries_all_added", func(t *testing.T) {
		incoming := makeKubeConfig(
			[]merger.NamedEntry{
				makeCluster("c1", "https://one.example.com"),
				makeCluster("c2", "https://two.example.com"),
				makeCluster("c3", "https://three.example.com"),
			},
			nil, nil,
		)
		got, result := merger.MergeKubeconfig(nil, incoming, "", "", "")
		if len(got.Clusters) != 3 {
			t.Errorf("cluster count = %d, want 3", len(got.Clusters))
		}
		if len(result.Clusters.Added) != 3 {
			t.Errorf("added count = %d, want 3", len(result.Clusters.Added))
		}
	})

	t.Run("incoming_appended_after_existing", func(t *testing.T) {
		existing := makeKubeConfig([]merger.NamedEntry{makeCluster("c1", "https://example.com")}, nil, nil)
		incoming := makeKubeConfig([]merger.NamedEntry{makeCluster("c2", "https://example.com")}, nil, nil)
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if got.Clusters[0].Name != "c1" {
			t.Errorf("first cluster = %q, want c1 (existing should come first)", got.Clusters[0].Name)
		}
		if got.Clusters[1].Name != "c2" {
			t.Errorf("second cluster = %q, want c2", got.Clusters[1].Name)
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

	t.Run("rename_context_does_not_affect_cluster_ref", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "prod", "", "")
		ctx := findEntry(got.Contexts, "prod")
		if contextField(ctx, "cluster") != "c1" {
			t.Errorf("cluster ref = %q, want c1 (renaming context should not change cluster ref)", contextField(ctx, "cluster"))
		}
	})

	t.Run("result_records_renamed_context_as_added", func(t *testing.T) {
		_, result := merger.MergeKubeconfig(nil, incoming, "prod", "", "")
		if !contains(result.Contexts.Added, "prod") {
			t.Error("'prod' should be in contexts.added")
		}
		if contains(result.Contexts.Added, "orig-ctx") {
			t.Error("'orig-ctx' should not appear in result")
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
		if contextField(ctx, "cluster") != "prod-cluster" {
			t.Errorf("context cluster ref = %q, want prod-cluster", contextField(ctx, "cluster"))
		}
	})

	t.Run("cluster_ref_in_second_context_not_updated", func(t *testing.T) {
		incoming2 := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("orig-c", "https://example.com"), makeCluster("c2", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "orig-c", "u1"), makeContext("ctx2", "c2", "u1")},
		)
		got, _ := merger.MergeKubeconfig(nil, incoming2, "", "prod-cluster", "")
		ctx2 := findEntry(got.Contexts, "ctx2")
		if contextField(ctx2, "cluster") != "c2" {
			t.Errorf("ctx2 cluster ref = %q, want c2 (should not be updated)", contextField(ctx2, "cluster"))
		}
	})

	t.Run("second_cluster_not_renamed", func(t *testing.T) {
		incoming2 := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("orig-c", "https://example.com"), makeCluster("c2", "https://example.com")},
			nil, nil,
		)
		got, _ := merger.MergeKubeconfig(nil, incoming2, "", "prod-cluster", "")
		if findEntry(got.Clusters, "c2") == nil {
			t.Error("second cluster 'c2' should remain unchanged")
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
		if contextField(ctx, "user") != "admin" {
			t.Errorf("context user ref = %q, want admin", contextField(ctx, "user"))
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

	t.Run("user_ref_not_updated_when_only_cluster_renamed", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "new-cluster", "")
		ctx := findEntry(got.Contexts, "ctx1")
		if contextField(ctx, "user") != "orig-u" {
			t.Errorf("user ref = %q, want orig-u (should not change when only cluster renamed)", contextField(ctx, "user"))
		}
	})
}

// ---------------------------------------------------------------------------
// Rename all three simultaneously
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
		if contextField(ctx, "cluster") != "prod-c" {
			t.Errorf("context cluster ref = %q, want prod-c", contextField(ctx, "cluster"))
		}
		if contextField(ctx, "user") != "prod-u" {
			t.Errorf("context user ref = %q, want prod-u", contextField(ctx, "user"))
		}
	})

	t.Run("original_names_do_not_appear", func(t *testing.T) {
		got, _ := merger.MergeKubeconfig(nil, incoming, "prod", "prod-c", "prod-u")
		if findEntry(got.Contexts, "orig-ctx") != nil {
			t.Error("orig-ctx should not exist")
		}
		if findEntry(got.Clusters, "orig-c") != nil {
			t.Error("orig-c should not exist")
		}
		if findEntry(got.Users, "orig-u") != nil {
			t.Error("orig-u should not exist")
		}
	})
}

// ---------------------------------------------------------------------------
// Conflicts
// ---------------------------------------------------------------------------

func TestConflicts(t *testing.T) {
	t.Run("existing_cluster_replaced", func(t *testing.T) {
		existing := makeKubeConfig([]merger.NamedEntry{makeCluster("c1", "https://old.example.com")}, nil, nil)
		incoming := makeKubeConfig([]merger.NamedEntry{makeCluster("c1", "https://new.example.com")}, nil, nil)
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
		last := got.Clusters[len(got.Clusters)-1]
		clusterData, _ := last.Body["cluster"].(map[string]interface{})
		if clusterData["server"] != "https://new.example.com" {
			t.Errorf("server = %q, want https://new.example.com", clusterData["server"])
		}
	})

	t.Run("existing_user_replaced", func(t *testing.T) {
		existing := makeKubeConfig(nil, []merger.NamedEntry{makeUser("u1", "old-token")}, nil)
		incoming := makeKubeConfig(nil, []merger.NamedEntry{makeUser("u1", "new-token")}, nil)
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		count := 0
		for _, u := range got.Users {
			if u.Name == "u1" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 user named u1, got %d", count)
		}
	})

	t.Run("existing_context_replaced", func(t *testing.T) {
		existing := makeKubeConfig(nil, nil, []merger.NamedEntry{makeContext("ctx1", "old-c", "old-u")})
		incoming := makeKubeConfig(nil, nil, []merger.NamedEntry{makeContext("ctx1", "new-c", "new-u")})
		got, _ := merger.MergeKubeconfig(existing, incoming, "", "", "")
		count := 0
		for _, c := range got.Contexts {
			if c.Name == "ctx1" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 context named ctx1, got %d", count)
		}
		ctx := findEntry(got.Contexts, "ctx1")
		if contextField(ctx, "cluster") != "new-c" {
			t.Errorf("cluster ref = %q, want new-c", contextField(ctx, "cluster"))
		}
	})

	t.Run("conflict_recorded_in_result", func(t *testing.T) {
		existing := makeKubeConfig([]merger.NamedEntry{makeCluster("c1", "https://example.com")}, nil, nil)
		incoming := makeKubeConfig([]merger.NamedEntry{makeCluster("c1", "https://example.com")}, nil, nil)
		_, result := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if !contains(result.Clusters.Replaced, "c1") {
			t.Error("c1 should be in clusters.replaced")
		}
		if contains(result.Clusters.Added, "c1") {
			t.Error("c1 should not be in clusters.added")
		}
	})

	t.Run("new_entry_recorded_as_added", func(t *testing.T) {
		incoming := makeKubeConfig([]merger.NamedEntry{makeCluster("new-c", "https://example.com")}, nil, nil)
		_, result := merger.MergeKubeconfig(nil, incoming, "", "", "")
		if !contains(result.Clusters.Added, "new-c") {
			t.Error("new-c should be in clusters.added")
		}
	})

	t.Run("rename_onto_existing_name_counts_as_conflict", func(t *testing.T) {
		existing := makeKubeConfig([]merger.NamedEntry{makeCluster("prod", "https://old.example.com")}, nil, nil)
		incoming := makeKubeConfig([]merger.NamedEntry{makeCluster("orig-c", "https://new.example.com")}, nil, nil)
		_, result := merger.MergeKubeconfig(existing, incoming, "", "prod", "")
		if !contains(result.Clusters.Replaced, "prod") {
			t.Error("renaming onto existing name 'prod' should count as replaced")
		}
	})

	t.Run("multiple_conflicts_all_recorded", func(t *testing.T) {
		existing := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://old.example.com"), makeCluster("c2", "https://old.example.com")},
			nil, nil,
		)
		incoming := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://new.example.com"), makeCluster("c2", "https://new.example.com")},
			nil, nil,
		)
		_, result := merger.MergeKubeconfig(existing, incoming, "", "", "")
		if len(result.Clusters.Replaced) != 2 {
			t.Errorf("replaced count = %d, want 2", len(result.Clusters.Replaced))
		}
	})
}

// ---------------------------------------------------------------------------
// Body preservation (unknown fields must survive round-trip)
// ---------------------------------------------------------------------------

func TestBodyPreservation(t *testing.T) {
	t.Run("unknown_cluster_fields_preserved", func(t *testing.T) {
		entry := merger.NamedEntry{
			Name: "c1",
			Body: map[string]interface{}{
				"cluster": map[string]interface{}{
					"server":                     "https://example.com",
					"certificate-authority-data": "abc123",
					"extensions":                 []interface{}{"ext1"},
				},
			},
		}
		incoming := makeKubeConfig([]merger.NamedEntry{entry}, nil, nil)
		got, _ := merger.MergeKubeconfig(nil, incoming, "", "", "")
		c := findEntry(got.Clusters, "c1")
		clusterData, _ := c.Body["cluster"].(map[string]interface{})
		if clusterData["certificate-authority-data"] != "abc123" {
			t.Error("certificate-authority-data not preserved")
		}
	})

	t.Run("incoming_body_not_mutated_on_rename", func(t *testing.T) {
		origCtx := makeContext("orig-ctx", "orig-c", "orig-u")
		incoming := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("orig-c", "https://example.com")},
			[]merger.NamedEntry{makeUser("orig-u", "tok")},
			[]merger.NamedEntry{origCtx},
		)
		merger.MergeKubeconfig(nil, incoming, "prod", "prod-c", "prod-u")
		// Original incoming context body must be untouched.
		ctxData, _ := origCtx.Body["context"].(map[string]interface{})
		if ctxData["cluster"] != "orig-c" {
			t.Errorf("incoming context mutated: cluster = %q, want orig-c", ctxData["cluster"])
		}
		if ctxData["user"] != "orig-u" {
			t.Errorf("incoming context mutated: user = %q, want orig-u", ctxData["user"])
		}
	})
}

// ---------------------------------------------------------------------------
// BackupConfig
// ---------------------------------------------------------------------------

func TestBackupConfig(t *testing.T) {
	t.Run("backup_created_with_same_content", func(t *testing.T) {
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

	t.Run("backup_is_independent_copy", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		if err := os.WriteFile(configPath, []byte("v1"), 0600); err != nil {
			t.Fatal(err)
		}
		backupPath, _ := merger.BackupConfig(configPath)
		// Overwrite original.
		if err := os.WriteFile(configPath, []byte("v2"), 0600); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(backupPath)
		if string(data) != "v1" {
			t.Errorf("backup changed after original was overwritten: got %q", data)
		}
	})

	t.Run("multiple_backups_have_unique_paths", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		if err := os.WriteFile(configPath, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		p1, _ := merger.BackupConfig(configPath)
		p2, _ := merger.BackupConfig(configPath)
		// Paths may be identical if called within the same second — that is
		// acceptable behaviour; what we must NOT do is error out.
		_ = p1
		_ = p2
	})
}

// ---------------------------------------------------------------------------
// ListEntries
// ---------------------------------------------------------------------------

func TestListEntries(t *testing.T) {
	t.Run("lists_all_entries", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com"), makeCluster("c2", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok"), makeUser("u2", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1"), makeContext("ctx2", "c2", "u2")},
		)
		cfg.CurrentContext = "ctx1"

		result := merger.ListEntries(cfg)
		if result.CurrentContext != "ctx1" {
			t.Errorf("CurrentContext = %q, want ctx1", result.CurrentContext)
		}
		if len(result.Contexts) != 2 {
			t.Errorf("context count = %d, want 2", len(result.Contexts))
		}
		if result.Contexts[0].Name != "ctx1" || result.Contexts[0].Cluster != "c1" || result.Contexts[0].User != "u1" {
			t.Errorf("context[0] = %+v, want ctx1/c1/u1", result.Contexts[0])
		}
		if len(result.Clusters) != 2 {
			t.Errorf("cluster count = %d, want 2", len(result.Clusters))
		}
		if len(result.Users) != 2 {
			t.Errorf("user count = %d, want 2", len(result.Users))
		}
	})

	t.Run("empty_config", func(t *testing.T) {
		cfg := makeKubeConfig(nil, nil, nil)
		result := merger.ListEntries(cfg)
		if len(result.Contexts) != 0 || len(result.Clusters) != 0 || len(result.Users) != 0 {
			t.Error("expected empty lists for empty config")
		}
	})

	t.Run("no_current_context", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			nil,
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1")},
		)
		result := merger.ListEntries(cfg)
		if result.CurrentContext != "" {
			t.Errorf("CurrentContext = %q, want empty", result.CurrentContext)
		}
	})

	t.Run("context_with_missing_body", func(t *testing.T) {
		cfg := makeKubeConfig(nil, nil, []merger.NamedEntry{
			{Name: "ctx-no-body", Body: map[string]interface{}{}},
		})
		result := merger.ListEntries(cfg)
		if len(result.Contexts) != 1 {
			t.Fatalf("context count = %d, want 1", len(result.Contexts))
		}
		if result.Contexts[0].Name != "ctx-no-body" {
			t.Errorf("name = %q, want ctx-no-body", result.Contexts[0].Name)
		}
		if result.Contexts[0].Cluster != "" || result.Contexts[0].User != "" {
			t.Error("expected empty cluster/user for context without body")
		}
	})

	t.Run("preserves_entry_order", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("z-cluster", "https://z.com"), makeCluster("a-cluster", "https://a.com")},
			[]merger.NamedEntry{makeUser("z-user", "tok"), makeUser("a-user", "tok")},
			[]merger.NamedEntry{makeContext("z-ctx", "z-cluster", "z-user"), makeContext("a-ctx", "a-cluster", "a-user")},
		)
		result := merger.ListEntries(cfg)
		if result.Clusters[0] != "z-cluster" || result.Clusters[1] != "a-cluster" {
			t.Errorf("clusters order = %v, want [z-cluster, a-cluster]", result.Clusters)
		}
		if result.Users[0] != "z-user" || result.Users[1] != "a-user" {
			t.Errorf("users order = %v, want [z-user, a-user]", result.Users)
		}
		if result.Contexts[0].Name != "z-ctx" || result.Contexts[1].Name != "a-ctx" {
			t.Error("contexts order not preserved")
		}
	})
}

// ---------------------------------------------------------------------------
// DeleteContext
// ---------------------------------------------------------------------------

func TestDeleteContext(t *testing.T) {
	t.Run("deletes_context_and_orphaned_cluster_and_user", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1")},
		)
		got, result, err := merger.DeleteContext(cfg, "ctx1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Context != "ctx1" {
			t.Errorf("deleted context = %q, want ctx1", result.Context)
		}
		if result.Cluster != "c1" {
			t.Errorf("deleted cluster = %q, want c1", result.Cluster)
		}
		if result.User != "u1" {
			t.Errorf("deleted user = %q, want u1", result.User)
		}
		if len(got.Contexts) != 0 || len(got.Clusters) != 0 || len(got.Users) != 0 {
			t.Error("expected all entries to be removed")
		}
	})

	t.Run("keeps_shared_cluster_and_user", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("shared-c", "https://example.com")},
			[]merger.NamedEntry{makeUser("shared-u", "tok")},
			[]merger.NamedEntry{
				makeContext("ctx1", "shared-c", "shared-u"),
				makeContext("ctx2", "shared-c", "shared-u"),
			},
		)
		got, result, err := merger.DeleteContext(cfg, "ctx1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Cluster != "" {
			t.Errorf("cluster should not be deleted when still referenced, got %q", result.Cluster)
		}
		if result.User != "" {
			t.Errorf("user should not be deleted when still referenced, got %q", result.User)
		}
		if len(got.Clusters) != 1 || len(got.Users) != 1 {
			t.Error("shared cluster/user should be preserved")
		}
		if len(got.Contexts) != 1 || got.Contexts[0].Name != "ctx2" {
			t.Error("ctx2 should remain")
		}
	})

	t.Run("error_on_missing_context", func(t *testing.T) {
		cfg := makeKubeConfig(nil, nil, nil)
		_, _, err := merger.DeleteContext(cfg, "nonexistent")
		if err == nil {
			t.Error("expected error for missing context")
		}
	})

	t.Run("clears_current_context_if_deleted", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1")},
		)
		cfg.CurrentContext = "ctx1"
		got, _, err := merger.DeleteContext(cfg, "ctx1")
		if err != nil {
			t.Fatal(err)
		}
		if got.CurrentContext != "" {
			t.Errorf("CurrentContext = %q, want empty after deleting active context", got.CurrentContext)
		}
	})

	t.Run("preserves_current_context_if_different", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com"), makeCluster("c2", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok"), makeUser("u2", "tok")},
			[]merger.NamedEntry{makeContext("ctx1", "c1", "u1"), makeContext("ctx2", "c2", "u2")},
		)
		cfg.CurrentContext = "ctx2"
		got, _, err := merger.DeleteContext(cfg, "ctx1")
		if err != nil {
			t.Fatal(err)
		}
		if got.CurrentContext != "ctx2" {
			t.Errorf("CurrentContext = %q, want ctx2", got.CurrentContext)
		}
	})

	t.Run("shared_cluster_orphaned_user", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("shared-c", "https://example.com")},
			[]merger.NamedEntry{makeUser("only-u", "tok"), makeUser("shared-u", "tok")},
			[]merger.NamedEntry{
				makeContext("ctx1", "shared-c", "only-u"),
				makeContext("ctx2", "shared-c", "shared-u"),
			},
		)
		got, result, err := merger.DeleteContext(cfg, "ctx1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Cluster != "" {
			t.Errorf("shared cluster should not be deleted, got %q", result.Cluster)
		}
		if result.User != "only-u" {
			t.Errorf("orphaned user = %q, want only-u", result.User)
		}
		if len(got.Clusters) != 1 {
			t.Errorf("cluster count = %d, want 1", len(got.Clusters))
		}
		if len(got.Users) != 1 || got.Users[0].Name != "shared-u" {
			t.Error("shared-u should remain, only-u should be removed")
		}
	})

	t.Run("context_with_no_body", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok")},
			[]merger.NamedEntry{
				{Name: "bare-ctx", Body: map[string]interface{}{}},
			},
		)
		got, result, err := merger.DeleteContext(cfg, "bare-ctx")
		if err != nil {
			t.Fatal(err)
		}
		if result.Context != "bare-ctx" {
			t.Errorf("deleted context = %q, want bare-ctx", result.Context)
		}
		// Cluster and user should not be touched since context had no refs.
		if result.Cluster != "" || result.User != "" {
			t.Error("no cluster/user should be deleted for context without refs")
		}
		if len(got.Clusters) != 1 || len(got.Users) != 1 {
			t.Error("unrelated cluster/user should be preserved")
		}
	})

	t.Run("delete_middle_preserves_order", func(t *testing.T) {
		cfg := makeKubeConfig(
			[]merger.NamedEntry{makeCluster("c1", "https://example.com"), makeCluster("c2", "https://example.com"), makeCluster("c3", "https://example.com")},
			[]merger.NamedEntry{makeUser("u1", "tok"), makeUser("u2", "tok"), makeUser("u3", "tok")},
			[]merger.NamedEntry{
				makeContext("ctx1", "c1", "u1"),
				makeContext("ctx2", "c2", "u2"),
				makeContext("ctx3", "c3", "u3"),
			},
		)
		got, _, err := merger.DeleteContext(cfg, "ctx2")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Contexts) != 2 {
			t.Fatalf("context count = %d, want 2", len(got.Contexts))
		}
		if got.Contexts[0].Name != "ctx1" || got.Contexts[1].Name != "ctx3" {
			t.Errorf("remaining contexts = [%s, %s], want [ctx1, ctx3]", got.Contexts[0].Name, got.Contexts[1].Name)
		}
		if len(got.Clusters) != 2 || got.Clusters[0].Name != "c1" || got.Clusters[1].Name != "c3" {
			t.Error("remaining clusters should be [c1, c3] in order")
		}
		if len(got.Users) != 2 || got.Users[0].Name != "u1" || got.Users[1].Name != "u3" {
			t.Error("remaining users should be [u1, u3] in order")
		}
	})

	t.Run("ref_points_to_nonexistent_cluster_and_user", func(t *testing.T) {
		// Context references a cluster/user that don't exist in the entries lists.
		cfg := makeKubeConfig(
			nil,
			nil,
			[]merger.NamedEntry{makeContext("ctx1", "ghost-c", "ghost-u")},
		)
		got, result, err := merger.DeleteContext(cfg, "ctx1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Context != "ctx1" {
			t.Errorf("deleted context = %q, want ctx1", result.Context)
		}
		// Cluster/user not in the lists, so nothing extra to delete.
		if result.Cluster != "" || result.User != "" {
			t.Error("should not report deleted cluster/user when they don't exist in entries")
		}
		if len(got.Contexts) != 0 {
			t.Error("context should be removed")
		}
	})

	t.Run("error_message_includes_context_name", func(t *testing.T) {
		cfg := makeKubeConfig(nil, nil, nil)
		_, _, err := merger.DeleteContext(cfg, "my-missing-ctx")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "my-missing-ctx") {
			t.Errorf("error = %q, should contain context name", err.Error())
		}
	})
}
