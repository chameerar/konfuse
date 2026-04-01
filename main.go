package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func emit(data interface{}) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
	fmt.Print(buf.String())
}

func fail(useJSON bool, message, hint string, code int) {
	if useJSON {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(errorOutput{Error: message, Hint: hint})
		fmt.Fprint(os.Stderr, buf.String())
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", message)
		if hint != "" {
			fmt.Fprintf(os.Stderr, "Try:   %s\n", hint)
		}
	}
	os.Exit(code)
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

func main() {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")

	// Check for subcommands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "list":
			runList(os.Args[2:], defaultKubeconfig)
			return
		case "delete":
			runDelete(os.Args[2:], defaultKubeconfig)
			return
		}
	}

	showVersion := flag.Bool("version", false, "Print version and exit")
	renameContext := flag.String("rename-context", "", "Rename the first incoming context")
	renameCluster := flag.String("rename-cluster", "", "Rename the first incoming cluster")
	renameUser := flag.String("rename-user", "", "Rename the first incoming user")
	kubeconfig := flag.String("kubeconfig", defaultKubeconfig, "Target kubeconfig to merge into (default: ~/.kube/config)")
	dryRun := flag.Bool("dry-run", false, "Preview what would be merged without writing any changes")
	jsonOutput := flag.Bool("json", false, "Output results as JSON (auto-enabled when stdout is not a TTY)")
	_ = flag.Bool("yes", false, "Skip confirmation prompts (also auto-skipped in non-TTY / piped contexts)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: konfuse <input.yaml> [flags]\n")
		fmt.Fprintf(os.Stderr, "       konfuse list [flags]\n")
		fmt.Fprintf(os.Stderr, "       konfuse delete <context-name> [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Merge a new kubeconfig file into your existing kubeconfig.\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  list     List contexts, clusters, and users in the kubeconfig\n")
		fmt.Fprintf(os.Stderr, "  delete   Delete a context and its orphaned cluster/user\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  input    Path to the kubeconfig YAML file to merge\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  konfuse new-cluster.yaml\n")
		fmt.Fprintf(os.Stderr, "  konfuse new-cluster.yaml --rename-context prod --rename-cluster eks-prod\n")
		fmt.Fprintf(os.Stderr, "  konfuse new-cluster.yaml --dry-run --json\n")
		fmt.Fprintf(os.Stderr, "  konfuse new-cluster.yaml --kubeconfig /path/to/config\n")
		fmt.Fprintf(os.Stderr, "  konfuse list\n")
		fmt.Fprintf(os.Stderr, "  konfuse delete my-context\n")
	}
	input, flagArgs := extractPositional(os.Args[1:])
	flag.CommandLine.Parse(flagArgs) //nolint:errcheck

	if *showVersion {
		fmt.Printf("konfuse %s\n", version)
		os.Exit(exitOK)
	}

	if input == "" {
		fmt.Fprintln(os.Stderr, "Error: input file argument is required")
		flag.Usage()
		os.Exit(exitUsage)
	}

	useJSON := *jsonOutput || !isTTY()

	// Validate input file exists and is non-empty.
	fi, statErr := os.Stat(input)
	if os.IsNotExist(statErr) {
		fail(useJSON,
			fmt.Sprintf("Input file not found: %s", input),
			"konfuse <path-to-kubeconfig.yaml>",
			exitNotFound,
		)
	}
	if statErr == nil && fi.Size() == 0 {
		fail(useJSON,
			fmt.Sprintf("Input file is empty: %s", input),
			"Ensure the file is a valid kubeconfig YAML",
			exitNotFound,
		)
	}

	// Load and validate incoming kubeconfig.
	incoming, err := loadYAML(input)
	if err != nil {
		fail(useJSON,
			fmt.Sprintf("Failed to parse YAML: %s", err),
			"Ensure the file is valid YAML",
			exitError,
		)
	}
	if incoming == nil || incoming.Kind != "Config" {
		fail(useJSON,
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
			fail(useJSON,
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
			emit(dryRunOutput{
				DryRun:       true,
				Target:       *kubeconfig,
				Changes:      result,
				HasConflicts: hasConflicts,
			})
		} else {
			fmt.Println("Dry run — no changes will be written")
			fmt.Println()
			printChanges(result, true)
			if hasConflicts {
				fmt.Println("\nwarning: conflicts detected. Use --rename-* flags to avoid replacing existing entries.")
			}
		}
		os.Exit(exitOK)
	}

	// Backup then save.
	var backupPath *string
	if existingPathExists {
		bp, err := merger.BackupConfig(*kubeconfig)
		if err != nil {
			fail(useJSON, fmt.Sprintf("Failed to create backup: %s", err), "", exitError)
		}
		if bp != "" {
			backupPath = &bp
		}
	}

	if err := saveYAML(*kubeconfig, merged); err != nil {
		fail(useJSON, fmt.Sprintf("Failed to write kubeconfig: %s", err), "", exitError)
	}

	if useJSON {
		emit(mergeOutput{
			DryRun:       false,
			Target:       *kubeconfig,
			Backup:       backupPath,
			Changes:      result,
			HasConflicts: hasConflicts,
		})
	} else {
		if backupPath != nil {
			fmt.Printf("backup: %s\n", *backupPath)
		}
		fmt.Println()
		printChanges(result, false)
		if hasConflicts {
			fmt.Println("\nwarning: some entries were replaced. Use --rename-* flags to keep both versions.")
		}
		fmt.Printf("\nsaved: %s\n", *kubeconfig)
	}

	os.Exit(exitOK)
}

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

func runList(args []string, defaultKubeconfig string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Path to kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	fs.Parse(args) //nolint:errcheck

	useJSON := *jsonOutput || !isTTY()

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		fail(useJSON, fmt.Sprintf("Failed to load kubeconfig: %s", err), "", exitError)
	}

	result := merger.ListEntries(cfg)

	if useJSON {
		emit(result)
	} else {
		if result.CurrentContext != "" {
			fmt.Printf("current-context: %s\n\n", result.CurrentContext)
		}
		fmt.Println("CONTEXTS")
		if len(result.Contexts) == 0 {
			fmt.Println("  (none)")
		}
		for _, ctx := range result.Contexts {
			marker := " "
			if ctx.Name == result.CurrentContext {
				marker = "*"
			}
			fmt.Printf("  %s %-20s cluster=%-20s user=%s\n", marker, ctx.Name, ctx.Cluster, ctx.User)
		}
		fmt.Println()
		fmt.Println("CLUSTERS")
		if len(result.Clusters) == 0 {
			fmt.Println("  (none)")
		}
		for _, name := range result.Clusters {
			fmt.Printf("    %s\n", name)
		}
		fmt.Println()
		fmt.Println("USERS")
		if len(result.Users) == 0 {
			fmt.Println("  (none)")
		}
		for _, name := range result.Users {
			fmt.Printf("    %s\n", name)
		}
	}
	os.Exit(exitOK)
}

func runDelete(args []string, defaultKubeconfig string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	kubeconfig := fs.String("kubeconfig", defaultKubeconfig, "Path to kubeconfig")
	jsonOutput := fs.Bool("json", false, "Output as JSON (auto-enabled when stdout is not a TTY)")
	fs.Parse(args) //nolint:errcheck

	useJSON := *jsonOutput || !isTTY()

	contextName := fs.Arg(0)
	if contextName == "" {
		fail(useJSON, "context name is required", "konfuse delete <context-name>", exitUsage)
	}

	cfg, err := loadYAML(*kubeconfig)
	if err != nil {
		fail(useJSON, fmt.Sprintf("Failed to load kubeconfig: %s", err), "", exitError)
	}

	// Backup before modifying.
	bp, err := merger.BackupConfig(*kubeconfig)
	if err != nil {
		fail(useJSON, fmt.Sprintf("Failed to create backup: %s", err), "", exitError)
	}

	cfg, result, err := merger.DeleteContext(cfg, contextName)
	if err != nil {
		fail(useJSON, err.Error(), "konfuse list", exitError)
	}

	if err := saveYAML(*kubeconfig, cfg); err != nil {
		fail(useJSON, fmt.Sprintf("Failed to write kubeconfig: %s", err), "", exitError)
	}

	if useJSON {
		emit(struct {
			Deleted merger.DeleteResult `json:"deleted"`
			Backup  string              `json:"backup,omitempty"`
		}{
			Deleted: result,
			Backup:  bp,
		})
	} else {
		if bp != "" {
			fmt.Printf("backup: %s\n\n", bp)
		}
		fmt.Printf("  - Deleted context: %s\n", result.Context)
		if result.Cluster != "" {
			fmt.Printf("  - Deleted cluster: %s\n", result.Cluster)
		}
		if result.User != "" {
			fmt.Printf("  - Deleted user: %s\n", result.User)
		}
		fmt.Printf("\nsaved: %s\n", *kubeconfig)
	}
	os.Exit(exitOK)
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

func printChanges(result merger.MergeResult, dryRun bool) {
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
			fmt.Printf("  + %s %s: %s\n", addVerb, s.label, name)
		}
		for _, name := range s.result.Replaced {
			fmt.Printf("  ! %s %s: %s\n", replaceVerb, s.label, name)
		}
	}
}
