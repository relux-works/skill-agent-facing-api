# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

A Go library (`agentquery`) that provides a generic, reusable query layer for building agent-optimized CLI tools. It implements the "agent-facing API" design pattern: a mini-query DSL for structured reads with field projection/batching, plus scoped grep for full-text search. The repo also serves as a skill for AI coding agents (the `SKILL.md` documents the pattern itself).

## Build & Test

The Go module lives in `agentquery/` (not at the repo root — the root is a skill, not a Go module).

```bash
# Run all tests
cd agentquery && go test ./...

# Run a single test
cd agentquery && go test -run TestParseSingle ./...

# Run tests with coverage
cd agentquery && go test -cover ./...

# Run only cobraext tests
cd agentquery && go test ./cobraext/...

# Build the example CLI
cd example && go build -o taskdemo .

# Run the example (--format is required: "json" or "compact"/"llm")
./example/taskdemo q 'schema()' --format json
./example/taskdemo q 'summary()' --format json
./example/taskdemo q 'count()' --format json
./example/taskdemo q 'count(status=done)' --format json
./example/taskdemo grep "TODO" --format json

# Pagination
./example/taskdemo q 'list(skip=2, take=3) { overview }' --format json
./example/taskdemo q 'list(status=done, skip=0, take=2) { minimal }' --format json

# Sorting
./example/taskdemo q 'list(sort_name=asc) { overview }' --format json
./example/taskdemo q 'list(sort_priority=desc, sort_name=asc) { overview }' --format json

# Distinct values for a filterable field
./example/taskdemo q 'distinct(status)' --format json

# Compact output for LLM agents
./example/taskdemo q 'list() { overview }' --format compact
./example/taskdemo q 'get(task-1) { overview }' --format compact
./example/taskdemo grep "TODO" --format compact

# Mutations (write operations)
./example/taskdemo m 'create(title="Fix bug", status=todo)' --format json
./example/taskdemo m 'update(task-1, status=done)' --format json
./example/taskdemo m 'delete(task-1)' --format json --confirm
./example/taskdemo m 'delete(task-1)' --format json --dry-run
./example/taskdemo m 'create(title="Test")' --format compact
```

No linter is configured. No CI/CD pipeline exists.

## Architecture

### Library (`agentquery/`)

The core is a generic `Schema[T]` type parameterized on the domain item type. Users register fields, presets, operations, and a data loader on the schema, then call `Query()` or `Search()`.

**Query data flow:** Input string → `Parse()` (tokenizer + recursive descent) → `Query` AST → `Schema.executeStatement()` → `OperationHandler` receives `OperationContext` with parsed statement, `FieldSelector`, and lazy item loader → returns JSON-serializable result → serialized as JSON (default) or compact tabular (via `--format compact` or `*WithMode()` API).

**Mutation data flow:** Input string → `Parse()` → `Statement` → `wrapMutation()` builds `MutationContext` (`ArgMap`, `DryRun`) → framework validates (required/enum from metadata) → `MutationHandler` returns `(result, error)` → wrapped into `MutationResult{Ok, Result, Errors}` → serialized as JSON or compact.

Key files in `agentquery/`:

- **`schema.go`** — Central `Schema[T]` type. Registers fields, presets, operations, mutations, loader. `OperationWithMetadata()` registers operations with metadata (parameters, examples) for schema introspection. Built-in `schema` operation returns operations, fields, presets, defaults, `operationMetadata` (when present), `filterableFields`, `sortableFields`, `mutations`, and `mutationMetadata`. `SortField()` registers a comparator, `SortFields()` exposes the map for `SortSlice()`. `QueryJSONWithMode()` and `SearchJSONWithMode()` for caller-specified output format.
- **`parser.go`** — Tokenizer + recursive descent parser. Grammar: `operation(params) { fields }` with `;` batching. Validates operations and resolves fields (including preset expansion) at parse time via `ParserConfig`.
- **`selector.go`** — `FieldSelector[T]` applies field projection to domain items. Created internally by Schema from parsed field lists. Lazy — only calls accessors for selected fields. `ApplyValues()` returns ordered values without keys (used by compact formatter).
- **`query.go`** — `Schema.Query()` and `QueryJSON()`. Executes parsed statements, handles batching (single result unwrapped, multiple → `[]any`). Per-statement errors don't abort the batch. `formatLLMReadable()` handles compact output for single and batch queries.
- **`format.go`** — `FormatCompact()` tabular formatter. Lists → CSV-style header + value rows. Single objects → key:value pairs. Falls back to JSON for complex/error types. Handles CSV escaping and value formatting.
- **`search.go`** — `Search()` does recursive regex search with file extension filtering, glob filtering, case-insensitive flag, and context lines. Independent of Schema but also available as `Schema.Search()`. `FormatSearchCompact()` produces grouped-by-file text output.
- **`ast.go`** — AST types: `Query`, `Statement`, `Arg`.
- **`mutation.go`** — Mutation registration (`Mutation()`, `MutationWithMetadata()`), `wrapMutation()` adapter (MutationHandler → OperationHandler bridge), framework-level validation (`validateMutationArgs` — required params, enum constraints), `DryRun` support. `HasMutations()` and `IsMutationDestructive()` for cobra safety layer.
- **`types.go`** — Shared types: `FieldAccessor[T]`, `OperationHandler[T]`, `OperationContext[T]` (includes `Predicate` — auto-built from filterable fields), `SearchResult`, `SearchOptions`, `OutputMode` (`HumanReadable`, `LLMReadable`), `ParameterDef` (includes `Enum`, `Required`), `OperationMetadata`, `SortComparator[T]`, `SortSpec`, `SortDirection` (`Asc`, `Desc`), `MutationHandler[T]`, `MutationContext[T]` (`ArgMap`, `DryRun`, `PositionalArg()`, `RequireArg()`, `ArgDefault()`), `MutationResult` (`Ok`, `Result`, `Errors`), `MutationError`, `MutationMetadata` (`Destructive`, `Idempotent`).
- **`helpers.go`** — Generic convenience functions for operation handlers: `FilterItems[T]` (filter by predicate), `CountItems[T]` (count matching items without allocating), `MatchAll[T]` (default predicate), `Distinct[T]` (unique values in first-seen order), `DistinctCount[T]` (count per unique value), `GroupBy[T]` (group items by key function).
- **`filter.go`** — `FilterableField[T]()` registers a string accessor for case-insensitive equality filtering. Schema auto-builds `ctx.Predicate` from registered filters + query args. On first registration, auto-registers a built-in `distinct` operation: `distinct(field_name)` → unique values.
- **`sort.go`** — `SortFieldOf[T,V]()` creates a comparator from a `cmp.Ordered` accessor. `SortableField[T,V]()` and `SortableFieldFunc[T]()` register sort fields on Schema. `ParseSortSpecs()` extracts `sort_<field>=asc|desc` from args. `BuildSortFunc()` chains multi-field comparators. `SortSlice()` is the one-liner for operation handlers.
- **`paginate.go`** — `PaginateSlice[T]` extracts `skip`/`take` from args and slices items. `ParseSkipTake` for manual control. Conventions: skip defaults to 0, take=0 means no limit.
- **`error.go`** — `ParseError` (syntax) and `Error` (runtime, with code/message/details). Mutation-specific error codes: `ErrConflict`, `ErrForbidden`, `ErrPrecondition`, `ErrRequired`, `ErrInvalidValue`.
- **`cobraext/command.go`** — Cobra command factories (`QueryCommand`, `SearchCommand`, `MutateCommand`, `AddCommands`). Isolated sub-package so non-Cobra users don't import it. `--format` flag on all commands (`json` default, `compact`/`llm` for token-efficient output). `MutateCommand` adds `--dry-run` and `--confirm` safety flags. `AddCommands` conditionally adds `m` subcommand only when mutations are registered.

### Example (`example/`)

Separate Go module (`example/go.mod`) with a `taskdemo` CLI that wires up a `Task` domain type against the library. Shows how to register fields, presets, operations (`get`, `list`, `count`, `summary`) with metadata, mutations (`create`, `update`, `delete`) with mutation metadata, and use `cobraext.AddCommands`. Mutation handlers demonstrate `ctx.PositionalArg()`, `ctx.RequireArg()`, and `ctx.ArgDefault()` convenience methods.

### Assets (`assets/`)

Reference implementations (standalone `.go` files with `// ADAPT THIS` markers) and a query patterns catalog. These are documentation/templates, not compiled code — they're not part of the `agentquery` module.

## DSL Grammar

```
batch     = query (";" query)*
query     = operation "(" params ")" [ "{" fields "}" ]
params    = param ("," param)*
param     = key "=" value | value
fields    = identifier+
```

Identifiers: letters, digits, underscore, hyphen. Values can also be quoted strings (`"..."`).

**Conventions (no grammar changes needed):**
- **Filter args**: any `key=value` where `key` is a registered filterable field. Case-insensitive equality. Multiple filters are AND-ed. Auto-injected into `ctx.Predicate`.
- **Sort args**: `sort_<field>=asc|desc` (e.g. `sort_name=asc, sort_priority=desc`). Parsed by `SortSlice()` in handlers.
- **Distinct operation**: `distinct(field_name)` — auto-registered when any `FilterableField` is registered. Returns unique values for the field.
- **Mutations**: use the same grammar as queries. No new syntax. Mutation operations are registered separately via `Mutation()`/`MutationWithMetadata()` but parsed identically. Convention: `dry_run=true` arg for preview mode, positional first arg as target ID for update/delete.

## Key Design Decisions

- **Generics over interfaces**: `Schema[T]` is parameterized on the domain type. Field accessors are `func(T) any`, not reflection-based.
- **Parse-time validation**: Operations and fields are validated during parsing (not execution), producing early errors with position info.
- **Preset expansion at parse time**: Presets (like `overview`) expand to field lists in the parser via `FieldResolver` interface (implemented by Schema).
- **Lazy item loading**: `OperationContext.Items` is a `func() ([]T, error)` — only called if the operation needs the dataset.
- **Batch error isolation**: In a batch query `a(); b(); c()`, if `b()` fails, `a()` and `c()` still return results. The error is inlined as `{"error": {"message": "..."}}`.
- **cobraext is optional**: The Cobra integration lives in a sub-package. Core library has zero dependencies beyond stdlib.
- **Operation metadata is optional and backwards compatible**: `OperationWithMetadata()` adds parameter/example metadata for schema introspection. Plain `Operation()` still works — those operations just won't have metadata in `schema()` output. No migration needed.
- **Pagination and counting are library helpers, not framework mandates**: `PaginateSlice`, `FilterItems`, `CountItems` are generic convenience functions that operation handlers opt into. The framework doesn't enforce pagination — handlers decide if/how to use these.
- **Output format is a transport concern, not a schema concern**: Schema stays format-agnostic. `QueryJSON()` / `SearchJSON()` always produce JSON. `QueryJSONWithMode()` / `SearchJSONWithMode()` accept an explicit `OutputMode` for callers that need compact output. The `--format` CLI flag (`json` default, `compact`/`llm`) puts the format decision where it belongs — at the call site. Same CLI tool serves humans (JSON), TUI apps (JSON), and LLM agents (`--format compact`).
- **FilterableField is a standalone function, not a Schema method**: Go methods cannot introduce additional type parameters beyond the receiver's. `FilterableField[T]()` needs only `T`, but `SortableField[T,V]()` needs `V cmp.Ordered` too. Package-level functions sidestep this constraint.
- **ctx.Predicate is auto-injected**: Schema builds a predicate from registered filterable fields and query args before calling the handler. Handlers use `ctx.Predicate` directly — no manual arg parsing for filters. Defaults to `MatchAll` when no args match registered filters.
- **Sort uses `sort_<field>=asc|desc` arg convention**: Zero parser changes — sort directives piggyback on the existing `key=value` param syntax. `ParseSortSpecs()` extracts them by the `sort_` prefix. Multi-field sort uses first-non-zero chaining.
- **distinct is auto-registered via FilterableField**: The first `FilterableField()` call auto-registers a built-in `distinct` operation with full metadata. No manual registration needed. Keeps the filter and distinct APIs coupled — if a field is filterable, you can always get its distinct values.
- **Filter accessors are string-only**: `FilterableField` takes `func(T) string`, not `func(T) any`. Case-insensitive string equality (via `strings.EqualFold`) covers 95%+ of agent filter use-cases (status, assignee, type, priority). Avoids type-switching complexity in the predicate builder.
- **Mutations use the same parser and grammar as queries**: Zero parser changes. Operation name determines read vs write. `Mutation()` registers in both `mutations` map (for metadata/introspection) and `operations` map (for parser validation/dispatch).
- **MutationHandler wraps into OperationHandler via wrapMutation adapter**: The adapter builds `MutationContext` (`ArgMap`, `DryRun`), runs framework validation, calls the handler, and wraps the result into `MutationResult`. Mutations dispatch through the same `executeStatement()` path as queries.
- **Mutation errors are data, not Go errors**: Handler errors are wrapped into `MutationResult{Ok: false, Errors: [...]}` and returned from `Query()` as a normal value. Consistent with batch error isolation — a failing mutation in a batch doesn't abort sibling statements.
- **Two-layer mutation validation**: Framework (Layer 1) validates required params and enum constraints from `MutationMetadata` before calling the handler. Domain (Layer 2) validation belongs in the handler itself. If metadata isn't registered, only domain validation runs.
- **MutateCommand (`m`) adds CLI safety layer**: `--confirm` required for destructive mutations, `--dry-run` for preview. Mutations are also accessible via `q` without safety flags (for programmatic callers who handle safety themselves).
- **Schema introspection separates reads and writes**: `introspect()` includes separate `mutations` list and `mutationMetadata` map alongside `operations` and `operationMetadata`. Agents clearly see the read/write boundary.
- **MutationMetadata includes Destructive and Idempotent flags**: Follows the MCP tool annotations pattern. Agents use these for safety decisions — confirm before destructive, skip confirm for idempotent.
