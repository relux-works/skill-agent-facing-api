# Mutation Design Proposal for agentquery

**Date**: 2026-02-18
**Task**: TASK-260218-3g2c28
**Status**: Design Proposal
**Inputs**: GraphQL mutations research, CQRS/Hasura/tRPC/Firestore/CLI/Agent API patterns research

---

## Table of Contents

1. [DSL Syntax for Mutations](#1-dsl-syntax-for-mutations)
2. [Schema Registration API (Go)](#2-schema-registration-api-go)
3. [Schema Introspection](#3-schema-introspection)
4. [Execution & Response](#4-execution--response)
5. [Cobra Integration](#5-cobra-integration)
6. [Parser Changes](#6-parser-changes)
7. [Implementation Plan](#7-implementation-plan)
8. [Open Questions](#8-open-questions)

---

## 1. DSL Syntax for Mutations

### Decision: Same Grammar, Different Registry

Every researched pattern agrees: **the operation name determines read vs write, not the syntax**. tRPC, SQL, GraphQL, kubectl, and all CLI tools use the same grammar/parser for both. The current `operation(args) { fields }` grammar covers mutations without changes.

### Mutation Invocations

```
# Create
create(title="Fix login bug", status=todo, priority=high)

# Update by ID (positional first arg = target ID)
update(task-1, status=done)

# Delete by ID
delete(task-1)

# Domain-specific verbs (CQRS naming: domain verbs, not CRUD)
assign(task-1, assignee=alice)
archive(task-1)
reopen(task-1)
```

No field projection on mutation results. Mutations return a fixed response shape (see section 4). The `{ fields }` clause is syntactically allowed (parser doesn't change), but mutation handlers ignore it. This is documented, not enforced — handlers could use it if they want.

### Batched Mutations

Same `;` separator as queries. Sequential execution, per-statement error isolation (no rollback):

```
update(task-1, status=done); assign(task-2, assignee=bob); delete(task-3)
```

### Mixed Batches (Queries + Mutations)

**Allowed.** The parser doesn't distinguish queries from mutations — it sees operations. The schema routes each statement to the correct handler (query handler or mutation handler). This matches tRPC and SQL behavior.

```
update(task-1, status=done); get(task-1) { overview }; count(status=done)
```

The `update` runs first (mutation), then `get` reads the updated state, then `count` reflects the change. Sequential execution guarantees this ordering.

### Dry-Run Convention

`dry_run=true` as a regular arg, handled by the framework before calling the mutation handler:

```
delete(task-1, dry_run=true)
update(task-1, status=done, dry_run=true)
```

In dry-run mode, the framework calls the handler's validation logic but skips execution. See section 2 for how this is implemented.

---

## 2. Schema Registration API (Go)

### New Types

```go
// MutationHandler is the function signature for mutation implementations.
// It receives a MutationContext and returns a result object and/or error.
// The result should be a JSON-serializable representation of the affected entity
// (or a confirmation map for destructive operations like delete).
type MutationHandler[T any] func(ctx MutationContext[T]) (any, error)

// MutationContext provides data to mutation handlers during execution.
// Unlike OperationContext, it does not include Selector (no field projection)
// but adds ArgMap for convenient key-value access and DryRun flag.
type MutationContext[T any] struct {
    Mutation   string            // mutation operation name
    Statement  Statement         // full parsed statement (for positional args)
    Args       []Arg             // parsed arguments
    ArgMap     map[string]string // key=value args as map (convenience)
    Items      func() ([]T, error) // lazy item loader (for lookups/validation)
    DryRun     bool              // true when dry_run=true was passed
}

// MutationResult wraps a mutation's outcome for consistent response shape.
// Handlers return (any, error) — the framework wraps into MutationResult
// before serialization.
type MutationResult struct {
    Ok     bool   `json:"ok"`
    Result any    `json:"result,omitempty"`
    Errors []MutationError `json:"errors,omitempty"`
}

// MutationError describes a validation or domain error from a mutation.
// Field pinpoints which input argument caused the error (empty for general errors).
type MutationError struct {
    Field   string `json:"field,omitempty"`   // which arg caused the error
    Message string `json:"message"`
    Code    string `json:"code,omitempty"`     // machine-readable: REQUIRED, INVALID_VALUE, CONFLICT, NOT_FOUND
}

// MutationMetadata extends OperationMetadata with mutation-specific annotations.
// These are surfaced by schema() introspection and used by agents for
// safety decisions (confirm before destructive, skip confirm for idempotent).
type MutationMetadata struct {
    Description string         `json:"description,omitempty"`
    Parameters  []ParameterDef `json:"parameters,omitempty"`
    Examples    []string       `json:"examples,omitempty"`
    Destructive bool           `json:"destructive"`   // irreversible changes (delete, drop)
    Idempotent  bool           `json:"idempotent"`    // repeated calls with same args are safe
}
```

### ParameterDef Extension

The existing `ParameterDef` struct needs two new fields for mutation parameter validation:

```go
type ParameterDef struct {
    Name        string   `json:"name"`
    Type        string   `json:"type"`                  // "string", "int", "bool"
    Optional    bool     `json:"optional"`
    Default     any      `json:"default,omitempty"`
    Description string   `json:"description,omitempty"`
    Enum        []string `json:"enum,omitempty"`         // NEW: allowed values
    Required    bool     `json:"required,omitempty"`     // NEW: explicit required (inverse of Optional, for clarity in mutation metadata)
}
```

Note: `Required` is the inverse of `Optional`. Both exist for ergonomics — read operations naturally express "this param is optional" while mutations naturally express "this param is required". The schema introspection emits both; consuming code uses whichever is clearer.

### Schema Registration Methods

```go
// Mutation registers a named mutation with its handler function.
// The mutation is stored separately from read operations (different registry).
func (s *Schema[T]) Mutation(name string, handler MutationHandler[T]) {
    s.mutations[name] = handler
    // Also register as an operation in the parser's known-operations map
    // so the parser accepts the name. The executeStatement method routes
    // to the mutation registry first, then falls back to operations.
    s.operations[name] = s.wrapMutation(name, handler)
}

// MutationWithMetadata registers a named mutation with its handler and metadata.
// Metadata is stored separately for schema introspection.
func (s *Schema[T]) MutationWithMetadata(name string, handler MutationHandler[T], meta MutationMetadata) {
    s.Mutation(name, handler)
    s.mutationMetadata[name] = meta
}
```

### Internal Wiring: wrapMutation

The key insight: mutations register in a separate `mutations` map but are also added to the `operations` map (wrapped) so the parser accepts them and `executeStatement` can dispatch them. The wrapper adapts `MutationHandler` to `OperationHandler`:

```go
func (s *Schema[T]) wrapMutation(name string, handler MutationHandler[T]) OperationHandler[T] {
    return func(ctx OperationContext[T]) (any, error) {
        // Build ArgMap from args
        argMap := make(map[string]string, len(ctx.Statement.Args))
        for _, arg := range ctx.Statement.Args {
            if arg.Key != "" {
                argMap[arg.Key] = arg.Value
            }
        }

        // Check dry_run flag
        dryRun := false
        if v, ok := argMap["dry_run"]; ok && (v == "true" || v == "1" || v == "yes") {
            dryRun = true
            delete(argMap, "dry_run") // don't pass to handler
        }

        mctx := MutationContext[T]{
            Mutation:   name,
            Statement:  ctx.Statement,
            Args:       ctx.Statement.Args,
            ArgMap:     argMap,
            Items:      ctx.Items,
            DryRun:     dryRun,
        }

        // Framework-level validation from metadata (if registered)
        if meta, ok := s.mutationMetadata[name]; ok {
            if errs := validateArgs(mctx.ArgMap, mctx.Args, meta.Parameters); len(errs) > 0 {
                return MutationResult{Ok: false, Errors: errs}, nil
            }
        }

        result, err := handler(mctx)
        if err != nil {
            // Convert Go error to MutationResult with error
            return MutationResult{
                Ok:     false,
                Errors: []MutationError{{Message: err.Error()}},
            }, nil
        }

        return MutationResult{Ok: true, Result: result}, nil
    }
}
```

### Framework-Level Validation

```go
// validateArgs checks required params and enum constraints from metadata.
// Returns nil if all checks pass. This is Layer 1 (structural) validation;
// Layer 2 (domain) validation belongs in the handler.
func validateArgs(argMap map[string]string, args []Arg, params []ParameterDef) []MutationError {
    var errs []MutationError
    for _, p := range params {
        if p.Required || !p.Optional {
            // Check if required param is present (in argMap or as positional)
            if _, ok := argMap[p.Name]; !ok {
                // Check positional args
                found := false
                for _, a := range args {
                    if a.Key == "" && a.Value != "" {
                        found = true // positional arg present
                        break
                    }
                }
                if !found || p.Name != args[0].Key { // simplified — real impl needs positional matching
                    errs = append(errs, MutationError{
                        Field:   p.Name,
                        Message: fmt.Sprintf("required parameter %q is missing", p.Name),
                        Code:    "REQUIRED",
                    })
                }
            }
        }
        // Enum validation
        if len(p.Enum) > 0 {
            if val, ok := argMap[p.Name]; ok {
                valid := false
                for _, e := range p.Enum {
                    if strings.EqualFold(val, e) {
                        valid = true
                        break
                    }
                }
                if !valid {
                    errs = append(errs, MutationError{
                        Field:   p.Name,
                        Message: fmt.Sprintf("invalid value %q for %s, must be one of: %s", val, p.Name, strings.Join(p.Enum, ", ")),
                        Code:    "INVALID_VALUE",
                    })
                }
            }
        }
    }
    return errs
}
```

### Example: Registering Mutations in a CLI Tool

```go
schema.MutationWithMetadata("create", createHandler, agentquery.MutationMetadata{
    Description: "Create a new task",
    Parameters: []agentquery.ParameterDef{
        {Name: "title", Type: "string", Required: true, Description: "Task title"},
        {Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}, Default: "todo"},
        {Name: "assignee", Type: "string", Description: "Assignee username"},
        {Name: "priority", Type: "string", Enum: []string{"low", "medium", "high"}, Default: "medium"},
    },
    Destructive: false,
    Idempotent:  false,
    Examples:    []string{`create(title="Fix login bug")`, `create(title="New feature", status=in-progress, assignee=alice)`},
})

schema.MutationWithMetadata("update", updateHandler, agentquery.MutationMetadata{
    Description: "Update task fields by ID",
    Parameters: []agentquery.ParameterDef{
        {Name: "id", Type: "string", Required: true, Description: "Task ID (positional)"},
        {Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}},
        {Name: "assignee", Type: "string"},
        {Name: "title", Type: "string"},
    },
    Destructive: false,
    Idempotent:  true,
    Examples:    []string{`update(task-1, status=done)`, `update(task-1, assignee=bob, title="New title")`},
})

schema.MutationWithMetadata("delete", deleteHandler, agentquery.MutationMetadata{
    Description: "Delete a task by ID",
    Parameters: []agentquery.ParameterDef{
        {Name: "id", Type: "string", Required: true, Description: "Task ID (positional)"},
    },
    Destructive: true,
    Idempotent:  true,
    Examples:    []string{`delete(task-1)`},
})
```

### Example: Mutation Handler Implementation

```go
func createHandler(ctx agentquery.MutationContext[Task]) (any, error) {
    title := ctx.ArgMap["title"]
    if title == "" {
        return nil, &agentquery.Error{
            Code:    agentquery.ErrValidation,
            Message: "title is required",
        }
    }

    status := ctx.ArgMap["status"]
    if status == "" {
        status = "todo"
    }

    if ctx.DryRun {
        return map[string]any{
            "dry_run": true,
            "would_create": map[string]any{
                "title":  title,
                "status": status,
            },
        }, nil
    }

    // Actual creation logic (domain-specific)
    newTask := Task{
        ID:     generateID(),
        Name:   title,
        Status: status,
        Assignee: ctx.ArgMap["assignee"],
    }
    // ... persist newTask ...

    return map[string]any{
        "id":     newTask.ID,
        "title":  newTask.Name,
        "status": newTask.Status,
    }, nil
}
```

---

## 3. Schema Introspection

### Mutations in schema() Output

Mutations appear in a **separate `mutations` section**, not mixed with operations. This gives agents a clear read/write boundary:

```json
{
  "operations": ["list", "get", "count", "summary", "schema", "distinct"],
  "mutations": ["create", "update", "delete", "assign"],
  "fields": ["id", "name", "status", "assignee", "description"],
  "presets": {
    "minimal": ["id", "status"],
    "overview": ["id", "name", "status", "assignee"],
    "full": ["id", "name", "status", "assignee", "description"]
  },
  "defaultFields": ["default"],
  "filterableFields": ["status", "assignee"],
  "sortableFields": ["id", "name", "status", "assignee"],
  "operationMetadata": {
    "list": {
      "description": "List tasks with optional filters, sorting, and pagination",
      "parameters": [...]
    }
  },
  "mutationMetadata": {
    "create": {
      "description": "Create a new task",
      "parameters": [
        {"name": "title", "type": "string", "required": true, "description": "Task title"},
        {"name": "status", "type": "string", "enum": ["todo", "in-progress", "done"], "default": "todo"},
        {"name": "assignee", "type": "string", "description": "Assignee username"},
        {"name": "priority", "type": "string", "enum": ["low", "medium", "high"], "default": "medium"}
      ],
      "destructive": false,
      "idempotent": false,
      "examples": ["create(title=\"Fix login bug\")", "create(title=\"New feature\", status=in-progress)"]
    },
    "update": {
      "description": "Update task fields by ID",
      "parameters": [
        {"name": "id", "type": "string", "required": true, "description": "Task ID (positional)"},
        {"name": "status", "type": "string", "enum": ["todo", "in-progress", "done"]},
        {"name": "assignee", "type": "string"},
        {"name": "title", "type": "string"}
      ],
      "destructive": false,
      "idempotent": true,
      "examples": ["update(task-1, status=done)"]
    },
    "delete": {
      "description": "Delete a task by ID",
      "parameters": [
        {"name": "id", "type": "string", "required": true, "description": "Task ID (positional)"}
      ],
      "destructive": true,
      "idempotent": true,
      "examples": ["delete(task-1)"]
    }
  }
}
```

### Agent Discovery Flow

1. `schema()` — discover available operations and mutations, see their metadata
2. Read `mutations` array — know which operations have side effects
3. Read `mutationMetadata.<name>.parameters` — learn exact input shape
4. Read `mutationMetadata.<name>.destructive` — decide whether to confirm with user
5. Read `mutationMetadata.<name>.examples` — see concrete invocations
6. Execute: `create(title="Fix bug")` via `q` or `m` command

### Implementation in introspect()

```go
func (s *Schema[T]) introspect() map[string]any {
    // ... existing operations, fields, presets, defaults ...

    // Include mutations list (sorted) if any mutations are registered
    if len(s.mutations) > 0 {
        muts := make([]string, 0, len(s.mutations))
        for name := range s.mutations {
            muts = append(muts, name)
        }
        sort.Strings(muts)
        result["mutations"] = muts
    }

    // Include mutationMetadata if any mutations have metadata registered
    if len(s.mutationMetadata) > 0 {
        meta := make(map[string]MutationMetadata, len(s.mutationMetadata))
        for name, m := range s.mutationMetadata {
            meta[name] = m
        }
        result["mutationMetadata"] = meta
    }

    return result
}
```

---

## 4. Execution & Response

### MutationResult Structure

Every mutation returns a consistent envelope:

**Success:**
```json
{
  "ok": true,
  "result": {
    "id": "task-42",
    "title": "Fix login bug",
    "status": "todo"
  }
}
```

**Validation error (structural — framework-level):**
```json
{
  "ok": false,
  "errors": [
    {"field": "title", "message": "required parameter \"title\" is missing", "code": "REQUIRED"}
  ]
}
```

**Domain error (handler-level):**
```json
{
  "ok": false,
  "errors": [
    {"field": "assignee", "message": "user \"nobody\" not found", "code": "NOT_FOUND"}
  ]
}
```

**Dry-run success:**
```json
{
  "ok": true,
  "result": {
    "dry_run": true,
    "would_create": {
      "title": "Fix login bug",
      "status": "todo"
    }
  }
}
```

### Error Handling: Two Layers

| Layer | Who | When | Shape |
|-------|-----|------|-------|
| Structural | Framework (wrapMutation) | Before handler call | `MutationResult{Ok: false, Errors: [...]}`— returns as data, not Go error |
| Domain | Handler | During handler execution | Handler returns `(nil, error)` — framework wraps into `MutationResult{Ok: false}` |

**Critical**: mutation errors are **always returned as data** (HTTP 200 equivalent), never as Go errors from `Query()`. This matches GraphQL's payload-error pattern and agentquery's existing batch-error-isolation pattern. The outer `Query()` function returns a Go error only for parse errors (malformed DSL).

### Batch Execution

Same semantics as read batches — per-statement error isolation:

```
create(title="A"); create(title="B"); delete(bad-id)
```

Result:
```json
[
  {"ok": true, "result": {"id": "task-42", "title": "A"}},
  {"ok": true, "result": {"id": "task-43", "title": "B"}},
  {"ok": false, "errors": [{"message": "task \"bad-id\" not found", "code": "NOT_FOUND"}]}
]
```

### Compact Output Format

Mutation results in compact mode:

**Single success:**
```
ok:true
id:task-42
title:Fix login bug
status:todo
```

**Single error:**
```
ok:false
error:required parameter "title" is missing (field:title, code:REQUIRED)
```

**Batch:**
```
ok:true
id:task-42
title:A

ok:true
id:task-43
title:B

ok:false
error:task "bad-id" not found (code:NOT_FOUND)
```

The `FormatCompact` function handles `MutationResult` as a `map[string]any` with special handling for the `ok` + `errors` keys.

### New Error Codes

```go
const (
    // Existing
    ErrParse      = "PARSE_ERROR"
    ErrNotFound   = "NOT_FOUND"
    ErrValidation = "VALIDATION_ERROR"
    ErrInternal   = "INTERNAL_ERROR"

    // New for mutations
    ErrConflict     = "CONFLICT"          // duplicate key, unique constraint violation
    ErrForbidden    = "FORBIDDEN"         // authorization failure
    ErrPrecondition = "PRECONDITION_FAILED" // optimistic concurrency check failed
    ErrRequired     = "REQUIRED"          // required parameter missing
    ErrInvalidValue = "INVALID_VALUE"     // enum/type mismatch
)
```

---

## 5. Cobra Integration

### Separate `m` / `mutate` Subcommand

Mutations get a dedicated `m` command, parallel to `q`. Reasons:

1. **Safety**: agents/scripts that use `q` for reads cannot accidentally invoke mutations
2. **Policy hook point**: `m` command can enforce `--confirm` for destructive ops
3. **Discoverability**: `--help` shows reads and writes separately
4. **Convention**: maps to `query` / `mutate` in GraphQL client vernacular

```
./taskdemo q  'list(status=done) { overview }' --format json     # read
./taskdemo m  'create(title="Fix bug")' --format json            # write
./taskdemo m  'delete(task-1)' --format json --confirm           # destructive write
./taskdemo m  'update(task-1, status=done)' --format json --dry-run  # dry-run
```

### MutateCommand Factory

```go
// MutateCommand creates an "m" subcommand that parses and executes a DSL
// mutation against the given schema. Same arg/flag pattern as QueryCommand.
// Additional flags: --dry-run, --confirm.
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

            // Inject dry_run into the query string if --dry-run flag is set
            // Alternative: pass via separate context (but query string is simpler)
            if dryRun {
                input = injectDryRun(input)
            }

            // Confirmation check for destructive mutations
            if !confirm && !dryRun {
                if needsConfirm(schema, input) {
                    return fmt.Errorf("destructive mutation requires --confirm flag (or use --dry-run to preview)")
                }
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
    cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the mutation without executing")
    cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destructive mutations")

    return cmd
}
```

### Updated AddCommands

```go
// AddCommands adds the "q", "grep", and "m" commands as subcommands of parent.
func AddCommands[T any](parent *cobra.Command, schema *agentquery.Schema[T]) {
    parent.AddCommand(QueryCommand(schema))
    parent.AddCommand(SearchCommand(schema))
    parent.AddCommand(MutateCommand(schema))
}
```

### Backwards Compatibility

`AddCommands` adds the `m` command only if the schema has registered mutations. If no mutations are registered, `m` is not added — existing read-only CLIs see no change:

```go
func AddCommands[T any](parent *cobra.Command, schema *agentquery.Schema[T]) {
    parent.AddCommand(QueryCommand(schema))
    parent.AddCommand(SearchCommand(schema))
    if schema.HasMutations() {
        parent.AddCommand(MutateCommand(schema))
    }
}
```

### Can Mutations Run via `q`?

**Yes, but without safety flags.** Mutations registered via `Mutation()` are also in the `operations` map (via `wrapMutation`), so `q 'create(title="X")'` works. The `m` command adds the safety layer (`--confirm`, `--dry-run`). This is intentional — programmatic callers who know what they're doing can use `q`, while interactive/agent callers use `m` for safety.

---

## 6. Parser Changes

### Decision: Zero Parser Changes

The current grammar already covers mutations:

```
batch     = query (";" query)*
query     = operation "(" params ")" [ "{" fields "}" ]
params    = param ("," param)*
param     = key "=" value | value
fields    = identifier+
```

Mutations use this grammar exactly:
- `create(title="Fix bug")` — operation with key=value args
- `update(task-1, status=done)` — operation with positional arg + key=value args
- `delete(task-1)` — operation with positional arg
- `delete(task-1, dry_run=true)` — with framework arg

The operation name is validated against the combined operations+mutations map via `ParserConfig.Operations`. No new tokens, no grammar extensions.

### Why This Works

1. **Identifier rules** already accept hyphenated names (`update-task`, `mark-done`)
2. **Quoted strings** handle values with spaces (`title="Fix login bug"`)
3. **Key=value args** handle all mutation parameters
4. **Positional args** handle target IDs (`delete(task-1)`)
5. **Semicolons** handle batched mutations
6. **Braces** are syntactically allowed (parser doesn't reject them) — mutation handlers just ignore them

### parserConfig Update

The only change: `parserConfig()` builds the operations map from both `s.operations` and `s.mutations`:

```go
func (s *Schema[T]) parserConfig() *ParserConfig {
    ops := make(map[string]bool, len(s.operations)+len(s.mutations))
    for name := range s.operations {
        ops[name] = true
    }
    // mutations are already in s.operations via wrapMutation,
    // so no additional loop needed
    return &ParserConfig{
        Operations:    ops,
        FieldResolver: s,
    }
}
```

Since `Mutation()` already adds to `s.operations` via `wrapMutation`, the parser config doesn't need any changes either.

---

## 7. Implementation Plan

### Phase 1: Core Types and Schema Registration

**Files**: `agentquery/mutation.go` (new), `agentquery/types.go` (extend)

1. **Add new types** to `types.go`:
   - `MutationHandler[T]` type alias
   - `MutationContext[T]` struct
   - `MutationResult` struct
   - `MutationError` struct
   - `MutationMetadata` struct
   - New error code constants (`ErrConflict`, `ErrForbidden`, `ErrPrecondition`, `ErrRequired`, `ErrInvalidValue`)

2. **Create `mutation.go`** with:
   - `Mutation()` method on Schema
   - `MutationWithMetadata()` method on Schema
   - `wrapMutation()` internal method
   - `validateArgs()` framework validation
   - `HasMutations()` method on Schema
   - Add `mutations` and `mutationMetadata` maps to Schema struct
   - Initialize the new maps in `NewSchema()`

3. **Extend `ParameterDef`** in `types.go`:
   - Add `Enum []string` field
   - Add `Required bool` field

**Complexity**: Medium. Core plumbing, no tricky parts.
**Tests**: `mutation_test.go` — register mutations, call `Query()`, verify `MutationResult` shape.

### Phase 2: Schema Introspection

**Files**: `agentquery/schema.go` (modify `introspect()`)

4. **Extend `introspect()`** to include `mutations` and `mutationMetadata` sections.

**Complexity**: Low. Follow existing pattern for `operationMetadata`.
**Tests**: `schema_test.go` — register mutations, call `schema()`, verify JSON output includes `mutations` and `mutationMetadata`.

### Phase 3: Cobra Integration

**Files**: `agentquery/cobraext/command.go` (extend)

5. **Add `MutateCommand()`** factory with `--dry-run` and `--confirm` flags.
6. **Add `needsConfirm()`** helper that checks `MutationMetadata.Destructive`.
7. **Add `injectDryRun()`** helper (or use `MutateWithMode()` method on Schema).
8. **Update `AddCommands()`** to conditionally include `m` command.

**Complexity**: Low-Medium. Similar to existing `QueryCommand`.
**Tests**: `command_test.go` — execute mutations via Cobra command, verify output and flag behavior.

### Phase 4: Example Update

**Files**: `example/main.go` (extend)

9. **Add mutation registrations** (`create`, `update`, `delete`) with handlers.
10. **Add in-memory store** (replace `sampleTasks()` with a mutable store for mutations to work on).

**Complexity**: Low. Demonstration code.
**Tests**: Manual (run example CLI).

### Phase 5: Documentation

**Files**: `CLAUDE.md` (update), `README.md` (update if exists), assets/ (update reference implementations)

11. **Update `CLAUDE.md`** with mutation DSL examples, new types, new methods.
12. **Update reference assets** with mutation patterns.

**Complexity**: Low.

### Dependency Graph

```
Phase 1 (types + registration)
  └─> Phase 2 (introspection)
  └─> Phase 3 (cobra) — depends on Phase 1
  └─> Phase 4 (example) — depends on Phase 1 + Phase 3
Phase 5 (docs) — depends on all above
```

Phases 2 and 3 can be done in parallel after Phase 1.

---

## 8. Open Questions

### Q1: Should `ParameterDef.Required` replace `Optional` or coexist?

**Proposal**: Coexist. `Optional` is already in use for read operations. Adding `Required` is additive. Introspection can emit both. But we should document the convention: for read operations use `Optional`, for mutation parameters use `Required`.

**Alternative**: Deprecate `Optional` in favor of `Required` everywhere. Breaking change for existing metadata consumers.

### Q2: Should `MutationContext` include `Predicate` (like `OperationContext`)?

**Proposal**: No. Mutations target specific entities by ID, not by filter. If a mutation needs to find items, it uses `Items()` and searches manually. If bulk mutations with filters are needed later, `Predicate` can be added then.

**Alternative**: Include `Predicate` for bulk mutations like `delete-all(status=done)`.

### Q3: Should dry-run be framework-level or handler-level?

**Proposal**: Both. The framework sets `ctx.DryRun = true` and the handler decides what to do with it. For handlers that don't check `DryRun`, the framework does NOT automatically prevent execution — this is the handler's responsibility. The framework provides the flag; the handler respects it.

**Alternative**: Framework intercepts execution — handler is never called in dry-run mode. But this prevents the handler from computing "what would happen" previews.

### Q4: Should the `m` command be the only way to invoke mutations?

**Proposal**: No. Mutations are also invocable via `q` (since they're in the operations map). The `m` command adds safety (`--confirm`, `--dry-run`). Power users and scripts can use `q` directly if they want.

**Alternative**: Strict separation — `q` rejects mutation operation names. Requires parser to distinguish reads from writes, which contradicts the "zero parser changes" constraint.

### Q5: Transaction/atomic batch support?

**Proposal**: Out of scope for v1. Independent batch execution (matching existing semantics). Handlers can implement internal transactions if needed. If demand arises, add `--atomic` flag in a future version that wraps all mutations in a handler-provided transaction callback.

### Q6: Should mutation responses support field projection?

**Proposal**: No for v1. Mutation responses are always complete. The result shape is decided by the handler (usually the affected entity). Keeps things simple. If agents request it, add projection later — the `{ fields }` syntax is already parsed, handlers just ignore it.

---

## Design Summary

| Aspect | Decision |
|--------|----------|
| **Syntax** | Same `operation(args)` grammar, zero parser changes |
| **Registration** | `Mutation()` / `MutationWithMetadata()` on Schema |
| **Handler type** | `MutationHandler[T]` with `MutationContext[T]` |
| **Response shape** | `MutationResult{Ok, Result, Errors}` — always data, never Go error |
| **Introspection** | Separate `mutations` + `mutationMetadata` sections in `schema()` |
| **Cobra** | New `m` command with `--dry-run` and `--confirm` |
| **Batching** | Mixed batches allowed, sequential, per-statement error isolation |
| **Validation** | Two-layer: framework (required/enum) + handler (domain) |
| **Safety** | `Destructive` flag in metadata, `--confirm` required for destructive mutations |
| **Backward compat** | Fully backward compatible — read-only schemas unaffected |

---

## Sources

- `.research/260218_graphql_mutations.md` — GraphQL mutations deep-dive
- `.research/260218_alternative_mutation_patterns.md` — CQRS, Hasura/PostgREST, tRPC, Firestore, CLI tools, Agent APIs
- `agentquery/` source code — existing patterns for operations, fields, presets, filters, sort, cobra integration
