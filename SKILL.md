---
name: agent-facing-api
description: Design pattern for building agent-optimized CLI query layers. Two-layer approach — mini-query DSL for structured reads (field projection, batching) + scoped grep for full-text search. Minimizes token overhead, eliminates MCP session cost.
triggers:
  - agent API design
  - query layer for agents
  - agent-facing CLI
  - DSL for agent reads
  - building tools for agents
  - agent query interface
  - token-efficient API
---

# Agent-Facing API Pattern

A design pattern for building CLI tools that AI agents use efficiently. Three layers — structured DSL for reads, scoped grep for full-text search, DSL mutations for writes — all through one CLI binary.

---

## When to Use

- Building a CLI tool that agents will query (task boards, config managers, data stores, etc.)
- Existing CLI has verbose human-facing output that wastes agent context
- Need to minimize tokens per query without adding infrastructure (MCP servers, HTTP APIs)
- Want batch queries in a single tool call

---

## The Pattern

```
┌─────────────────────────────────────────────┐
│                Agent Context                │
│                                             │
│  Structured read ──► q   (query DSL)        │
│  Text search     ──► grep (scoped ripgrep)  │
│  Write/mutate    ──► m   (mutation DSL)      │
└─────────────────────────────────────────────┘
```

Three access modes, one CLI binary. No extra processes, no session overhead, no tool definitions beyond the Bash tool the agent already has.

### Why not MCP?

MCP loads tool definitions into the agent's context at session start. For a typical tool with 10-15 operations, that's ~2,000-3,000 tokens of dead weight per session. The DSL approach costs zero — the agent calls Bash with a query string.

Per-query, MCP and DSL produce identical output when backed by the same field selection engine. But DSL supports batching (multiple queries in one call), while MCP requires separate tool calls. MCP never breaks even for typical agent sessions.

See [references/comparison-example.md](references/comparison-example.md) for a real-world measurement on a 346-element board.

---

## Layer 1: Query DSL (Reads)

The primary read interface for agents. A single `q` subcommand that accepts a compact query string and returns structured JSON.

### Design Principles

1. **Single entry point** — one subcommand (e.g., `mytool q '<query>'`)
2. **Operations map to read use-cases** — not CRUD, just the queries agents actually need
3. **Field projection** — agent requests only the fields it needs (`{ id status }`)
4. **Field presets** — named bundles for common patterns (`{ overview }`, `{ full }`)
5. **Batching** — semicolons separate queries, one tool call returns all results
6. **JSON output** — always, no flags needed (agents parse JSON, humans use CLI)
7. **Compact by default** — minimal preset returns the absolute minimum useful data

### Syntax Template

```
<operation>(<params>) { <fields or preset> }
```

- **Operation** — what to query: `get`, `list`, `count`, `summary`, `agents`, etc.
- **Params** — filters: `type=task, status=done`, pagination: `skip=10, take=5`, sorting: `sort_name=asc, sort_priority=desc`, an ID, etc.
- **Fields** — explicit: `{ id name status }` or preset: `{ overview }`
- **Omitted fields** — default to a sensible minimal set

### Implementation Checklist

When adding a DSL to an existing CLI:

- [ ] Define 3-6 operations covering the core read use-cases
- [ ] Implement field projection (whitelist of known fields per element type)
- [ ] Create 3-4 field presets (`minimal`, `default`, `overview`, `full`)
- [ ] Support batching via `;` separator
- [ ] Always output JSON (no `--json` flag needed — DSL is agent-only)
- [ ] Parse the query string in the CLI, not via shell eval (security)
- [ ] Error responses as JSON too: `{"error": "element not found"}`
- [ ] Register operation metadata (parameters, examples) for schema introspection — agents discover the API via `schema()` without external docs
- [ ] Register filterable fields (`FilterableField`) for declarative predicate building and auto `distinct` operation
- [ ] Register sortable fields (`SortableField`) for `sort_<field>=asc|desc` arg convention

### Example: Task Board DSL

```bash
# Single element
mytool q 'get(TASK-42) { status assignee }'
# → {"id":"TASK-42","status":"development","assignee":"agent-auth"}

# Filtered list with preset
mytool q 'list(type=task, status=development) { overview }'
# → [{"id":"TASK-42","name":"jwt-tokens","status":"development","assignee":"agent-auth"}, ...]

# Batch — three lookups in one call
mytool q 'get(T1) { status }; get(T2) { status }; get(T3) { status }'
# → [{"id":"T1","status":"done"}, {"id":"T2","status":"development"}, {"id":"T3","status":"blocked"}]

# Paginated list — skip first 10, return next 5
mytool q 'list(type=task, skip=10, take=5) { overview }'
# → [{"id":"TASK-53","name":"...","status":"todo","assignee":"agent-ui"}, ...]

# Pagination with filters
mytool q 'list(status=done, skip=0, take=3) { minimal }'
# → [{"id":"TASK-03","status":"done"}, {"id":"TASK-07","status":"done"}, {"id":"TASK-12","status":"done"}]

# Count (no field projection needed)
mytool q 'count()'
# → {"count": 48}

# Count with filter
mytool q 'count(status=done)'
# → {"count": 31}

# Sorted list — multi-field sort
mytool q 'list(sort_priority=desc, sort_name=asc) { overview }'
# → [{...priority:high, name:"A"...}, {priority:high, name:"B"...}, {priority:low, ...}]

# Distinct values for a filterable field
mytool q 'distinct(status)'
# → ["todo", "in-progress", "done", "blocked"]

# Summary (no field projection needed)
mytool q 'summary()'
# → {"epics":5,"stories":12,"tasks":48,"done":31,"in_progress":10,"blocked":2}
```

### Schema Introspection

Every schema has a built-in `schema()` operation. When operations are registered with `OperationWithMetadata`, the introspection output includes `operationMetadata` — parameter definitions (name, type, optional, default) and usage examples per operation. Agents call `schema()` once to discover the full API contract:

```bash
mytool q 'schema()'
```
```json
{
  "operations": ["count", "distinct", "get", "list", "schema", "summary"],
  "fields": ["id", "name", "status", "assignee", "description"],
  "presets": {"minimal": ["id", "status"], "overview": ["id", "name", "status", "assignee"]},
  "defaultFields": ["default"],
  "filterableFields": ["status", "assignee"],
  "sortableFields": ["name", "status", "priority"],
  "operationMetadata": {
    "list": {
      "description": "List tasks with optional filters and pagination",
      "parameters": [
        {"name": "status", "type": "string", "optional": true},
        {"name": "skip", "type": "int", "optional": true, "default": 0, "description": "Skip first N items"},
        {"name": "take", "type": "int", "optional": true, "description": "Return at most N items"}
      ],
      "examples": ["list() { overview }", "list(status=done, skip=0, take=2) { overview }"]
    },
    "count": {
      "description": "Count tasks matching optional filters",
      "parameters": [
        {"name": "status", "type": "string", "optional": true}
      ],
      "examples": ["count()", "count(status=done)"]
    },
    "distinct": {
      "description": "Returns unique values for a filterable field.",
      "parameters": [
        {"name": "field", "type": "string", "optional": false, "description": "Name of a registered filterable field."}
      ],
      "examples": ["distinct(status)", "distinct(assignee)"]
    }
  }
}
```

Operation metadata is optional and backwards compatible — operations registered with plain `Operation()` still work, they just won't appear in `operationMetadata`.

### Output Modes

The `--format` flag is **required** on CLI commands — the caller explicitly chooses the output format:

| Flag | Mode | Format | Use case |
|------|------|--------|----------|
| `--format json` | `HumanReadable` | Standard JSON | Human inspection, TUI apps, piping to `jq` |
| `--format compact` | `LLMReadable` | Tabular text | Agent consumption — fewer tokens |
| `--format llm` | `LLMReadable` | (alias for compact) | Same as above |

Format is a **transport concern**, not a schema setting. The same CLI tool serves different consumers:

```bash
# Agent calls with compact format
mytool q 'list(status=todo) { overview }' --format compact

# TUI app or human calls with JSON
mytool q 'list()' --format json
```

Programmatically, use `QueryJSONWithMode()` / `SearchJSONWithMode()` to specify format per-call. `QueryJSON()` / `SearchJSON()` always return JSON.

**Compact format examples:**

List queries produce CSV-style output (header + rows):
```
id,name,status,assignee
task-1,Auth service refactor,in-progress,alice
task-2,Dashboard performance,todo,bob
```

Single-element queries produce key:value pairs:
```
id:task-1
name:Auth service refactor
status:in-progress
```

Search results are grouped by file:
```
README.md
  3: matching line
  4  context line
other.md
  12: another match
```

### Token Budget

Typical per-query costs (input + output + ~80 tok framing):

| Query type | Tokens | Notes |
|------------|--------|-------|
| Element lookup (minimal) | ~110 | ID + 2-3 fields |
| Element lookup (full) | ~380 | All fields |
| Filtered list (overview) | ~150-300 | Scales with result count |
| Paginated list (take=5) | ~150-200 | Bounded by take param |
| Count | ~90 | Fixed size: `{"count": N}` |
| Summary | ~450 | Fixed size, scales with board |
| Batch of 3 (status only) | ~140 | Single call, 3 results |

Compare to CLI text output for the same data: 1.5-5x more tokens (ANSI codes, formatting, verbose labels).

---

## Layer 2: Full-Text Search

Full-text search across the data store. For when the agent needs to find content by text pattern rather than by structured fields.

The library provides a `SearchProvider` interface — consumers implement search however they want (filesystem walk, database FTS, external search service). A built-in `FileSystemSearchProvider` handles the common case of searching `.md` files in a directory.

### Design Principles

1. **Scoped to the data source** — don't search the whole filesystem
2. **Regex support** — agents build patterns dynamically
3. **File filter** — narrow search to specific file types (`--file progress.md`)
4. **Case-insensitive flag** — `-i`
5. **Context lines** — `-C N` for surrounding context
6. **Pluggable backend** — implement `SearchProvider` interface for custom search

### Implementation Checklist

- [ ] Implement `SearchProvider` interface or use built-in `FileSystemSearchProvider`
- [ ] Subcommand: `mytool grep <pattern>` (auto-wired by `cobraext.AddCommands`)
- [ ] Scope: only search within the tool's data source
- [ ] Flags: `--file <glob>`, `-i`, `-C <N>`, `--format`
- [ ] Support both JSON and compact output (grouped-by-file text)

### When to Use Grep (vs DSL)

| Signal | Use |
|--------|-----|
| "Find elements mentioning X" | Grep |
| "Who has notes about deployment?" | Grep: `mytool grep "deployment" --file progress.md` |
| "What's the status of TASK-42?" | DSL: `mytool q 'get(TASK-42) { status }'` |
| "List all blocked tasks" | DSL: `mytool q 'list(status=blocked) { overview }'` |
| "Search for TODO comments" | Grep: `mytool grep "TODO" -i` |

### Token Budget

Grep output scales with matches. A narrow search (specific term + file filter) is cheap (~100-300 tokens). A broad search (common term, no filter) can return thousands of lines — catastrophically expensive.

**Rule of thumb:** always combine with `--file` filter when possible.

---

## Layer 3: Mutations (Writes)

Writes use the same DSL grammar as reads, accessed through a separate `m` subcommand with safety flags:

```bash
# Create
mytool m 'create(title="Fix bug", status=todo)' --format json

# Update (positional ID + named params)
mytool m 'update(item-1, status=done)' --format json

# Delete (destructive — requires --confirm)
mytool m 'delete(item-1)' --format json --confirm

# Preview without applying (dry run)
mytool m 'delete(item-1)' --format json --dry-run

# Batch mutations
mytool m 'update(item-1, status=done); update(item-2, status=done)' --format json
```

### Safety Flags

- `--confirm` — required for mutations marked `Destructive: true` in metadata
- `--dry-run` — injects `dry_run=true`; handler returns a preview without applying changes
- Non-destructive mutations need neither flag

### Mutation Registration

Mutations are registered via `Mutation()` or `MutationWithMetadata()`. Metadata provides parameter definitions, examples, and safety annotations for schema introspection:

```go
schema.MutationWithMetadata("update", handler, agentquery.MutationMetadata{
    Description: "Update item fields by ID",
    Parameters: []agentquery.ParameterDef{
        {Name: "id", Type: "string", Required: true, Description: "Item ID (positional)"},
        {Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}},
    },
    Destructive: false,
    Idempotent:  true,
    Examples:    []string{`update(item-1, status=done)`},
})
```

### MutationContext Convenience Methods

Handlers receive `MutationContext` with built-in helpers to reduce boilerplate:

```go
func myHandler(ctx agentquery.MutationContext[Item]) (any, error) {
    // First positional (keyless) arg — e.g. "item-1" from "update(item-1, ...)"
    id := ctx.PositionalArg()

    // Named arg with error if missing/empty
    status, err := ctx.RequireArg("status")

    // Named arg with default value
    priority := ctx.ArgDefault("priority", "medium")

    // Full arg map and dry-run flag also available
    _ = ctx.ArgMap["custom_field"]
    _ = ctx.DryRun
}
```

### Mutation Validation (Two Layers)

1. **Framework (Layer 1)** — validates required params and enum constraints from `MutationMetadata` before calling the handler. Automatic, no code needed.
2. **Domain (Layer 2)** — business logic validation inside the handler itself (e.g., "cannot delete item with children").

### Schema Introspection with Mutations

`schema()` output includes separate `mutations` and `mutationMetadata` sections alongside read operations:

```json
{
  "operations": ["count", "get", "list", "schema", "summary"],
  "mutations": ["create", "delete", "update"],
  "mutationMetadata": {
    "delete": {
      "description": "Delete an item by ID",
      "destructive": true,
      "idempotent": true,
      "parameters": [{"name": "id", "type": "string", "required": true}]
    }
  }
}
```

Agents clearly see the read/write boundary.

### Implementation Checklist (Mutations)

- [ ] Register mutations with metadata (description, parameters, examples, destructive/idempotent flags)
- [ ] Use `ctx.PositionalArg()` for ID-based mutations
- [ ] Use `ctx.RequireArg()` / `ctx.ArgDefault()` to reduce boilerplate
- [ ] Handle `ctx.DryRun` — return preview without side effects
- [ ] Mark destructive mutations (`Destructive: true`) — CLI enforces `--confirm`
- [ ] Error responses as `MutationResult{Ok: false, Errors: [...]}`
- [ ] Use `cobraext.AddCommands()` to auto-wire `q`, `grep`, and `m` subcommands

---

## Decision Table

```
Need data from the tool?
  Structured query (status, filter, lookup)?  ──► DSL (q subcommand)
  Text search across content?                 ──► Grep
  Mutation (create, update, delete)?          ──► DSL (m subcommand)
  Human reading the output?                   ──► CLI command (text + colors)
```

---

## Anti-Patterns

| Anti-Pattern | Why it's bad | Fix |
|-------------|-------------|-----|
| MCP server for simple read queries | ~2,200 tok session overhead, no batching | DSL subcommand |
| Returning all fields by default | Agent pays for data it ignores | Field projection with compact default |
| No batching support | 3 lookups = 3 tool calls = 3x framing overhead | Semicolon-separated queries |
| Grep for structured queries | Unreliable, returns raw file content, can return 10K+ tokens | DSL for anything with known fields |
| CLI `--json` flag on every command | Still returns all fields, verbose JSON | Dedicated DSL with projection |
| Human-formatted output to agents | ANSI codes, alignment padding, box drawing = wasted tokens | JSON-only DSL layer |
| JSON output to LLM agents | JSON keys repeated per item, quoting overhead | Use `--format compact` — tabular format with schema-once header |

---

## Implementation Guide

### Recommended Stack

- **Language:** Go (single binary, fast startup, good for CLI tools)
- **Library:** `agentquery` (this repo — generic Schema[T] with fields, operations, mutations, search)
- **CLI framework:** Cobra (standard Go CLI library), with `cobraext.AddCommands()` auto-wiring
- **DSL parser:** Built into `agentquery` — hand-written recursive descent
- **Field selection:** Built into `agentquery` — `FieldSelector[T]` with presets, lazy evaluation
- **Search:** Implement `SearchProvider` interface or use built-in filesystem search

### Architecture

With `agentquery`, the architecture is simpler — the library handles parsing, field projection, validation, and serialization:

```
cmd/
├── root.go          # Main CLI entry + cobraext.AddCommands(root, schema)
└── ...              # Any additional CLI-specific commands

internal/
├── domain/          # Core data model
└── query/           # Schema wiring
    ├── schema.go    # agentquery.NewSchema[T](), register fields/presets/operations/mutations
    ├── operations.go # Read operation handlers (get, list, summary, etc.)
    └── mutations.go  # Write mutation handlers (create, update, delete, etc.)
```

Key: `agentquery.Schema[T]` is the single source of truth for fields, presets, operations, and mutations. If you add MCP later, it uses the same Schema — identical output guaranteed.

### Parser Tips

The DSL grammar is trivial — don't over-engineer it:

```
query     = operation "(" params ")" [ "{" fields "}" ]
params    = param ("," param)*
param     = key "=" value | value   (positional for IDs, key=value for filters/pagination)
fields    = field+ | preset
batch     = query (";" query)*
```

Pagination uses `skip`/`take` keyword params: `list(skip=10, take=5) { overview }`. Count is a separate operation: `count(status=done)`. Sorting uses `sort_<field>=asc|desc` params: `list(sort_name=asc, sort_priority=desc) { overview }`. No grammar changes needed — `key=value` params handle filters, pagination, and sorting.

A hand-written parser in 100-200 lines of Go handles this. No need for yacc, ANTLR, or parser combinators.

---

## Measuring Success

After implementing the DSL layer, measure token efficiency:

1. **Output size:** pipe DSL output through `wc -c`, compare to CLI text output
2. **Token estimate:** JSON ~3 bytes/token, English text ~4 bytes/token
3. **Batch savings:** compare N separate calls vs 1 batched call (N-1 fewer framing overheads at ~80 tok each)
4. **Session overhead:** should be zero (no tool definitions loaded)

Target: **30-50% token reduction** on reads vs CLI text output, **50%+ reduction** on batch operations.

---

## Assets

Reference implementations and pattern catalogs in `assets/`:

| File | What | Use for |
|------|------|---------|
| [`dsl-parser.go`](assets/dsl-parser.go) | Tokenizer + recursive descent parser + AST | Copy and adapt: operations, fields, presets |
| [`field-selector.go`](assets/field-selector.go) | Field projection with presets and lazy evaluation | Copy and adapt: valid fields, presets, `Apply()` |
| [`scoped-grep.go`](assets/scoped-grep.go) | Scoped regex search with file filters and context | Copy and adapt: file extensions, data directory |
| [`query-patterns.md`](assets/query-patterns.md) | Query catalog: inputs, expected JSON, anti-patterns | Reference for designing your operations |

All Go files are self-contained, domain-agnostic, with `// ADAPT THIS` markers at customization points.

---

## References

- [Real-world comparison: MCP vs DSL vs Grep](references/comparison-example.md) — measured on a 346-element task board, covers 5 workflows with exact token counts
