// Command taskdemo demonstrates the agentquery library with a simple task tracker domain.
//
// Build:
//
//	go build -o taskdemo .
//
// Usage (reads):
//
//	./taskdemo q 'schema()' --format json
//	./taskdemo q 'summary()' --format json
//	./taskdemo q 'get(task-1) { overview }' --format json
//	./taskdemo q 'list(status=done) { minimal }' --format json
//	./taskdemo q 'list(sort_name=asc) { overview }' --format json
//	./taskdemo q 'list(status=done, sort_name=desc) { overview }' --format json
//	./taskdemo q 'count()' --format json
//	./taskdemo q 'count(status=done)' --format json
//	./taskdemo q 'count(assignee=alice)' --format json
//	./taskdemo q 'count(status=todo, assignee=bob)' --format json
//	./taskdemo q 'list(skip=2, take=3) { overview }' --format json
//	./taskdemo q 'list(status=done, skip=0, take=2) { minimal }' --format json
//	./taskdemo q 'list(take=1) { full }' --format json
//	./taskdemo q 'distinct(status)' --format json
//	./taskdemo q 'distinct(assignee)' --format json
//	./taskdemo grep "TODO" --format json
//
// Usage (mutations):
//
//	./taskdemo m 'create(title="Fix login bug", status=todo)' --format json
//	./taskdemo m 'create(title="Test")' --format compact
//	./taskdemo m 'create(title="Dry run test")' --format json --dry-run
//	./taskdemo m 'update(task-1, status=done)' --format json
//	./taskdemo m 'update(task-1, title="New title", assignee=bob)' --format json
//	./taskdemo m 'delete(task-1)' --format json --confirm
//	./taskdemo m 'delete(task-1)' --format json --dry-run
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
	"github.com/relux-works/skill-agent-facing-api/agentquery/cobraext"
	"github.com/spf13/cobra"
)

// Task is the domain type for this example.
type Task struct {
	ID          string
	Name        string
	Status      string
	Assignee    string
	Priority    string
	Description string
}

// taskStore is a simple in-memory store with mutex protection.
// Mutations modify the store; reads return a snapshot.
type taskStore struct {
	mu    sync.Mutex
	tasks []Task
	nextN int // counter for generating IDs
}

func newTaskStore() *taskStore {
	return &taskStore{
		tasks: []Task{
			{ID: "task-1", Name: "Auth service refactor", Status: "in-progress", Assignee: "alice", Priority: "high", Description: "Refactor auth to use JWT tokens"},
			{ID: "task-2", Name: "Dashboard performance", Status: "todo", Assignee: "bob", Priority: "medium", Description: "Optimize dashboard load time to under 2s"},
			{ID: "task-3", Name: "Fix login redirect bug", Status: "done", Assignee: "alice", Priority: "high", Description: "Users get stuck on /callback after OAuth"},
			{ID: "task-4", Name: "Add dark mode", Status: "done", Assignee: "carol", Priority: "low", Description: "Implement dark mode toggle in settings"},
			{ID: "task-5", Name: "Pagination API", Status: "in-progress", Assignee: "dave", Priority: "medium", Description: "Add cursor-based pagination to list endpoints"},
			{ID: "task-6", Name: "CI pipeline speedup", Status: "todo", Assignee: "", Priority: "medium", Description: "Reduce CI build time from 12min to under 5min"},
			{ID: "task-7", Name: "Write onboarding docs", Status: "done", Assignee: "carol", Priority: "low", Description: "New developer onboarding guide"},
			{ID: "task-8", Name: "Rate limiter middleware", Status: "in-progress", Assignee: "bob", Priority: "high", Description: "Add per-user rate limiting to public API"},
		},
		nextN: 9,
	}
}

func (s *taskStore) load() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Task, len(s.tasks))
	copy(cp, s.tasks)
	return cp, nil
}

func (s *taskStore) add(t Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, t)
}

func (s *taskStore) generateID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("task-%d", s.nextN)
	s.nextN++
	return id
}

func (s *taskStore) findIndex(id string) int {
	for i, t := range s.tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (s *taskStore) update(id string, apply func(*Task)) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findIndex(id)
	if idx < 0 {
		return nil, &agentquery.Error{Code: agentquery.ErrNotFound, Message: fmt.Sprintf("task %q not found", id)}
	}
	apply(&s.tasks[idx])
	cp := s.tasks[idx]
	return &cp, nil
}

func (s *taskStore) remove(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findIndex(id)
	if idx < 0 {
		return nil, &agentquery.Error{Code: agentquery.ErrNotFound, Message: fmt.Sprintf("task %q not found", id)}
	}
	removed := s.tasks[idx]
	s.tasks = append(s.tasks[:idx], s.tasks[idx+1:]...)
	return &removed, nil
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
	store := newTaskStore()

	schema := agentquery.NewSchema[Task](
		agentquery.WithDataDir(dataDir()),
		agentquery.WithExtensions(".md"),
	)

	// Register fields — these map field names to accessor functions on Task.
	schema.Field("id", func(t Task) any { return t.ID })
	schema.Field("name", func(t Task) any { return t.Name })
	schema.Field("status", func(t Task) any { return t.Status })
	schema.Field("assignee", func(t Task) any { return t.Assignee })
	schema.Field("priority", func(t Task) any { return t.Priority })
	schema.Field("description", func(t Task) any { return t.Description })

	// Register presets — named bundles of fields for common projections.
	schema.Preset("minimal", "id", "status")
	schema.Preset("default", "id", "name", "status")
	schema.Preset("overview", "id", "name", "status", "assignee", "priority")
	schema.Preset("full", "id", "name", "status", "assignee", "priority", "description")

	// Default fields used when no projection is specified.
	schema.DefaultFields("default")

	// Register filterable fields — enables declarative filtering via query args
	// and auto-registers the built-in "distinct" operation.
	agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
	agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })
	agentquery.FilterableField(schema, "priority", func(t Task) string { return t.Priority })

	// Register sortable fields — enables sort_<field>=asc|desc in query args.
	agentquery.SortableField(schema, "id", func(t Task) string { return t.ID })
	agentquery.SortableField(schema, "name", func(t Task) string { return t.Name })
	agentquery.SortableField(schema, "status", func(t Task) string { return t.Status })
	agentquery.SortableField(schema, "assignee", func(t Task) string { return t.Assignee })
	agentquery.SortableField(schema, "priority", func(t Task) string { return t.Priority })

	// Set the data loader — returns a snapshot from the mutable store.
	schema.SetLoader(store.load)

	// Register read operations with metadata for schema introspection.
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

	// opList is registered as a closure to capture schema for SortSlice access.
	opList := func(ctx agentquery.OperationContext[Task]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}

		filtered := agentquery.FilterItems(items, ctx.Predicate)

		// Sort after filtering, before pagination.
		if err := agentquery.SortSlice(filtered, ctx.Statement.Args, schema.SortFields()); err != nil {
			return nil, err
		}

		// Apply skip/take pagination after filtering and sorting, before field projection.
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

	schema.OperationWithMetadata("list", opList, agentquery.OperationMetadata{
		Description: "List tasks with optional filters, sorting, and pagination",
		Parameters: []agentquery.ParameterDef{
			{Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
			{Name: "assignee", Type: "string", Optional: true, Description: "Filter by assignee"},
			{Name: "priority", Type: "string", Optional: true, Description: "Filter by priority"},
			{Name: "sort_<field>", Type: "asc|desc", Optional: true, Description: "Sort by field (id, name, status, assignee, priority)"},
			{Name: "skip", Type: "int", Optional: true, Default: 0, Description: "Skip first N items"},
			{Name: "take", Type: "int", Optional: true, Description: "Return at most N items"},
		},
		Examples: []string{
			"list() { overview }",
			"list(status=done) { minimal }",
			"list(sort_name=asc) { overview }",
			"list(status=done, sort_name=desc, skip=0, take=2) { overview }",
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

	// Register mutations — write operations that modify the in-memory store.
	schema.MutationWithMetadata("create", mutCreate(store), agentquery.MutationMetadata{
		Description: "Create a new task",
		Parameters: []agentquery.ParameterDef{
			{Name: "title", Type: "string", Required: true, Description: "Task title"},
			{Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}, Default: "todo", Description: "Task status"},
			{Name: "assignee", Type: "string", Description: "Assignee username"},
			{Name: "priority", Type: "string", Enum: []string{"low", "medium", "high"}, Default: "medium", Description: "Task priority"},
		},
		Destructive: false,
		Idempotent:  false,
		Examples:    []string{`create(title="Fix login bug")`, `create(title="New feature", status=in-progress, assignee=alice, priority=high)`},
	})

	schema.MutationWithMetadata("update", mutUpdate(store), agentquery.MutationMetadata{
		Description: "Update task fields by ID",
		Parameters: []agentquery.ParameterDef{
			{Name: "id", Type: "string", Required: true, Description: "Task ID (positional)"},
			{Name: "title", Type: "string", Description: "New title"},
			{Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}, Description: "New status"},
			{Name: "assignee", Type: "string", Description: "New assignee"},
			{Name: "priority", Type: "string", Enum: []string{"low", "medium", "high"}, Description: "New priority"},
		},
		Destructive: false,
		Idempotent:  true,
		Examples:    []string{`update(task-1, status=done)`, `update(task-1, title="New title", assignee=bob)`},
	})

	schema.MutationWithMetadata("delete", mutDelete(store), agentquery.MutationMetadata{
		Description: "Delete a task by ID",
		Parameters: []agentquery.ParameterDef{
			{Name: "id", Type: "string", Required: true, Description: "Task ID (positional)"},
		},
		Destructive: true,
		Idempotent:  true,
		Examples:    []string{`delete(task-1)`},
	})

	// Wire up Cobra root command with q, grep, and m subcommands.
	root := &cobra.Command{
		Use:   "taskdemo",
		Short: "Example task tracker using agentquery",
	}
	cobraext.AddCommands(root, schema)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- Read operation handlers ---

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

// opCount returns the count of tasks matching optional filters.
func opCount(ctx agentquery.OperationContext[Task]) (any, error) {
	items, err := ctx.Items()
	if err != nil {
		return nil, err
	}

	n := agentquery.CountItems(items, ctx.Predicate)
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

// --- Mutation handlers ---

// mutCreate returns a handler that creates a new task in the store.
func mutCreate(store *taskStore) agentquery.MutationHandler[Task] {
	return func(ctx agentquery.MutationContext[Task]) (any, error) {
		title := ctx.ArgMap["title"]
		if title == "" {
			return nil, &agentquery.Error{Code: agentquery.ErrValidation, Message: "title is required"}
		}

		status := ctx.ArgMap["status"]
		if status == "" {
			status = "todo"
		}
		priority := ctx.ArgMap["priority"]
		if priority == "" {
			priority = "medium"
		}

		if ctx.DryRun {
			return map[string]any{
				"dry_run": true,
				"would_create": map[string]any{
					"title":    title,
					"status":   status,
					"assignee": ctx.ArgMap["assignee"],
					"priority": priority,
				},
			}, nil
		}

		task := Task{
			ID:       store.generateID(),
			Name:     title,
			Status:   status,
			Assignee: ctx.ArgMap["assignee"],
			Priority: priority,
		}
		store.add(task)

		return map[string]any{
			"id":       task.ID,
			"title":    task.Name,
			"status":   task.Status,
			"assignee": task.Assignee,
			"priority": task.Priority,
		}, nil
	}
}

// mutUpdate returns a handler that updates an existing task by ID.
func mutUpdate(store *taskStore) agentquery.MutationHandler[Task] {
	return func(ctx agentquery.MutationContext[Task]) (any, error) {
		if len(ctx.Args) == 0 {
			return nil, &agentquery.Error{Code: agentquery.ErrValidation, Message: "update requires a task ID"}
		}
		targetID := ctx.Args[0].Value

		if ctx.DryRun {
			// Preview: show what fields would change.
			changes := map[string]any{}
			for _, key := range []string{"title", "status", "assignee", "priority"} {
				if v, ok := ctx.ArgMap[key]; ok {
					changes[key] = v
				}
			}
			return map[string]any{
				"dry_run":      true,
				"id":           targetID,
				"would_update": changes,
			}, nil
		}

		updated, err := store.update(targetID, func(t *Task) {
			if v, ok := ctx.ArgMap["title"]; ok {
				t.Name = v
			}
			if v, ok := ctx.ArgMap["status"]; ok {
				t.Status = v
			}
			if v, ok := ctx.ArgMap["assignee"]; ok {
				t.Assignee = v
			}
			if v, ok := ctx.ArgMap["priority"]; ok {
				t.Priority = v
			}
		})
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"id":       updated.ID,
			"title":    updated.Name,
			"status":   updated.Status,
			"assignee": updated.Assignee,
			"priority": updated.Priority,
		}, nil
	}
}

// mutDelete returns a handler that removes a task by ID.
func mutDelete(store *taskStore) agentquery.MutationHandler[Task] {
	return func(ctx agentquery.MutationContext[Task]) (any, error) {
		if len(ctx.Args) == 0 {
			return nil, &agentquery.Error{Code: agentquery.ErrValidation, Message: "delete requires a task ID"}
		}
		targetID := ctx.Args[0].Value

		if ctx.DryRun {
			// Preview: verify the task exists and show what would be deleted.
			items, err := ctx.Items()
			if err != nil {
				return nil, err
			}
			for _, t := range items {
				if t.ID == targetID {
					return map[string]any{
						"dry_run":      true,
						"would_delete": map[string]any{"id": t.ID, "title": t.Name, "status": t.Status},
					}, nil
				}
			}
			return nil, &agentquery.Error{Code: agentquery.ErrNotFound, Message: fmt.Sprintf("task %q not found", targetID)}
		}

		removed, err := store.remove(targetID)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"deleted": true,
			"id":      removed.ID,
			"title":   removed.Name,
		}, nil
	}
}
