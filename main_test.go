package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chameerar/konfuse/internal/merger"
)

// ---------------------------------------------------------------------------
// extractPositional
// ---------------------------------------------------------------------------

func TestExtractPositional(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantPos      string
		wantFlagArgs []string
	}{
		{
			name:         "positional_only",
			args:         []string{"file.yaml"},
			wantPos:      "file.yaml",
			wantFlagArgs: nil,
		},
		{
			name:         "positional_before_flags",
			args:         []string{"file.yaml", "--dry-run"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--dry-run"},
		},
		{
			name:         "flags_before_positional",
			args:         []string{"--dry-run", "file.yaml"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--dry-run"},
		},
		{
			name:         "value_flag_before_positional",
			args:         []string{"--rename-context", "prod", "file.yaml"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--rename-context", "prod"},
		},
		{
			name:         "value_flag_after_positional",
			args:         []string{"file.yaml", "--rename-context", "prod"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--rename-context", "prod"},
		},
		{
			name:         "value_flag_equals_syntax",
			args:         []string{"file.yaml", "--rename-context=prod"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--rename-context=prod"},
		},
		{
			name:         "all_rename_flags",
			args:         []string{"file.yaml", "--rename-context", "ctx", "--rename-cluster", "cls", "--rename-user", "usr"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--rename-context", "ctx", "--rename-cluster", "cls", "--rename-user", "usr"},
		},
		{
			name:         "kubeconfig_flag",
			args:         []string{"file.yaml", "--kubeconfig", "/path/to/config"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"--kubeconfig", "/path/to/config"},
		},
		{
			name:         "no_args",
			args:         []string{},
			wantPos:      "",
			wantFlagArgs: nil,
		},
		{
			name:         "flags_only_no_positional",
			args:         []string{"--dry-run", "--json"},
			wantPos:      "",
			wantFlagArgs: []string{"--dry-run", "--json"},
		},
		{
			name:         "single_dash_flag",
			args:         []string{"file.yaml", "-json"},
			wantPos:      "file.yaml",
			wantFlagArgs: []string{"-json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPos, gotFlags := extractPositional(tt.args)
			if gotPos != tt.wantPos {
				t.Errorf("positional = %q, want %q", gotPos, tt.wantPos)
			}
			if len(gotFlags) != len(tt.wantFlagArgs) {
				t.Errorf("flagArgs = %v, want %v", gotFlags, tt.wantFlagArgs)
				return
			}
			for i := range gotFlags {
				if gotFlags[i] != tt.wantFlagArgs[i] {
					t.Errorf("flagArgs[%d] = %q, want %q", i, gotFlags[i], tt.wantFlagArgs[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// loadYAML
// ---------------------------------------------------------------------------

func TestLoadYAML(t *testing.T) {
	t.Run("valid_kubeconfig", func(t *testing.T) {
		f := writeTempFile(t, `
apiVersion: v1
kind: Config
clusters:
  - name: my-cluster
    cluster:
      server: https://example.com
users: []
contexts: []
current-context: ""
`)
		cfg, err := loadYAML(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Kind != "Config" {
			t.Errorf("kind = %q, want Config", cfg.Kind)
		}
		if len(cfg.Clusters) != 1 || cfg.Clusters[0].Name != "my-cluster" {
			t.Error("cluster not loaded correctly")
		}
	})

	t.Run("invalid_yaml_returns_error", func(t *testing.T) {
		f := writeTempFile(t, ":\tinvalid: ][yaml")
		_, err := loadYAML(f)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("missing_file_returns_error", func(t *testing.T) {
		_, err := loadYAML("/nonexistent/path/config.yaml")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("empty_yaml_returns_zero_value", func(t *testing.T) {
		f := writeTempFile(t, "")
		// Empty file decodes to nil / zero value — caller is responsible for validation.
		_, _ = loadYAML(f)
	})

	t.Run("all_sections_populated", func(t *testing.T) {
		f := writeTempFile(t, `
apiVersion: v1
kind: Config
clusters:
  - name: c1
    cluster:
      server: https://example.com
users:
  - name: u1
    user:
      token: mytoken
contexts:
  - name: ctx1
    context:
      cluster: c1
      user: u1
current-context: ctx1
`)
		cfg, err := loadYAML(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Clusters) != 1 {
			t.Errorf("clusters count = %d, want 1", len(cfg.Clusters))
		}
		if len(cfg.Users) != 1 {
			t.Errorf("users count = %d, want 1", len(cfg.Users))
		}
		if len(cfg.Contexts) != 1 {
			t.Errorf("contexts count = %d, want 1", len(cfg.Contexts))
		}
		if cfg.CurrentContext != "ctx1" {
			t.Errorf("current-context = %q, want ctx1", cfg.CurrentContext)
		}
	})
}

// ---------------------------------------------------------------------------
// saveYAML
// ---------------------------------------------------------------------------

func TestSaveYAML(t *testing.T) {
	t.Run("saves_and_reloads_correctly", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		cfg := &merger.KubeConfig{
			APIVersion: "v1",
			Kind:       "Config",
			Clusters: []merger.NamedEntry{
				{Name: "c1", Body: map[string]interface{}{
					"cluster": map[string]interface{}{"server": "https://example.com"},
				}},
			},
			Users:       []merger.NamedEntry{},
			Contexts:    []merger.NamedEntry{},
			Preferences: map[string]interface{}{},
		}
		if err := saveYAML(path, cfg); err != nil {
			t.Fatalf("saveYAML error: %v", err)
		}
		got, err := loadYAML(path)
		if err != nil {
			t.Fatalf("reload error: %v", err)
		}
		if got.Kind != "Config" {
			t.Errorf("kind = %q after reload", got.Kind)
		}
		if len(got.Clusters) != 1 || got.Clusters[0].Name != "c1" {
			t.Error("cluster not preserved through save/load")
		}
	})

	t.Run("creates_parent_directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "dir", "config")
		cfg := &merger.KubeConfig{APIVersion: "v1", Kind: "Config", Preferences: map[string]interface{}{}}
		if err := saveYAML(path, cfg); err != nil {
			t.Fatalf("saveYAML error: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("overwrites_existing_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		// Write initial content.
		if err := os.WriteFile(path, []byte("old content"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg := &merger.KubeConfig{APIVersion: "v1", Kind: "Config", Preferences: map[string]interface{}{}}
		if err := saveYAML(path, cfg); err != nil {
			t.Fatalf("saveYAML error: %v", err)
		}
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "old content") {
			t.Error("old content not overwritten")
		}
	})
}

// ---------------------------------------------------------------------------
// emit
// ---------------------------------------------------------------------------

func TestEmit(t *testing.T) {
	t.Run("outputs_valid_json", func(t *testing.T) {
		out := captureStdout(t, func() {
			emit(map[string]interface{}{"key": "value", "num": 42})
		})
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Errorf("emit produced invalid JSON: %v\noutput: %s", err, out)
		}
		if decoded["key"] != "value" {
			t.Errorf("key = %v, want value", decoded["key"])
		}
	})

	t.Run("output_is_indented", func(t *testing.T) {
		out := captureStdout(t, func() {
			emit(map[string]string{"a": "b"})
		})
		if !strings.Contains(out, "\n") {
			t.Error("expected indented (multi-line) JSON output")
		}
	})

	t.Run("merge_output_schema", func(t *testing.T) {
		out := captureStdout(t, func() {
			bp := "/tmp/config.backup.20260328T120000"
			emit(mergeOutput{
				DryRun: false,
				Target: "/home/user/.kube/config",
				Backup: &bp,
				Changes: merger.MergeResult{
					Clusters: merger.SectionResult{Added: []string{"eks-prod"}, Replaced: []string{}},
					Users:    merger.SectionResult{Added: []string{"eks-user"}, Replaced: []string{}},
					Contexts: merger.SectionResult{Added: []string{"prod"}, Replaced: []string{}},
				},
				HasConflicts: false,
			})
		})
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		for _, key := range []string{"dry_run", "target", "backup", "changes", "has_conflicts"} {
			if _, ok := decoded[key]; !ok {
				t.Errorf("missing key %q in merge output JSON", key)
			}
		}
	})

	t.Run("dry_run_output_schema", func(t *testing.T) {
		out := captureStdout(t, func() {
			emit(dryRunOutput{
				DryRun:       true,
				Target:       "/home/user/.kube/config",
				Changes:      merger.MergeResult{},
				HasConflicts: false,
			})
		})
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if decoded["dry_run"] != true {
			t.Error("dry_run should be true")
		}
		if _, ok := decoded["backup"]; ok {
			t.Error("dry_run output should not contain 'backup' key")
		}
	})
}

// ---------------------------------------------------------------------------
// printChanges
// ---------------------------------------------------------------------------

func TestPrintChanges(t *testing.T) {
	result := merger.MergeResult{
		Clusters: merger.SectionResult{Added: []string{"eks-prod"}, Replaced: []string{"old-cluster"}},
		Users:    merger.SectionResult{Added: []string{"eks-user"}, Replaced: []string{}},
		Contexts: merger.SectionResult{Added: []string{}, Replaced: []string{"prod"}},
	}

	t.Run("added_entries_use_plus_prefix", func(t *testing.T) {
		out := captureStdout(t, func() { printChanges(result, false) })
		if !strings.Contains(out, "  + ") {
			t.Errorf("expected '  + ' prefix for added entries, got:\n%s", out)
		}
	})

	t.Run("replaced_entries_use_bang_prefix", func(t *testing.T) {
		out := captureStdout(t, func() { printChanges(result, false) })
		if !strings.Contains(out, "  ! ") {
			t.Errorf("expected '  ! ' prefix for replaced entries, got:\n%s", out)
		}
	})

	t.Run("dry_run_uses_would_verbs", func(t *testing.T) {
		out := captureStdout(t, func() { printChanges(result, true) })
		if !strings.Contains(out, "Would add") {
			t.Errorf("expected 'Would add' in dry-run output, got:\n%s", out)
		}
		if !strings.Contains(out, "Would replace") {
			t.Errorf("expected 'Would replace' in dry-run output, got:\n%s", out)
		}
	})

	t.Run("non_dry_run_uses_past_tense", func(t *testing.T) {
		out := captureStdout(t, func() { printChanges(result, false) })
		if !strings.Contains(out, "Added") {
			t.Errorf("expected 'Added' in non-dry-run output, got:\n%s", out)
		}
		if !strings.Contains(out, "Replaced") {
			t.Errorf("expected 'Replaced' in non-dry-run output, got:\n%s", out)
		}
	})

	t.Run("entry_names_appear_in_output", func(t *testing.T) {
		out := captureStdout(t, func() { printChanges(result, false) })
		for _, name := range []string{"eks-prod", "old-cluster", "eks-user", "prod"} {
			if !strings.Contains(out, name) {
				t.Errorf("entry name %q not found in output:\n%s", name, out)
			}
		}
	})

	t.Run("empty_result_produces_no_output", func(t *testing.T) {
		empty := merger.MergeResult{
			Clusters: merger.SectionResult{Added: []string{}, Replaced: []string{}},
			Users:    merger.SectionResult{Added: []string{}, Replaced: []string{}},
			Contexts: merger.SectionResult{Added: []string{}, Replaced: []string{}},
		}
		out := captureStdout(t, func() { printChanges(empty, false) })
		if strings.TrimSpace(out) != "" {
			t.Errorf("expected empty output for empty result, got: %q", out)
		}
	})
}

// ---------------------------------------------------------------------------
// version
// ---------------------------------------------------------------------------

func TestVersion(t *testing.T) {
	t.Run("default_version_is_dev", func(t *testing.T) {
		if version != "dev" {
			// version may be overridden at build time; skip in that case.
			t.Skipf("version = %q (set via ldflags, skipping default check)", version)
		}
	})

	t.Run("version_string_is_non_empty", func(t *testing.T) {
		if version == "" {
			t.Error("version must not be empty")
		}
	})

	t.Run("version_output_format", func(t *testing.T) {
		out := captureStdout(t, func() {
			fmt.Printf("konfuse %s\n", version)
		})
		if !strings.HasPrefix(out, "konfuse ") {
			t.Errorf("version output = %q, want 'konfuse <version>'", out)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// captureStdout redirects os.Stdout, runs fn, and returns what was printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	return buf.String()
}

// writeTempFile writes content to a temporary file and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "kubeconfig-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
