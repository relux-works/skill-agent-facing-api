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

	// Register operations.
	schema.Operation("get", opGet)
	schema.Operation("list", opList)
	schema.Operation("summary", opSummary)

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

// opList returns tasks filtered by optional status and assignee keyword args.
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
	items, err := ctx.Items()
	if err != nil {
		return nil, err
	}

	// Extract optional filters from keyword arguments.
	var filterStatus, filterAssignee string
	for _, arg := range ctx.Statement.Args {
		switch arg.Key {
		case "status":
			filterStatus = arg.Value
		case "assignee":
			filterAssignee = arg.Value
		}
	}

	var results []map[string]any
	for _, task := range items {
		if filterStatus != "" && !strings.EqualFold(task.Status, filterStatus) {
			continue
		}
		if filterAssignee != "" && !strings.EqualFold(task.Assignee, filterAssignee) {
			continue
		}
		results = append(results, ctx.Selector.Apply(task))
	}

	if results == nil {
		results = []map[string]any{}
	}
	return results, nil
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
