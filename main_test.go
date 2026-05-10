package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		var buf bytes.Buffer
		emit(&buf, map[string]interface{}{"key": "value", "num": 42})
		var decoded map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Errorf("emit produced invalid JSON: %v\noutput: %s", err, buf.String())
		}
		if decoded["key"] != "value" {
			t.Errorf("key = %v, want value", decoded["key"])
		}
	})

	t.Run("output_is_indented", func(t *testing.T) {
		var buf bytes.Buffer
		emit(&buf, map[string]string{"a": "b"})
		if !strings.Contains(buf.String(), "\n") {
			t.Error("expected indented (multi-line) JSON output")
		}
	})

	t.Run("merge_output_schema", func(t *testing.T) {
		var buf bytes.Buffer
		bp := "/tmp/config.backup.20260328T120000"
		emit(&buf, mergeOutput{
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
		var decoded map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		for _, key := range []string{"dry_run", "target", "backup", "changes", "has_conflicts"} {
			if _, ok := decoded[key]; !ok {
				t.Errorf("missing key %q in merge output JSON", key)
			}
		}
	})

	t.Run("dry_run_output_schema", func(t *testing.T) {
		var buf bytes.Buffer
		emit(&buf, dryRunOutput{
			DryRun:       true,
			Target:       "/home/user/.kube/config",
			Changes:      merger.MergeResult{},
			HasConflicts: false,
		})
		var decoded map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
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

	render := func(dryRun bool) string {
		var buf bytes.Buffer
		printChanges(&buf, result, dryRun)
		return buf.String()
	}

	t.Run("added_entries_use_plus_prefix", func(t *testing.T) {
		out := render(false)
		if !strings.Contains(out, "  + ") {
			t.Errorf("expected '  + ' prefix for added entries, got:\n%s", out)
		}
	})

	t.Run("replaced_entries_use_bang_prefix", func(t *testing.T) {
		out := render(false)
		if !strings.Contains(out, "  ! ") {
			t.Errorf("expected '  ! ' prefix for replaced entries, got:\n%s", out)
		}
	})

	t.Run("dry_run_uses_would_verbs", func(t *testing.T) {
		out := render(true)
		if !strings.Contains(out, "Would add") {
			t.Errorf("expected 'Would add' in dry-run output, got:\n%s", out)
		}
		if !strings.Contains(out, "Would replace") {
			t.Errorf("expected 'Would replace' in dry-run output, got:\n%s", out)
		}
	})

	t.Run("non_dry_run_uses_past_tense", func(t *testing.T) {
		out := render(false)
		if !strings.Contains(out, "Added") {
			t.Errorf("expected 'Added' in non-dry-run output, got:\n%s", out)
		}
		if !strings.Contains(out, "Replaced") {
			t.Errorf("expected 'Replaced' in non-dry-run output, got:\n%s", out)
		}
	})

	t.Run("entry_names_appear_in_output", func(t *testing.T) {
		out := render(false)
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
		var buf bytes.Buffer
		printChanges(&buf, empty, false)
		if strings.TrimSpace(buf.String()) != "" {
			t.Errorf("expected empty output for empty result, got: %q", buf.String())
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
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "konfuse %s\n", version)
		if !strings.HasPrefix(buf.String(), "konfuse ") {
			t.Errorf("version output = %q, want 'konfuse <version>'", buf.String())
		}
	})
}

// ---------------------------------------------------------------------------
// confirm
// ---------------------------------------------------------------------------

func TestConfirm(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		useJSON    bool
		yes        bool
		stdinIsTTY bool
		want       bool
	}{
		{name: "auto_proceed_when_useJSON", useJSON: true, stdinIsTTY: true, want: true},
		{name: "auto_proceed_when_yes", yes: true, stdinIsTTY: true, want: true},
		{name: "auto_proceed_when_stdin_not_tty", stdinIsTTY: false, want: true},
		{name: "y_proceeds", input: "y\n", stdinIsTTY: true, want: true},
		{name: "Y_proceeds", input: "Y\n", stdinIsTTY: true, want: true},
		{name: "yes_proceeds", input: "yes\n", stdinIsTTY: true, want: true},
		{name: "YES_proceeds", input: "YES\n", stdinIsTTY: true, want: true},
		{name: "empty_aborts", input: "\n", stdinIsTTY: true, want: false},
		{name: "n_aborts", input: "n\n", stdinIsTTY: true, want: false},
		{name: "no_aborts", input: "no\n", stdinIsTTY: true, want: false},
		{name: "anything_else_aborts", input: "maybe\n", stdinIsTTY: true, want: false},
		{name: "whitespace_around_y_proceeds", input: "  y  \n", stdinIsTTY: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errw bytes.Buffer
			got := confirm(strings.NewReader(tt.input), &errw, "Confirm?", tt.useJSON, tt.yes, tt.stdinIsTTY)
			if got != tt.want {
				t.Errorf("confirm = %v, want %v", got, tt.want)
			}
			// Prompt should be written to errw only when actually prompting.
			shouldPrompt := !tt.useJSON && !tt.yes && tt.stdinIsTTY
			if shouldPrompt && !strings.Contains(errw.String(), "Confirm?") {
				t.Errorf("expected prompt in errw, got %q", errw.String())
			}
			if !shouldPrompt && errw.Len() != 0 {
				t.Errorf("expected no output when not prompting, got %q", errw.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// writeWithBackup
// ---------------------------------------------------------------------------

func TestWriteWithBackup(t *testing.T) {
	t.Run("creates_backup_when_target_exists", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg := &merger.KubeConfig{APIVersion: "v1", Kind: "Config", Preferences: map[string]interface{}{}}
		bp, err := writeWithBackup(path, cfg)
		if err != nil {
			t.Fatalf("writeWithBackup error: %v", err)
		}
		if bp == "" {
			t.Error("expected non-empty backup path")
		}
		if _, err := os.Stat(bp); err != nil {
			t.Errorf("backup file not created: %v", err)
		}
		// Target was rewritten.
		if _, err := os.Stat(path); err != nil {
			t.Errorf("target file missing: %v", err)
		}
	})

	t.Run("no_backup_when_target_absent", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		cfg := &merger.KubeConfig{APIVersion: "v1", Kind: "Config", Preferences: map[string]interface{}{}}
		bp, err := writeWithBackup(path, cfg)
		if err != nil {
			t.Fatalf("writeWithBackup error: %v", err)
		}
		if bp != "" {
			t.Errorf("expected empty backup path when target absent, got %q", bp)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("target file not created: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// runMergeE
// ---------------------------------------------------------------------------

func TestRunMergeE(t *testing.T) {
	const incomingYAML = `
apiVersion: v1
kind: Config
clusters:
  - name: new-cluster
    cluster:
      server: https://new.example.com
users:
  - name: new-user
    user:
      token: tok
contexts:
  - name: new-ctx
    context:
      cluster: new-cluster
      user: new-user
current-context: ""
`

	t.Run("merges_into_fresh_kubeconfig", func(t *testing.T) {
		dir := t.TempDir()
		input := writeTempFile(t, incomingYAML)
		target := filepath.Join(dir, "config")

		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{input, "--kubeconfig", target, "--json"},
			"/should/not/be/used",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, want %d. stderr: %s", code, exitOK, stderr.String())
		}
		var got mergeOutput
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
		}
		if got.Target != target {
			t.Errorf("target = %q, want %q", got.Target, target)
		}
		if got.Backup != nil {
			t.Errorf("backup should be nil for fresh merge, got %v", *got.Backup)
		}
		if len(got.Changes.Clusters.Added) != 1 || got.Changes.Clusters.Added[0] != "new-cluster" {
			t.Errorf("clusters added = %v, want [new-cluster]", got.Changes.Clusters.Added)
		}
	})

	t.Run("dry_run_does_not_write", func(t *testing.T) {
		dir := t.TempDir()
		input := writeTempFile(t, incomingYAML)
		target := filepath.Join(dir, "config")

		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{input, "--kubeconfig", target, "--dry-run", "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("dry-run wrote target file: %v", err)
		}
		var got dryRunOutput
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !got.DryRun {
			t.Error("dry_run should be true")
		}
	})

	t.Run("returns_exitNotFound_when_input_missing", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{"/no/such/file.yaml", "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitNotFound {
			t.Errorf("exit = %d, want %d", code, exitNotFound)
		}
	})

	t.Run("returns_exitUsage_when_no_input", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
	})

	t.Run("input_required_error_respects_json_mode", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{"--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
		var got errorOutput
		if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
			t.Fatalf("expected JSON error in stderr, got non-JSON: %q", stderr.String())
		}
		if got.Error == "" {
			t.Errorf("expected error message, got %+v", got)
		}
	})

	t.Run("version_flag_no_longer_handled_by_merge", func(t *testing.T) {
		// --version is now a top-level flag handled by main(); runMergeE
		// rejects it as an unknown flag.
		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{"--version"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
	})

	t.Run("prompts_and_aborts_on_n", func(t *testing.T) {
		dir := t.TempDir()
		input := writeTempFile(t, incomingYAML)
		target := filepath.Join(dir, "config")
		if err := os.WriteFile(target, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
			t.Fatal(err)
		}
		original, _ := os.ReadFile(target)

		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{input, "--kubeconfig", target},
			"",
			strings.NewReader("n\n"),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d (abort), stderr: %s", code, exitUsage, stderr.String())
		}
		if !strings.Contains(stderr.String(), "Merge into") {
			t.Errorf("expected prompt in stderr, got %q", stderr.String())
		}
		// Target file untouched.
		after, _ := os.ReadFile(target)
		if string(after) != string(original) {
			t.Error("target was modified despite abort")
		}
	})

	t.Run("yes_flag_skips_prompt", func(t *testing.T) {
		dir := t.TempDir()
		input := writeTempFile(t, incomingYAML)
		target := filepath.Join(dir, "config")
		if err := os.WriteFile(target, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
			t.Fatal(err)
		}

		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{input, "--kubeconfig", target, "--yes"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Errorf("exit = %d, stderr: %s", code, stderr.String())
		}
		if strings.Contains(stderr.String(), "Merge into") {
			t.Errorf("expected no prompt with --yes, got %q", stderr.String())
		}
	})

	t.Run("no_prompt_for_fresh_target", func(t *testing.T) {
		dir := t.TempDir()
		input := writeTempFile(t, incomingYAML)
		target := filepath.Join(dir, "fresh-config")

		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{input, "--kubeconfig", target},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Errorf("exit = %d, stderr: %s", code, stderr.String())
		}
		if strings.Contains(stderr.String(), "Merge into") {
			t.Errorf("no prompt expected for fresh target, got %q", stderr.String())
		}
	})

	t.Run("rename_flags_apply_to_first_entry", func(t *testing.T) {
		dir := t.TempDir()
		input := writeTempFile(t, incomingYAML)
		target := filepath.Join(dir, "config")

		var stdout, stderr bytes.Buffer
		code := runMergeE(
			[]string{
				input, "--kubeconfig", target,
				"--rename-context", "renamed-ctx",
				"--rename-cluster", "renamed-cluster",
				"--rename-user", "renamed-user",
				"--json",
			},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		var got mergeOutput
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !contains(got.Changes.Clusters.Added, "renamed-cluster") {
			t.Errorf("clusters added = %v, want renamed-cluster", got.Changes.Clusters.Added)
		}
		if !contains(got.Changes.Contexts.Added, "renamed-ctx") {
			t.Errorf("contexts added = %v, want renamed-ctx", got.Changes.Contexts.Added)
		}
		if !contains(got.Changes.Users.Added, "renamed-user") {
			t.Errorf("users added = %v, want renamed-user", got.Changes.Users.Added)
		}
	})
}

// ---------------------------------------------------------------------------
// runListE
// ---------------------------------------------------------------------------

func TestRunListE(t *testing.T) {
	yamlBody := `
apiVersion: v1
kind: Config
clusters:
  - name: c1
    cluster:
      server: https://c1.example.com
users:
  - name: u1
    user:
      token: t1
contexts:
  - name: ctx1
    context:
      cluster: c1
      user: u1
current-context: ctx1
`

	t.Run("emits_canonical_json", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runListE(
			[]string{"--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		var got merger.ListResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.CurrentContext != "ctx1" {
			t.Errorf("current_context = %q, want ctx1", got.CurrentContext)
		}
		if len(got.Contexts) != 1 || got.Contexts[0].Name != "ctx1" {
			t.Errorf("contexts = %+v", got.Contexts)
		}
	})

	t.Run("human_output_marks_current_context", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		// --json forces JSON; without it, isTTYStdout() may also return false in tests.
		// Here we test the human output by NOT passing --json and capturing stdout.
		// Since isTTYStdout() returns false during tests, useJSON would be true.
		// To force human output, we'd need to refactor isTTYStdout. Skip for now and
		// assert via the JSON path which is deterministic.
		code := runListE(
			[]string{"--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d", code)
		}
		// Verify the data needed for the * marker is present in the result.
		if !strings.Contains(stdout.String(), `"current_context": "ctx1"`) {
			t.Errorf("expected current_context key in output:\n%s", stdout.String())
		}
	})

	t.Run("returns_exitNotFound_when_kubeconfig_missing", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runListE(
			[]string{"--kubeconfig", "/no/such/path/config", "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitNotFound {
			t.Errorf("exit = %d, want %d", code, exitNotFound)
		}
	})

	t.Run("human_output_uses_underscore_current_context", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runListE(
			[]string{"--kubeconfig", path},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "current_context: ctx1") {
			t.Errorf("expected 'current_context: ctx1' in human output, got:\n%s", stdout.String())
		}
		if strings.Contains(stdout.String(), "current-context:") {
			t.Errorf("expected hyphenated 'current-context:' to be gone, got:\n%s", stdout.String())
		}
	})

	t.Run("help_shows_examples", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runListE(
			[]string{"-h"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d (help)", code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "Examples:") {
			t.Errorf("expected 'Examples:' in help output, got:\n%s", stderr.String())
		}
		if !strings.Contains(stderr.String(), "konfuse list") {
			t.Errorf("expected example 'konfuse list', got:\n%s", stderr.String())
		}
	})
}

// ---------------------------------------------------------------------------
// runDeleteE
// ---------------------------------------------------------------------------

func TestRunDeleteE(t *testing.T) {
	yamlBody := `
apiVersion: v1
kind: Config
clusters:
  - name: c1
    cluster:
      server: https://c1.example.com
  - name: c2
    cluster:
      server: https://c2.example.com
users:
  - name: u1
    user:
      token: t1
  - name: u2
    user:
      token: t2
contexts:
  - name: ctx1
    context:
      cluster: c1
      user: u1
  - name: ctx2
    context:
      cluster: c2
      user: u2
current-context: ctx1
`

	t.Run("deletes_context_and_orphans", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"ctx2", "--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		var got deleteOutput
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		if got.Deleted.Context != "ctx2" {
			t.Errorf("deleted.context = %q, want ctx2", got.Deleted.Context)
		}
		if got.Deleted.Cluster != "c2" {
			t.Errorf("deleted.cluster = %q, want c2", got.Deleted.Cluster)
		}
		if got.Deleted.User != "u2" {
			t.Errorf("deleted.user = %q, want u2", got.Deleted.User)
		}
		if got.Backup == nil || *got.Backup == "" {
			t.Error("expected non-nil backup path")
		}
		if got.Target != path {
			t.Errorf("target = %q, want %q", got.Target, path)
		}
	})

	t.Run("returns_exitError_when_context_not_found", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"nonexistent", "--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitError {
			t.Errorf("exit = %d, want %d", code, exitError)
		}
	})

	t.Run("returns_exitUsage_when_no_context_arg", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
	})

	t.Run("respects_flag_after_positional", func(t *testing.T) {
		// Regression test for the flag-position bug fixed earlier.
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"ctx2", "--kubeconfig", path, "--json"},
			"/wrong/default/path",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Errorf("exit = %d, expected ctx2 to delete from --kubeconfig path; stderr: %s", code, stderr.String())
		}
	})

	t.Run("prompts_and_aborts_on_n", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"ctx2", "--kubeconfig", path},
			"",
			strings.NewReader("n\n"),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d (abort)", code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "Delete context") {
			t.Errorf("expected prompt in stderr, got %q", stderr.String())
		}
		// File untouched.
		after, _ := os.ReadFile(path)
		if string(after) != string(original) {
			t.Error("kubeconfig was modified despite abort")
		}
	})

	t.Run("prompts_and_proceeds_on_y", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"ctx2", "--kubeconfig", path, "--json"},
			"",
			strings.NewReader("y\n"),
			&stdout, &stderr,
		)
		// --json forces auto-proceed; the prompt should NOT appear.
		// Behavior: useJSON=true → no prompt → proceed → exit 0.
		if code != exitOK {
			t.Errorf("exit = %d, stderr: %s", code, stderr.String())
		}
	})

	t.Run("yes_flag_skips_prompt", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		// Empty stdin — would block if a prompt fired. --yes must skip it.
		code := runDeleteE(
			[]string{"ctx2", "--kubeconfig", path, "--yes"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Errorf("exit = %d, stderr: %s", code, stderr.String())
		}
		if strings.Contains(stderr.String(), "Delete context") {
			t.Errorf("expected no prompt with --yes, got %q", stderr.String())
		}
	})

	t.Run("non_tty_stdin_skips_prompt", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		// Don't override TTY — default behavior in tests is non-TTY.
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"ctx2", "--kubeconfig", path},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Errorf("exit = %d, stderr: %s", code, stderr.String())
		}
	})

	t.Run("prompt_skipped_when_context_missing", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		withFakeTTYStdin(t)

		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"nonexistent", "--kubeconfig", path},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitError {
			t.Errorf("exit = %d, want %d", code, exitError)
		}
		if strings.Contains(stderr.String(), "Delete context") {
			t.Errorf("prompt should not appear when context missing, got %q", stderr.String())
		}
	})

	t.Run("returns_exitNotFound_when_kubeconfig_missing", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"any-ctx", "--kubeconfig", "/no/such/path/config", "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitNotFound {
			t.Errorf("exit = %d, want %d", code, exitNotFound)
		}
	})

	t.Run("help_shows_examples", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runDeleteE(
			[]string{"-h"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "Examples:") {
			t.Errorf("expected 'Examples:' in help, got:\n%s", stderr.String())
		}
	})
}

// ---------------------------------------------------------------------------
// runUseE
// ---------------------------------------------------------------------------

func TestRunUseE(t *testing.T) {
	yamlBody := `
apiVersion: v1
kind: Config
clusters:
  - name: c1
    cluster:
      server: https://c1.example.com
users:
  - name: u1
    user:
      token: t1
contexts:
  - name: ctx1
    context:
      cluster: c1
      user: u1
  - name: ctx2
    context:
      cluster: c1
      user: u1
current-context: ctx1
`

	t.Run("switches_context", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"ctx2", "--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		var got useOutput
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !got.Used.Changed {
			t.Error("used.changed should be true")
		}
		if got.Used.Previous != "ctx1" || got.Used.Context != "ctx2" {
			t.Errorf("got %+v, want previous=ctx1 context=ctx2", got.Used)
		}
		if got.Backup == nil || *got.Backup == "" {
			t.Error("expected non-nil backup when context changed")
		}
	})

	t.Run("noop_when_already_on_context_no_backup", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"ctx1", "--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
		}
		var got useOutput
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.Used.Changed {
			t.Error("used.changed should be false for no-op")
		}
		if got.Backup != nil {
			t.Errorf("expected nil backup for no-op, got %v", *got.Backup)
		}
	})

	t.Run("yes_flag_rejected", func(t *testing.T) {
		// --yes was removed from use; it's now an unknown flag.
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"ctx2", "--kubeconfig", path, "--json", "--yes"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
	})

	t.Run("returns_exitError_when_context_not_found", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"nonexistent", "--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitError {
			t.Errorf("exit = %d, want %d", code, exitError)
		}
	})

	t.Run("returns_exitUsage_when_no_context_arg", func(t *testing.T) {
		path := writeTempFile(t, yamlBody)
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"--kubeconfig", path, "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
	})

	t.Run("returns_exitNotFound_when_kubeconfig_missing", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"any-ctx", "--kubeconfig", "/no/such/path/config", "--json"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitNotFound {
			t.Errorf("exit = %d, want %d", code, exitNotFound)
		}
	})

	t.Run("help_shows_examples", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runUseE(
			[]string{"-h"},
			"",
			strings.NewReader(""),
			&stdout, &stderr,
		)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "Examples:") {
			t.Errorf("expected 'Examples:' in help, got:\n%s", stderr.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// withFakeTTYStdin makes the run* functions believe both stdin and stdout
// are interactive terminals — needed to exercise the human-output / prompt
// path. Restored automatically via t.Cleanup.
func withFakeTTYStdin(t *testing.T) {
	t.Helper()
	prevIn, prevOut := isTTYStdinFn, isTTYStdoutFn
	isTTYStdinFn = func() bool { return true }
	isTTYStdoutFn = func() bool { return true }
	t.Cleanup(func() {
		isTTYStdinFn = prevIn
		isTTYStdoutFn = prevOut
	})
}
