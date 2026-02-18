// Package cobraext provides Cobra command factories for agentquery schemas.
// It isolates the github.com/spf13/cobra dependency so that users who don't
// need CLI integration never import it.
package cobraext

import (
	"fmt"
	"strings"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
	"github.com/spf13/cobra"
)

// parseOutputMode converts a string flag value to an OutputMode.
// "compact" and "llm" map to LLMReadable; "json" maps to HumanReadable.
// Returns an error for unrecognized values.
func parseOutputMode(s string) (agentquery.OutputMode, error) {
	switch strings.ToLower(s) {
	case "compact", "llm":
		return agentquery.LLMReadable, nil
	case "json":
		return agentquery.HumanReadable, nil
	default:
		return 0, fmt.Errorf("unknown format %q: use \"json\", \"compact\", or \"llm\"", s)
	}
}

// QueryCommand creates a "q" subcommand that parses and executes a DSL query
// against the given schema. The query string is taken from positional args.
// The --format flag is required and controls output serialization.
func QueryCommand[T any](schema *agentquery.Schema[T]) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "q <query>",
		Short: "Execute a structured DSL query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseOutputMode(format)
			if err != nil {
				return err
			}
			data, err := schema.QueryJSONWithMode(args[0], mode)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}

	cmd.Flags().StringVar(&format, "format", "", `Output format (required): "json" or "compact"/"llm"`)
	_ = cmd.MarkFlagRequired("format")
	return cmd
}

// SearchCommand creates a "grep" subcommand that performs a full-text regex
// search within the schema's data directory. Supports --file, -i, -C, and --format flags.
// The --format flag is required and controls output serialization.
func SearchCommand[T any](schema *agentquery.Schema[T]) *cobra.Command {
	var (
		fileGlob        string
		caseInsensitive bool
		contextLines    int
		format          string
	)

	cmd := &cobra.Command{
		Use:   "grep <pattern>",
		Short: "Full-text regex search across data files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := agentquery.SearchOptions{
				FileGlob:        fileGlob,
				CaseInsensitive: caseInsensitive,
				ContextLines:    contextLines,
			}
			mode, err := parseOutputMode(format)
			if err != nil {
				return err
			}
			data, err := schema.SearchJSONWithMode(args[0], opts, mode)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}

	cmd.Flags().StringVar(&fileGlob, "file", "", "Glob pattern to filter files (e.g. \"*.md\")")
	cmd.Flags().BoolVarP(&caseInsensitive, "ignore-case", "i", false, "Case-insensitive search")
	cmd.Flags().IntVarP(&contextLines, "context", "C", 0, "Number of context lines around matches")
	cmd.Flags().StringVar(&format, "format", "", `Output format (required): "json" or "compact"/"llm"`)
	_ = cmd.MarkFlagRequired("format")

	return cmd
}

// MutateCommand creates an "m" subcommand that executes a mutation (write operation)
// against the given schema. Supports --format, --dry-run, and --confirm flags.
// Destructive mutations require --confirm (or --dry-run to preview).
func MutateCommand[T any](schema *agentquery.Schema[T]) *cobra.Command {
	var (
		format  string
		dryRun  bool
		confirm bool
	)

	cmd := &cobra.Command{
		Use:   "m <mutation>",
		Short: "Execute a mutation (write operation)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseOutputMode(format)
			if err != nil {
				return err
			}

			input := args[0]

			// If --dry-run, inject dry_run=true into the mutation string.
			if dryRun {
				input = injectDryRun(input)
			}

			// If not --confirm and not --dry-run, check destructive mutations.
			if !confirm && !dryRun && needsConfirm(schema, args[0]) {
				return fmt.Errorf("destructive mutation requires --confirm flag (or use --dry-run to preview)")
			}

			data, err := schema.QueryJSONWithMode(input, mode)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}

	cmd.Flags().StringVar(&format, "format", "", `Output format (required): "json" or "compact"/"llm"`)
	_ = cmd.MarkFlagRequired("format")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the mutation without applying changes")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destructive mutations")

	return cmd
}

// injectDryRun injects dry_run=true into each statement of a mutation string.
// For batched mutations separated by ";", it injects into each statement.
func injectDryRun(input string) string {
	parts := strings.Split(input, ";")
	for i, part := range parts {
		parts[i] = injectDryRunSingle(strings.TrimSpace(part))
	}
	return strings.Join(parts, "; ")
}

// injectDryRunSingle injects dry_run=true into a single mutation statement.
// Examples:
//
//	"create(title=\"X\")" → "create(title=\"X\", dry_run=true)"
//	"delete(task-1)"      → "delete(task-1, dry_run=true)"
//	"operation()"         → "operation(dry_run=true)"
func injectDryRunSingle(s string) string {
	// Find the last ')' in the statement.
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return s
	}

	// Find the matching '(' to check if args are empty.
	openIdx := strings.Index(s, "(")
	if openIdx < 0 {
		return s
	}

	inner := strings.TrimSpace(s[openIdx+1 : idx])
	if inner == "" {
		return s[:openIdx+1] + "dry_run=true" + s[idx:]
	}
	return s[:idx] + ", dry_run=true" + s[idx:]
}

// needsConfirm checks whether any mutation in the input requires --confirm.
// Returns true if any statement references a destructive mutation.
func needsConfirm[T any](schema *agentquery.Schema[T], input string) bool {
	for _, part := range strings.Split(input, ";") {
		name := extractOperationName(strings.TrimSpace(part))
		if name != "" && schema.IsMutationDestructive(name) {
			return true
		}
	}
	return false
}

// extractOperationName returns the operation name (first identifier before "(") from a statement.
func extractOperationName(s string) string {
	idx := strings.Index(s, "(")
	if idx <= 0 {
		return ""
	}
	return strings.TrimSpace(s[:idx])
}

// AddCommands adds the "q" and "grep" commands as subcommands of parent.
// If the schema has mutations registered, also adds the "m" command.
func AddCommands[T any](parent *cobra.Command, schema *agentquery.Schema[T]) {
	parent.AddCommand(QueryCommand(schema))
	parent.AddCommand(SearchCommand(schema))
	if schema.HasMutations() {
		parent.AddCommand(MutateCommand(schema))
	}
}
