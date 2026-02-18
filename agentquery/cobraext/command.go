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

// AddCommands adds both the "q" and "grep" commands as subcommands of parent.
func AddCommands[T any](parent *cobra.Command, schema *agentquery.Schema[T]) {
	parent.AddCommand(QueryCommand(schema))
	parent.AddCommand(SearchCommand(schema))
}
