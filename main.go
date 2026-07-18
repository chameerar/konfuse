package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
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

type deleteOutput struct {
	Target  string              `json:"target"`
	Backup  *string             `json:"backup"`
	Deleted merger.DeleteResult `json:"deleted"`
}

type useOutput struct {
	Target string           `json:"target"`
	Backup *string          `json:"backup"`
	Used   merger.UseResult `json:"used"`
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

// isTTYStdinFn / isTTYStdoutFn are package-level indirections so tests can
// simulate an interactive terminal without manipulating os.Stdin / os.Stdout.
var (
	isTTYStdinFn  = isTTYStdin
	isTTYStdoutFn = isTTYStdout
)

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
		fmt.Fprintf(errw, "error: %s\n", message)
		if hint != "" {
			fmt.Fprintf(errw, "  try: %s\n", hint)
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

// cmdUsage builds a flag.Usage closure for a subcommand. The args slice is
// rendered as an "Arguments:" section before the flags so required positionals
// are visible in -h output.
func cmdUsage(fs *flag.FlagSet, synopsis, description string, args, examples []string) func() {
	return func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage: konfuse %s\n\n", synopsis)
		if description != "" {
			fmt.Fprintf(out, "%s\n\n", description)
		}
		if len(args) > 0 {
			fmt.Fprintln(out, "Arguments:")
			for _, a := range args {
				fmt.Fprintf(out, "  %s\n", a)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(out, "  -h, --help")
		fmt.Fprintln(out, "    \tShow this help and exit")
		if len(examples) > 0 {
			fmt.Fprintln(out, "\nExamples:")
			for _, ex := range examples {
				fmt.Fprintf(out, "  %s\n", ex)
			}
		}
	}
}

// printTopLevelUsage is shown by `konfuse --help` (no subcommand).
func printTopLevelUsage(errw io.Writer) {
	fmt.Fprintln(errw, "Usage: konfuse <command> [flags]")
	fmt.Fprintln(errw)
	fmt.Fprintln(errw, "Manage Kubernetes kubeconfig files: merge, list, switch context, delete context.")
	fmt.Fprintln(errw)
	fmt.Fprintln(errw, "Commands:")
	fmt.Fprintln(errw, "  merge <file>      Merge a kubeconfig file into the target kubeconfig")
	fmt.Fprintln(errw, "  list              List contexts, clusters, and users in the kubeconfig")
	fmt.Fprintln(errw, "  use <context>     Switch the active context (sets current-context)")
	fmt.Fprintln(errw, "  delete <context>  Delete a context and any orphaned cluster/user")
	fmt.Fprintln(errw)
	fmt.Fprintln(errw, "Flags:")
	fmt.Fprintln(errw, "  -h, --help        Show help (use `konfuse <command> -h` for command-specific help)")
	fmt.Fprintln(errw, "      --version     Print version and exit")
}

// hasTopLevelFlag scans args for an exact match of the given strings before any
// "--" separator. Used to make --version / --help work regardless of position.
func hasTopLevelFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		for _, f := range flags {
			if arg == f {
				return true
			}
		}
	}
	return false
}

func main() {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")

	// --version short-circuits everything. Works at any position so users
	// can drop it next to a file path or after a subcommand without thinking.
	if hasTopLevelFlag(os.Args[1:], "--version", "-version") {
		fmt.Printf("konfuse %s\n", version)
		os.Exit(exitOK)
	}

	if len(os.Args) <= 1 {
		printTopLevelUsage(os.Stderr)
		os.Exit(exitUsage)
	}

	switch os.Args[1] {
	case "--help", "-help", "-h":
		printTopLevelUsage(os.Stderr)
		os.Exit(exitOK)
	case "merge":
		os.Exit(runMergeE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
	case "list":
		os.Exit(runListE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
	case "delete":
		os.Exit(runDeleteE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
	case "use":
		os.Exit(runUseE(os.Args[2:], defaultKubeconfig, os.Stdin, os.Stdout, os.Stderr))
	default:
		// Merging requires the explicit `merge` subcommand — there is no
		// bare-file shortcut. When the argument looks like a path rather than
		// a flag, point the user at the merge command.
		arg := os.Args[1]
		hint := "konfuse --help"
		if !strings.HasPrefix(arg, "-") {
			hint = fmt.Sprintf("konfuse merge %s", arg)
		}
		os.Exit(fail(os.Stderr, !isTTYStdoutFn(), fmt.Sprintf("unknown command: %s", arg), hint, exitUsage))
	}
}

// ---------------------------------------------------------------------------
// Subcommand entry points
// ---------------------------------------------------------------------------

func runMergeE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	renameContext := fs.String("rename-context", "", "Rename-on-import: rename the first incoming context")
	renameCluster := fs.String("rename-cluster", "", "Rename-on-import: rename the first incoming cluster (also rewrites the context's cluster ref)")
	renameUser := fs.String("rename-user", "", "Rename-on-import: rename the first incoming user (also rewrites the context's user ref)")
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Target kubeconfig")
	dryRun := fs.Bool("dry-run", false, "Preview changes without writing")
	jsonOutput := fs.Bool("json", false, "Output results as JSON (auto-enabled when stdout is not a TTY)")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt before overwriting an existing kubeconfig")
	fs.Usage = cmdUsage(fs,
		"merge <input.yaml> [flags]",
		"Merge a kubeconfig file into the target kubeconfig. Rename-on-import flags affect only the first incoming entry of each kind.",
		[]string{"<input.yaml>    Path to the kubeconfig YAML file to merge (required)"},
		[]string{
			"konfuse merge new-cluster.yaml",
			"konfuse merge new-cluster.yaml --rename-context prod --rename-cluster eks-prod",
			"konfuse merge new-cluster.yaml --dry-run --json",
		})

	positionals, flagArgs := extractPositional(args)
	if code, ok := parseSubcommandFlags(fs, stderr, flagArgs); !ok {
		return code
	}

	useJSON := *jsonOutput || !isTTYStdoutFn()

	switch {
	case len(positionals) == 0:
		return fail(stderr, useJSON, "input file argument is required", "konfuse merge <path-to-kubeconfig.yaml>", exitUsage)
	case len(positionals) > 1:
		return fail(stderr, useJSON,
			fmt.Sprintf("unexpected extra argument(s): %s", strings.Join(positionals[1:], " ")),
			"konfuse merge <input.yaml> [flags]", exitUsage)
	}
	input := positionals[0]

	// Validate input file exists and is non-empty.
	fi, statErr := os.Stat(input)
	if os.IsNotExist(statErr) {
		return fail(stderr, useJSON,
			fmt.Sprintf("input file not found: %s", input),
			"konfuse merge <path-to-kubeconfig.yaml>",
			exitNotFound,
		)
	}
	if statErr == nil && fi.Size() == 0 {
		return fail(stderr, useJSON,
			fmt.Sprintf("input file is empty: %s", input),
			"",
			exitNotFound,
		)
	}

	// Load and validate incoming kubeconfig.
	incoming, err := loadYAML(input)
	if err != nil {
		return fail(stderr, useJSON,
			fmt.Sprintf("failed to parse YAML: %s", err),
			"",
			exitError,
		)
	}
	if incoming == nil || incoming.Kind != "Config" {
		return fail(stderr, useJSON,
			"input file is not a valid kubeconfig (missing kind: Config)",
			"",
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
				fmt.Sprintf("failed to parse existing kubeconfig: %s", err),
				"",
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

	if existingPathExists {
		prompt := fmt.Sprintf("Merge into %s? (will be backed up)", *kubeconfig)
		if !confirm(stdin, stderr, prompt, useJSON, *yes, isTTYStdinFn()) {
			return fail(stderr, useJSON, "aborted by user", "", exitUsage)
		}
	}

	bp, err := writeWithBackup(*kubeconfig, merged)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("failed to write kubeconfig: %s", err), "", exitError)
	}
	var backupPath *string
	if bp != "" {
		backupPath = &bp
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

	return exitOK
}

func runListE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Target kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	fs.Usage = cmdUsage(fs,
		"list [flags]",
		"List contexts, clusters, and users in the kubeconfig. Read-only.",
		nil,
		[]string{
			"konfuse list",
			"konfuse list --json",
			"konfuse list --kubeconfig /path/to/config",
		})

	positionals, flagArgs := extractPositional(args)
	if code, ok := parseSubcommandFlags(fs, stderr, flagArgs); !ok {
		return code
	}

	useJSON := *jsonOutput || !isTTYStdoutFn()

	if len(positionals) > 0 {
		return fail(stderr, useJSON,
			fmt.Sprintf("unexpected argument(s): %s", strings.Join(positionals, " ")),
			"konfuse list", exitUsage)
	}

	if _, err := os.Stat(*kubeconfig); os.IsNotExist(err) {
		return fail(stderr, useJSON, fmt.Sprintf("kubeconfig not found: %s", *kubeconfig), "", exitNotFound)
	}

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("failed to load kubeconfig: %s", err), "", exitError)
	}

	result := merger.ListEntries(cfg)

	if useJSON {
		emit(stdout, result)
	} else {
		if result.CurrentContext != "" {
			fmt.Fprintf(stdout, "current_context: %s\n\n", result.CurrentContext)
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
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Target kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt")
	fs.Usage = cmdUsage(fs,
		"delete <context-name> [flags]",
		"Delete a context from the kubeconfig and remove its cluster/user if no longer referenced.",
		[]string{"<context-name>  Context to delete (required)"},
		[]string{
			"konfuse delete my-context",
			"konfuse delete my-context --yes",
			"konfuse delete my-context --kubeconfig /path/to/config --json",
		})

	positionals, flagArgs := extractPositional(args)
	if code, ok := parseSubcommandFlags(fs, stderr, flagArgs); !ok {
		return code
	}

	useJSON := *jsonOutput || !isTTYStdoutFn()

	switch {
	case len(positionals) == 0:
		return fail(stderr, useJSON, "context name is required", "konfuse delete <context-name>", exitUsage)
	case len(positionals) > 1:
		return fail(stderr, useJSON,
			fmt.Sprintf("unexpected extra argument(s): %s", strings.Join(positionals[1:], " ")),
			"konfuse delete <context-name> [flags]", exitUsage)
	}
	contextName := positionals[0]

	if _, err := os.Stat(*kubeconfig); os.IsNotExist(err) {
		return fail(stderr, useJSON, fmt.Sprintf("kubeconfig not found: %s", *kubeconfig), "", exitNotFound)
	}

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("failed to load kubeconfig: %s", err), "", exitError)
	}

	cfg, result, err := merger.DeleteContext(cfg, contextName)
	if err != nil {
		return fail(stderr, useJSON, err.Error(), "konfuse list", exitError)
	}

	prompt := fmt.Sprintf("Delete context %q from %s?", contextName, *kubeconfig)
	if !confirm(stdin, stderr, prompt, useJSON, *yes, isTTYStdinFn()) {
		return fail(stderr, useJSON, "aborted by user", "", exitUsage)
	}

	bp, err := writeWithBackup(*kubeconfig, cfg)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("failed to write kubeconfig: %s", err), "", exitError)
	}
	var backupPath *string
	if bp != "" {
		backupPath = &bp
	}

	if useJSON {
		emit(stdout, deleteOutput{
			Target:  *kubeconfig,
			Backup:  backupPath,
			Deleted: result,
		})
	} else {
		fmt.Fprintf(stdout, "Deleted context %q\n", result.Context)

		if result.Cluster != "" || result.User != "" {
			fmt.Fprintf(stdout, "  - also removed")
			if result.Cluster != "" {
				fmt.Fprintf(stdout, " cluster %q", result.Cluster)
			}
			if result.User != "" {
				if result.Cluster != "" {
					fmt.Fprint(stdout, ",")
				}
				fmt.Fprintf(stdout, " user %q", result.User)
			}
			fmt.Fprintln(stdout)
		}

		if backupPath != nil {
			fmt.Fprintf(stdout, "backup: %s\n", filepath.Base(*backupPath))
		}
	}
	return exitOK
}

func runUseE(args []string, defaultKubeconfig string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Target kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	fs.Usage = cmdUsage(fs,
		"use <context-name> [flags]",
		"Switch the active context (sets current-context). No-op (no backup, no write) when already on the requested context.",
		[]string{"<context-name>  Context to switch to (required)"},
		[]string{
			"konfuse use prod",
			"konfuse use prod --kubeconfig /path/to/config",
			"konfuse use prod --json",
		})

	positionals, flagArgs := extractPositional(args)
	if code, ok := parseSubcommandFlags(fs, stderr, flagArgs); !ok {
		return code
	}

	useJSON := *jsonOutput || !isTTYStdoutFn()

	switch {
	case len(positionals) == 0:
		return fail(stderr, useJSON, "context name is required", "konfuse use <context-name>", exitUsage)
	case len(positionals) > 1:
		return fail(stderr, useJSON,
			fmt.Sprintf("unexpected extra argument(s): %s", strings.Join(positionals[1:], " ")),
			"konfuse use <context-name> [flags]", exitUsage)
	}
	contextName := positionals[0]

	if _, err := os.Stat(*kubeconfig); os.IsNotExist(err) {
		return fail(stderr, useJSON, fmt.Sprintf("kubeconfig not found: %s", *kubeconfig), "", exitNotFound)
	}

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		return fail(stderr, useJSON, fmt.Sprintf("failed to load kubeconfig: %s", err), "", exitError)
	}

	cfg, result, err := merger.UseContext(cfg, contextName)
	if err != nil {
		return fail(stderr, useJSON, err.Error(), "konfuse list", exitError)
	}

	// Skip the write (and the backup) when nothing changed.
	var backupPath *string
	if result.Changed {
		bp, werr := writeWithBackup(*kubeconfig, cfg)
		if werr != nil {
			return fail(stderr, useJSON, fmt.Sprintf("failed to write kubeconfig: %s", werr), "", exitError)
		}
		if bp != "" {
			backupPath = &bp
		}
	}

	if useJSON {
		emit(stdout, useOutput{
			Target: *kubeconfig,
			Backup: backupPath,
			Used:   result,
		})
	} else {
		if !result.Changed {
			fmt.Fprintf(stdout, "Already on context %q.\n", result.Context)
			return exitOK
		}

		if result.Previous != "" {
			fmt.Fprintf(stdout, "Switched to context %q (was %q).\n", result.Context, result.Previous)
		} else {
			fmt.Fprintf(stdout, "Switched to context %q.\n", result.Context)
		}

		if backupPath != nil {
			fmt.Fprintf(stdout, "backup: %s\n", filepath.Base(*backupPath))
		}
	}
	_ = stdin
	return exitOK
}

// extractPositional separates positional (non-flag) arguments from flag
// arguments so that Go's flag package parses correctly even when flags appear
// after a positional. All positionals are returned in order; callers decide
// how many are allowed and reject the rest (so a stray argument can never
// silently swallow a following flag such as --kubeconfig).
func extractPositional(args []string) (positionals []string, flagArgs []string) {
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
			if !strings.Contains(name, "=") && valueTakers[name] {
				skipNext = true
			}
		} else {
			positionals = append(positionals, arg)
		}
	}
	return
}

// parseSubcommandFlags parses flagArgs and formats failures in konfuse's own
// style instead of the flag package's raw "flag provided but not defined"
// output. It returns (code, ok): when ok is false the caller returns code
// immediately — exitOK when -h/--help was requested (help is printed to
// stderr), or exitUsage for a genuine flag error.
func parseSubcommandFlags(fs *flag.FlagSet, stderr io.Writer, flagArgs []string) (int, bool) {
	// Suppress the flag package's automatic error/usage output so we control
	// formatting; restore stderr before printing anything ourselves.
	fs.SetOutput(io.Discard)
	err := fs.Parse(flagArgs)
	fs.SetOutput(stderr)
	if err == nil {
		return exitOK, true
	}
	if errors.Is(err, flag.ErrHelp) {
		fs.Usage()
		return exitOK, false
	}
	return fail(stderr, !isTTYStdoutFn(), err.Error(), fmt.Sprintf("konfuse %s -h", fs.Name()), exitUsage), false
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
			fmt.Fprintf(out, "  ~ %s %s: %s\n", replaceVerb, s.label, name)
		}
	}
}
