// Command taskdemo demonstrates the agentquery library with a simple task tracker domain.
//
// Build:
//
//	go build -o taskdemo .
//
// Usage:
//
//	./taskdemo q 'schema()'
//	./taskdemo q 'summary()'
//	./taskdemo q 'get(task-1) { overview }'
//	./taskdemo q 'list(status=done) { minimal }'
//	./taskdemo q 'count()'
//	./taskdemo q 'count(status=done)'
//	./taskdemo q 'count(assignee=alice)'
//	./taskdemo q 'count(status=todo, assignee=bob)'
//	./taskdemo q 'list(skip=2, take=3) { overview }'
//	./taskdemo q 'list(status=done, skip=0, take=2) { minimal }'
//	./taskdemo q 'list(take=1) { full }'
//	./taskdemo grep "TODO"
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ivalx1s/skill-agent-facing-api/agentquery"
	"github.com/ivalx1s/skill-agent-facing-api/agentquery/cobraext"
	"github.com/spf13/cobra"
)

// Task is the domain type for this example.
type Task struct {
	ID          string
	Name        string
	Status      string
	Assignee    string
	Description string
}

// sampleTasks returns a fixed set of tasks for demonstration purposes.
func sampleTasks() []Task {
	return []Task{
		{ID: "task-1", Name: "Auth service refactor", Status: "in-progress", Assignee: "alice", Description: "Refactor auth to use JWT tokens"},
		{ID: "task-2", Name: "Dashboard performance", Status: "todo", Assignee: "bob", Description: "Optimize dashboard load time to under 2s"},
		{ID: "task-3", Name: "Fix login redirect bug", Status: "done", Assignee: "alice", Description: "Users get stuck on /callback after OAuth"},
		{ID: "task-4", Name: "Add dark mode", Status: "done", Assignee: "carol", Description: "Implement dark mode toggle in settings"},
		{ID: "task-5", Name: "Pagination API", Status: "in-progress", Assignee: "dave", Description: "Add cursor-based pagination to list endpoints"},
		{ID: "task-6", Name: "CI pipeline speedup", Status: "todo", Assignee: "", Description: "Reduce CI build time from 12min to under 5min"},
		{ID: "task-7", Name: "Write onboarding docs", Status: "done", Assignee: "carol", Description: "New developer onboarding guide"},
		{ID: "task-8", Name: "Rate limiter middleware", Status: "in-progress", Assignee: "bob", Description: "Add per-user rate limiting to public API"},
	}
}

// dataDir returns the absolute path to the example data directory.
// It resolves relative to the source file location so it works regardless of working directory.
func dataDir() string {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(file), "data")
	}
	// Fallback: assume working directory contains example/
	return "data"
}

func main() {
	// For LLM-optimized compact output, use --format compact on CLI commands:
	//   ./taskdemo q 'list() { overview }' --format compact
	//   ./taskdemo grep "TODO" --format compact
	schema := agentquery.NewSchema[Task](
		agentquery.WithDataDir(dataDir()),
		agentquery.WithExtensions(".md"),
	)

	// Register fields — these map field names to accessor functions on Task.
	schema.Field("id", func(t Task) any { return t.ID })
	schema.Field("name", func(t Task) any { return t.Name })
	schema.Field("status", func(t Task) any { return t.Status })
	schema.Field("assignee", func(t Task) any { return t.Assignee })
	schema.Field("description", func(t Task) any { return t.Description })

	// Register presets — named bundles of fields for common projections.
	schema.Preset("minimal", "id", "status")
	schema.Preset("default", "id", "name", "status")
	schema.Preset("overview", "id", "name", "status", "assignee")
	schema.Preset("full", "id", "name", "status", "assignee", "description")

	// Default fields used when no projection is specified.
	schema.DefaultFields("default")

	// Set the data loader — called lazily when an operation needs the item list.
	schema.SetLoader(func() ([]Task, error) {
		return sampleTasks(), nil
	})

	// Register operations with metadata for schema introspection.
	schema.OperationWithMetadata("get", opGet, agentquery.OperationMetadata{
		Description: "Find a single task by ID",
		Parameters: []agentquery.ParameterDef{
			{Name: "id", Type: "string", Optional: false, Description: "Task ID (positional)"},
		},
		Examples: []string{
			"get(task-1) { overview }",
			"get(task-3) { full }",
		},
	})

	schema.OperationWithMetadata("list", opList, agentquery.OperationMetadata{
		Description: "List tasks with optional filters and pagination",
		Parameters: []agentquery.ParameterDef{
			{Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
			{Name: "assignee", Type: "string", Optional: true, Description: "Filter by assignee"},
			{Name: "skip", Type: "int", Optional: true, Default: 0, Description: "Skip first N items"},
			{Name: "take", Type: "int", Optional: true, Description: "Return at most N items"},
		},
		Examples: []string{
			"list() { overview }",
			"list(status=done) { minimal }",
			"list(status=done, skip=0, take=2) { overview }",
			"list(assignee=alice) { full }",
		},
	})

	schema.OperationWithMetadata("count", opCount, agentquery.OperationMetadata{
		Description: "Count tasks matching optional filters",
		Parameters: []agentquery.ParameterDef{
			{Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
			{Name: "assignee", Type: "string", Optional: true, Description: "Filter by assignee"},
		},
		Examples: []string{
			"count()",
			"count(status=done)",
			"count(assignee=alice)",
		},
	})

	schema.OperationWithMetadata("summary", opSummary, agentquery.OperationMetadata{
		Description: "Return counts grouped by status",
		Examples: []string{
			"summary()",
		},
	})

	// Wire up Cobra root command with q and grep subcommands.
	root := &cobra.Command{
		Use:   "taskdemo",
		Short: "Example task tracker using agentquery",
	}
	cobraext.AddCommands(root, schema)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// opGet finds a single task by ID (first positional arg) and returns it projected through the selector.
func opGet(ctx agentquery.OperationContext[Task]) (any, error) {
	if len(ctx.Statement.Args) == 0 {
		return nil, &agentquery.Error{
			Code:    agentquery.ErrValidation,
			Message: "get requires a task ID argument",
		}
	}

	targetID := ctx.Statement.Args[0].Value
	items, err := ctx.Items()
	if err != nil {
		return nil, err
	}

	for _, task := range items {
		if task.ID == targetID {
			return ctx.Selector.Apply(task), nil
		}
	}

	return nil, &agentquery.Error{
		Code:    agentquery.ErrNotFound,
		Message: fmt.Sprintf("task %q not found", targetID),
		Details: map[string]any{"id": targetID},
	}
}

// taskFilterFromArgs builds a predicate from keyword arguments (status, assignee).
// Used by both list and count operations to share filtering logic.
func taskFilterFromArgs(args []agentquery.Arg) func(Task) bool {
	var filterStatus, filterAssignee string
	for _, arg := range args {
		switch arg.Key {
		case "status":
			filterStatus = arg.Value
		case "assignee":
			filterAssignee = arg.Value
		}
	}

	return func(t Task) bool {
		if filterStatus != "" && !strings.EqualFold(t.Status, filterStatus) {
			return false
		}
		if filterAssignee != "" && !strings.EqualFold(t.Assignee, filterAssignee) {
			return false
		}
		return true
	}
}

// opList returns tasks filtered by optional status and assignee keyword args.
// Supports skip/take pagination:
//
//	list(skip=2, take=3) { overview }
//	list(status=done, skip=0, take=5)
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
	items, err := ctx.Items()
	if err != nil {
		return nil, err
	}

	filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))

	// Apply skip/take pagination after filtering, before field projection
	page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0, len(page))
	for _, task := range page {
		results = append(results, ctx.Selector.Apply(task))
	}
	return results, nil
}

// opCount returns the count of tasks matching optional status and assignee filters.
// Accepts the same keyword args as list (status, assignee). Returns {"count": N}.
func opCount(ctx agentquery.OperationContext[Task]) (any, error) {
	items, err := ctx.Items()
	if err != nil {
		return nil, err
	}

	n := agentquery.CountItems(items, taskFilterFromArgs(ctx.Statement.Args))
	return map[string]any{"count": n}, nil
}

// opSummary returns counts grouped by status. Ignores field projections.
func opSummary(ctx agentquery.OperationContext[Task]) (any, error) {
	items, err := ctx.Items()
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	for _, task := range items {
		counts[task.Status]++
	}

	return map[string]any{
		"total":  len(items),
		"counts": counts,
	}, nil
}
