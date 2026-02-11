// Package cobraext provides Cobra command factories for agentquery schemas.
// It isolates the github.com/spf13/cobra dependency so that users who don't
// need CLI integration never import it.
package cobraext

import (
	"fmt"

	"github.com/agentquery/agentquery"
	"github.com/spf13/cobra"
)

// QueryCommand creates a "q" subcommand that parses and executes a DSL query
// against the given schema. The query string is taken from positional args.
// Output is JSON written to stdout.
func QueryCommand[T any](schema *agentquery.Schema[T]) *cobra.Command {
	return &cobra.Command{
		Use:   "q <query>",
		Short: "Execute a structured DSL query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := schema.QueryJSON(args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}

// SearchCommand creates a "grep" subcommand that performs a full-text regex
// search within the schema's data directory. Supports --file, -i, and -C flags.
func SearchCommand[T any](schema *agentquery.Schema[T]) *cobra.Command {
	var (
		fileGlob        string
		caseInsensitive bool
		contextLines    int
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
			data, err := schema.SearchJSON(args[0], opts)
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

	return cmd
}

// AddCommands adds both the "q" and "grep" commands as subcommands of parent.
func AddCommands[T any](parent *cobra.Command, schema *agentquery.Schema[T]) {
	parent.AddCommand(QueryCommand(schema))
	parent.AddCommand(SearchCommand(schema))
}
