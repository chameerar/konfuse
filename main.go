package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chameerar/konfuse/internal/merger"
	"gopkg.in/yaml.v3"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

// Output structs preserve key order in JSON (Go maps sort alphabetically).

type dryRunOutput struct {
	DryRun       bool               `json:"dry_run"`
	Target       string             `json:"target"`
	Changes      merger.MergeResult `json:"changes"`
	HasConflicts bool               `json:"has_conflicts"`
}

type mergeOutput struct {
	DryRun       bool               `json:"dry_run"`
	Target       string             `json:"target"`
	Backup       *string            `json:"backup"`
	Changes      merger.MergeResult `json:"changes"`
	HasConflicts bool               `json:"has_conflicts"`
}

type errorOutput struct {
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
}

const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3
)

func isTTYStdout() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func isTTYStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// emit writes data to out as indented JSON.
func emit(out io.Writer, data interface{}) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
	fmt.Fprint(out, buf.String())
}

// fail writes an error and returns the given exit code. Callers (the runXE
// functions) return this value; the wrapper hands it to os.Exit.
func fail(errw io.Writer, useJSON bool, message, hint string, code int) int {
	if useJSON {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(errorOutput{Error: message, Hint: hint})
		fmt.Fprint(errw, buf.String())
	} else {
		fmt.Fprintf(errw, "Error: %s\n", message)
		if hint != "" {
			fmt.Fprintf(errw, "Try:   %s\n", hint)
		}
	}
	return code
}

func loadYAML(path string) (*merger.KubeConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg merger.KubeConfig
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveYAML(path string, cfg *merger.KubeConfig) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	f, err := os.Create(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	return enc.Encode(cfg)
}

// confirm prompts the user (default-no) and returns true to proceed. Auto-
// proceeds when useJSON or yes is set, or when stdinIsTTY is false.
func confirm(in io.Reader, errw io.Writer, prompt string, useJSON, yes, stdinIsTTY bool) bool {
	if useJSON || yes || !stdinIsTTY {
		return true
	}
	fmt.Fprintf(errw, "%s [y/N]: ", prompt)
	reader := bufio.NewReader(in)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// writeWithBackup creates a timestamped backup of path (if it exists), writes
// cfg to path, and returns the backup path ("" when no backup was created).
func writeWithBackup(path string, cfg *merger.KubeConfig) (backupPath string, err error) {
	backupPath, err = merger.BackupConfig(path)
	if err != nil {
		return "", err
	}
	if err := saveYAML(path, cfg); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}

// cmdUsage builds a flag.Usage closure for a subcommand.
func cmdUsage(fs *flag.FlagSet, errw io.Writer, synopsis string, examples []string) func() {
	return func() {
		fmt.Fprintf(errw, "Usage: konfuse %s\n\n", synopsis)
		fmt.Fprintf(errw, "Flags:\n")
		fs.SetOutput(errw)
		fs.PrintDefaults()
		if len(examples) > 0 {
			fmt.Fprintf(errw, "\nExamples:\n")
			for _, ex := range examples {
				fmt.Fprintf(errw, "  %s\n", ex)
			}
		}
	}
}

func main() {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")

	// Check for subcommands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "list":
			os.Exit(runListE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
		case "delete":
			os.Exit(runDeleteE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
		case "use":
			os.Exit(runUseE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
		}
	}

	os.Exit(runMergeE(os.Args[1:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
}

// ---------------------------------------------------------------------------
// Subcommand entry points
// ---------------------------------------------------------------------------

func runMergeE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "Print version and exit")
	renameContext := fs.String("rename-context", "", "Rename the first incoming context")
	renameCluster := fs.String("rename-cluster", "", "Rename the first incoming cluster")
	renameUser := fs.String("rename-user", "", "Rename the first incoming user")
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Target kubeconfig to merge into (default: ~/.kube/config)")
	dryRun := fs.Bool("dry-run", false, "Preview what would be merged without writing any changes")
	jsonOutput := fs.Bool("json", false, "Output results as JSON (auto-enabled when stdout is not a TTY)")
	yes := fs.Bool("yes", false, "Skip confirmation prompts (also auto-skipped in non-TTY / piped contexts)")
	_ = yes // wired in PR 2

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: konfuse <input.yaml> [flags]\n")
		fmt.Fprintf(stderr, "       konfuse list [flags]\n")
		fmt.Fprintf(stderr, "       konfuse delete <context-name> [flags]\n")
		fmt.Fprintf(stderr, "       konfuse use <context-name> [flags]\n\n")
		fmt.Fprintf(stderr, "Merge a new kubeconfig file into your existing kubeconfig.\n\n")
		fmt.Fprintf(stderr, "Commands:\n")
		fmt.Fprintf(stderr, "  list     List contexts, clusters, and users in the kubeconfig\n")
		fmt.Fprintf(stderr, "  delete   Delete a context and its orphaned cluster/user\n")
		fmt.Fprintf(stderr, "  use      Switch the active context (sets current-context)\n\n")
		fmt.Fprintf(stderr, "Arguments:\n")
		fmt.Fprintf(stderr, "  input    Path to the kubeconfig YAML file to merge\n\n")
		fmt.Fprintf(stderr, "Flags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nExamples:\n")
		fmt.Fprintf(stderr, "  konfuse new-cluster.yaml\n")
		fmt.Fprintf(stderr, "  konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod\n")
		fmt.Fprintf(stderr, "  konfuse new-cluster.yaml --dry-run --json\n")
		fmt.Fprintf(stderr, "  konfuse new-cluster.yaml --kubeconfig /path/to/config\n")
		fmt.Fprintf(stderr, "  konfuse list\n")
		fmt.Fprintf(stderr, "  konfuse delete my-context\n")
		fmt.Fprintf(stderr, "  konfuse use my-context\n")
	}

	input, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}

	if *showVersion {
		fmt.Fprintf(stdout, "konfuse %s\n", version)
		return exitOK
	}

	if input == "" {
		fmt.Fprintln(stderr, "Error: input file argument is required")
		fs.Usage()
		return exitUsage
	}

	useJSON := *jsonOutput || !isTTYStdout()

	// Validate input file exists and is non-empty.
	fi, statErr := os.Stat(input)
	if os.IsNotExist(statErr) {
		return fail(stderr, useJSON,
			fmt.Sprintf("Input file not found: %s", input),
			"konfuse <path-to-kubeconfig.yaml>",
			exitNotFound,
		)
	}
	if statErr == nil && fi.Size() == 0 {
		return fail(stderr, useJSON,
			fmt.Sprintf("Input file is empty: %s", input),
			"Ensure the file is a valid kubeconfig YAML",
			exitNotFound,
		)
	}

	// Load and validate incoming kubeconfig.
	incoming, err := loadYAML(input)
	if err != nil {
		return fail(stderr, useJSON,
			fmt.Sprintf("Failed to parse YAML: %s", err),
			"Ensure the file is valid YAML",
			exitError,
		)
	}
	if incoming == nil || incoming.Kind != "Config" {
		return fail(stderr, useJSON,
			"Input file is not a valid kubeconfig (missing kind: Config)",
			"Ensure the file is a valid kubeconfig YAML",
			exitError,
		)
	}

	// Load existing kubeconfig (may not exist yet).
	var existing *merger.KubeConfig
	existingPathExists := false
	if _, err := os.Stat(*kubeconfig); err == nil {
		existingPathExists = true
		existing, err = loadYAML(*kubeconfig)
		if err != nil {
			return fail(stderr, useJSON,
				fmt.Sprintf("Failed to parse existing kubeconfig: %s", err),
				fmt.Sprintf("Fix or remove the corrupted file: %s", *kubeconfig),
				exitError,
			)
		}
	}

	// Compute merge (pure — no I/O).
	merged, result := merger.MergeKubeconfig(existing, incoming, *renameContext, *renameCluster, *renameUser)

	hasConflicts := len(result.Clusters.Replaced) > 0 ||
		len(result.Users.Replaced) > 0 ||
		len(result.Contexts.Replaced) > 0

	if *dryRun {
		if useJSON {
			emit(stdout, dryRunOutput{
				DryRun:       true,
				Target:       *kubeconfig,
				Changes:      result,
				HasConflicts: hasConflicts,
			})
		} else {
			fmt.Fprintln(stdout, "Dry run — no changes will be written")
			fmt.Fprintln(stdout)
			printChanges(stdout, result, true)
			if hasConflicts {
				fmt.Fprintln(stdout, "\nwarning: conflicts detected. Use --rename-* flags to avoid replacing existing entries.")
			}
		}
		return exitOK
	}

	// Backup then save.
	var backupPath *string
	if existingPathExists {
		bp, err := merger.BackupConfig(*kubeconfig)
		if err != nil {
			return fail(stderr, useJSON, fmt.Sprintf("Failed to create backup: %s", err), "", exitError)
		}
		if bp != "" {
			backupPath = &bp
		}
	}

	if err := saveYAML(*kubeconfig, merged); err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("Failed to write kubeconfig: %s", err), "", exitError)
	}

	if useJSON {
		emit(stdout, mergeOutput{
			DryRun:       false,
			Target:       *kubeconfig,
			Backup:       backupPath,
			Changes:      result,
			HasConflicts: hasConflicts,
		})
	} else {
		if backupPath != nil {
			fmt.Fprintf(stdout, "backup: %s\n", *backupPath)
		}
		fmt.Fprintln(stdout)
		printChanges(stdout, result, false)
		if hasConflicts {
			fmt.Fprintln(stdout, "\nwarning: some entries were replaced. Use --rename-* flags to keep both versions.")
		}
		fmt.Fprintf(stdout, "\nsaved: %s\n", *kubeconfig)
	}

	_ = stdin // reserved for confirm prompt in PR 2
	return exitOK
}

func runListE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Path to kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	useJSON := *jsonOutput || !isTTYStdout()

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("Failed to load kubeconfig: %s", err), "", exitError)
	}

	result := merger.ListEntries(cfg)

	if useJSON {
		emit(stdout, result)
	} else {
		if result.CurrentContext != "" {
			fmt.Fprintf(stdout, "current-context: %s\n\n", result.CurrentContext)
		}
		fmt.Fprintln(stdout, "CONTEXTS")
		if len(result.Contexts) == 0 {
			fmt.Fprintln(stdout, "  (none)")
		}
		for _, ctx := range result.Contexts {
			marker := " "
			if ctx.Name == result.CurrentContext {
				marker = "*"
			}
			fmt.Fprintf(stdout, "  %s %-20s cluster=%-20s user=%s\n", marker, ctx.Name, ctx.Cluster, ctx.User)
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "CLUSTERS")
		if len(result.Clusters) == 0 {
			fmt.Fprintln(stdout, "  (none)")
		}
		for _, name := range result.Clusters {
			fmt.Fprintf(stdout, "    %s\n", name)
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "USERS")
		if len(result.Users) == 0 {
			fmt.Fprintln(stdout, "  (none)")
		}
		for _, name := range result.Users {
			fmt.Fprintf(stdout, "    %s\n", name)
		}
	}
	_ = stdin
	return exitOK
}

func runDeleteE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	contextName, flagArgs := extractPositional(args)

	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Path to kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}

	useJSON := *jsonOutput || !isTTYStdout()

	if contextName == "" {
		return fail(stderr, useJSON, "context name is required", "konfuse delete <context-name>", exitUsage)
	}

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("Failed to load kubeconfig: %s", err), "", exitError)
	}

	// Backup before modifying.
	bp, err := merger.BackupConfig(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("Failed to create backup: %s", err), "", exitError)
	}

	cfg, result, err := merger.DeleteContext(cfg, contextName)
	if err != nil {
		return fail(stderr, useJSON, err.Error(), "konfuse list", exitError)
	}

	if err := saveYAML(*kubeconfig, cfg); err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("Failed to write kubeconfig: %s", err), "", exitError)
	}

	if useJSON {
		emit(stdout, struct {
			Deleted merger.DeleteResult `json:"deleted"`
			Backup  string              `json:"backup,omitempty"`
		}{
			Deleted: result,
			Backup:  bp,
		})
	} else {
		if bp != "" {
			fmt.Fprintf(stdout, "backup: %s\n\n", bp)
		}
		fmt.Fprintf(stdout, "  - Deleted context: %s\n", result.Context)
		if result.Cluster != "" {
			fmt.Fprintf(stdout, "  - Deleted cluster: %s\n", result.Cluster)
		}
		if result.User != "" {
			fmt.Fprintf(stdout, "  - Deleted user: %s\n", result.User)
		}
		fmt.Fprintf(stdout, "\nsaved: %s\n", *kubeconfig)
	}
	_ = stdin
	return exitOK
}

func runUseE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	contextName, flagArgs := extractPositional(args)

	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Path to kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}

	useJSON := *jsonOutput || !isTTYStdout()

	if contextName == "" {
		return fail(stderr, useJSON, "context name is required", "konfuse use <context-name>", exitUsage)
	}

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("Failed to load kubeconfig: %s", err), "", exitError)
	}

	cfg, result, err := merger.UseContext(cfg, contextName)
	if err != nil {
		return fail(stderr, useJSON, err.Error(), "konfuse list", exitError)
	}

	// Skip the write (and the backup) when nothing changed.
	var backupPath string
	if result.Changed {
		backupPath, err = merger.BackupConfig(*kubeconfig)
		if err != nil {
			return fail(stderr, useJSON, fmt.Sprintf("Failed to create backup: %s", err), "", exitError)
		}
		if err := saveYAML(*kubeconfig, cfg); err != nil {
			return fail(stderr, useJSON, fmt.Sprintf("Failed to write kubeconfig: %s", err), "", exitError)
		}
	}

	if useJSON {
		emit(stdout, struct {
			Used   merger.UseResult `json:"used"`
			Backup string           `json:"backup,omitempty"`
		}{
			Used:   result,
			Backup: backupPath,
		})
	} else {
		if !result.Changed {
			fmt.Fprintf(stdout, "already on context: %s\n", result.Context)
			return exitOK
		}
		if backupPath != "" {
			fmt.Fprintf(stdout, "backup: %s\n\n", backupPath)
		}
		if result.Previous != "" {
			fmt.Fprintf(stdout, "switched context: %s -> %s\n", result.Previous, result.Context)
		} else {
			fmt.Fprintf(stdout, "switched context: %s\n", result.Context)
		}
		fmt.Fprintf(stdout, "\nsaved: %s\n", *kubeconfig)
	}
	_ = stdin
	return exitOK
}

// extractPositional separates the first non-flag argument (the positional input
// file) from the flag arguments so that Go's flag package can parse them
// correctly even when flags appear after the positional arg.
func extractPositional(args []string) (positional string, flagArgs []string) {
	// Flags that consume the following argument as their value.
	valueTakers := map[string]bool{
		"rename-context": true,
		"rename-cluster": true,
		"rename-user":    true,
		"kubeconfig":     true,
	}
	skipNext := false
	for _, arg := range args {
		if skipNext {
			flagArgs = append(flagArgs, arg)
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			name := strings.TrimLeft(arg, "-")
			if idx := strings.Index(name, "="); idx < 0 && valueTakers[name] {
				skipNext = true
			}
		} else if positional == "" {
			positional = arg
		} else {
			flagArgs = append(flagArgs, arg) // unexpected extra positional
		}
	}
	return
}

func printChanges(out io.Writer, result merger.MergeResult, dryRun bool) {
	addVerb, replaceVerb := "Added", "Replaced"
	if dryRun {
		addVerb, replaceVerb = "Would add", "Would replace"
	}
	sections := []struct {
		label  string
		result merger.SectionResult
	}{
		{"cluster", result.Clusters},
		{"user", result.Users},
		{"context", result.Contexts},
	}
	for _, s := range sections {
		for _, name := range s.result.Added {
			fmt.Fprintf(out, "  + %s %s: %s\n", addVerb, s.label, name)
		}
		for _, name := range s.result.Replaced {
			fmt.Fprintf(out, "  ! %s %s: %s\n", replaceVerb, s.label, name)
		}
	}
}
