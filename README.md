# Agent-Facing API

Design pattern for building agent-optimized CLI query layers. Two-layer read approach that minimizes token overhead without adding infrastructure.

## The Problem

CLI tools built for humans produce verbose, formatted output with ANSI colors, alignment padding, and all fields included. When AI agents consume this output, they pay 1.5-5x more tokens than necessary. MCP servers fix the format but add ~2,200 tokens of session overhead (tool definitions) and don't support batching.

## The Solution

Two read layers on top of your existing CLI:

| Layer | Purpose | Output | Example |
|-------|---------|--------|---------|
| **Mini-Query DSL** | Structured reads | JSON or compact tabular | `mytool q 'get(ID) { status assignee }'` |
| **Mutations** | Structured writes | JSON or compact tabular | `mytool m 'update(ID, status=done)' --confirm` |
| **Scoped Grep** | Full-text search | JSON or grouped-by-file text | `mytool grep "pattern" --file progress.md` |

Writes use the same DSL grammar via a dedicated `m` subcommand with safety flags (`--confirm`, `--dry-run`). No new processes, no session overhead, no extra tool definitions.

### Why Not MCP?

- **Session overhead:** ~2,200 tokens of tool definitions loaded per session
- **No batching:** each query is a separate tool call (~80 tok framing each)
- **Break-even:** ~293 queries per session (unrealistic for typical agent work)
- **Identical output:** MCP and DSL return the same JSON when backed by the same field selection engine

See [references/comparison-example.md](references/comparison-example.md) for a detailed measurement on a real 346-element board.

## Design Principle: Format Is a Transport Concern

The source of data (Schema, database, API) must never decide how that data is serialized. The **caller** declares the format, always, at every layer.

This is the only correct way to pipe data from any source to any consumer. If you bake the format into the source, you lock out every other consumer:

| Layer | How the caller declares format |
|-------|-------------------------------|
| CLI | `--format compact` / `--format json` (required flag, no default) |
| SDK (per-call) | `QueryJSONWithMode(query, LLMReadable)` |
| REST API | `Accept: application/json` header |
| gRPC | Request field: `output_format: COMPACT` |

The source stays format-agnostic. It returns structured data; the transport layer serializes it for the consumer. A TUI app, an AI agent, and a human all call the same CLI; they only differ in `--format`.

**Anti-pattern:** configuring output format at schema/source initialization (e.g., `NewSchema(WithOutputMode(...))`). This couples the data model to a single consumer. When a second consumer appears with different needs, you're stuck.

## Output Modes

The `--format` flag (required on CLI commands) controls output serialization:

| `--format` | Mode | Format | Token cost |
|------------|------|--------|------------|
| `json` | `HumanReadable` | Standard JSON | Baseline |
| `compact` / `llm` | `LLMReadable` | Compact tabular text | ~30-50% fewer tokens |

Format is a **caller decision** (transport concern), not a schema setting. The same CLI tool serves different consumers: agents pass `--format compact`, TUI apps and humans pass `--format json`.

**Compact output examples:**

```
# List: CSV-style header + rows
id,name,status,assignee
task-1,Auth service refactor,in-progress,alice
task-2,Dashboard performance,todo,bob

# Single element: key:value pairs
id:task-1
name:Auth service refactor
status:in-progress

# Search: grouped by file
README.md
  3: matching line
  4  context line
```

## Quick Start

Read `SKILL.md` for the full pattern specification:

- **Layer 1: DSL**: syntax template, implementation checklist, token budgets
- **Layer 2: Grep**: scoping, filters, when to use vs DSL
- **Layer 3: CLI**: writes stay as commands
- **Anti-patterns**: common mistakes and their costs
- **Implementation guide**: architecture, parser tips, Go project structure

## Composable query model

`agentquery` includes one canonical pipeline for nested typed predicates,
bounded regular expressions, deterministic sorting, grouping, top-level
pagination, projection, and JSON/compact rendering. The frozen contract lives in
[`composable-query-expressions.md`](.spec/composable-query-expressions.md).
Its machine-checkable grammar and behavior precondition is
[`composable-query-expression-vectors.json`](.spec/composable-query-expression-vectors.json).

```go
limits := schema.QueryLimits()
model, err := agentquery.ParseQueryModel(input, limits)
if err != nil {
    return err
}
out, err := schema.QueryModelJSONASTWithMode(
    ctx, model, access, agentquery.LLMReadable,
)
```

The host opts operations in with `RegisterQueryOperationCapability`, registers
typed fields with `RegisterQueryField`, and supplies a
`SetQuerySnapshotLoader` that honors row and decoded-byte limits before
allocation. `QueryModelSchema(ctx, access)` is the fail-closed discovery entry:
missing or malformed capability metadata is an error, never permission to load
everything and post-filter locally.

```text
list() where satisfiesAll(
  equals(type, "task"),
  satisfiesAny(
    contains(name, "predicate"),
    matchesRegex(description, "(?i)group(?:ing|by)")
  ),
  not(equals(status, "done"))
) sortOrder(updated descending)
  groupBy(status) skip 0 take 3 { id name }
```

Grouped pagination selects groups, not members. Group members retain row sort
order; group keys use typed ascending order followed by null and missing. JSON
returns `GroupResult` records, while compact output emits deterministic
`@group,<presence>,<canonical-json-value-or-empty>,<count>` blocks.

### Migrating from legacy queries

- Existing `Parse`, `Render`, `Schema.Query`, `FilterableField`, `SortSlice`,
  and `PaginateSlice` callers remain source-compatible and keep legacy behavior.
- Opt in one operation at a time. Register its typed field catalog, capability,
  bounded loader, and caller-specific `QueryAccess` before routing it to
  `ParseQueryModel` and the `QueryModel*` entries.
- Legacy flat filters, `sort_FIELD`, `skip`, and `take` aliases lower into the
  same compiler/evaluator. Conflicting legacy and canonical controls refuse;
  there is no fetch-all/post-filter fallback.
- Parse with the exact value returned by `Schema.QueryLimits()`. A model parsed
  under another effective limit digest is refused before snapshot loading.
- Expect additive deterministic behavior: identity tie-breaking, typed errors,
  bounded snapshots/work/results, and grouped top-level pagination.

The reviewed additive release target is `agentquery/v1.7.0`. Publication must
wait for accepted independent review and candidate-bound green validation, then
re-check the authoritative remote tag inventory; an occupied tag advances to
the next unused minor and is never moved.

## Project tools

| Tool | Purpose | Command | Output |
| --- | --- | --- | --- |
| Go toolchain | Build and test the `agentquery` module | `(cd agentquery && go test ./...)` | Test output on stdout; redirect task logs to `.temp/` when evidence must persist |
| Go toolchain | Run focused composable-query contract and behavior tests | `(cd agentquery && go test -run 'TestQueryModel' ./...)` | Frozen-surface, parser, compiler, evaluator, refusal, and bound results on stdout |
| `gofmt` | Format additive Go source and tests | `(cd agentquery && gofmt -w query_model_*.go schema.go)` | Rewrites the named Go files in place |
| Go toolchain | Run the example CLI during local development | `(cd example && go run . --help)` | Human-readable CLI help on stdout |
| `jq` | Validate the frozen query-model vector document | `jq empty .spec/composable-query-expression-vectors.json` | No output on success; validation errors on stderr |
| `rg` | Search source and specifications without scanning generated artifacts | `rg '<pattern>' agentquery .spec README.md` | Matches on stdout; task-scoped captures belong under `.temp/` |
| `task-board` | Read and update tracked Story/Task lifecycle and attach outcomes | `task-board q --format compact 'get(TASK-ID) { status outcomeResources }'` | Board state and resources under the configured external board; never edit its files directly |

## Mutations (Write Operations)

Mutations use the same DSL grammar as queries; no new syntax. They're registered separately via `Mutation()`/`MutationWithMetadata()` and accessed through the `m` subcommand:

```bash
# Create
mytool m 'create(title="Fix bug", status=todo)' --format json

# Update (positional ID)
mytool m 'update(item-1, status=done)' --format json

# Delete (destructive, requires --confirm)
mytool m 'delete(item-1)' --format json --confirm

# Dry run (preview without applying)
mytool m 'delete(item-1)' --format json --dry-run
```

**Safety flags:**
- `--confirm`: required for mutations marked `Destructive: true`
- `--dry-run`: injects `dry_run=true` into parsed AST statements; quoted separators remain payload text
- Schema validation rejects unknown named arguments and unexpected positional fragments with typed mutation errors before handlers run.

For transports that inspect a request before execution, parse once with
`Schema.Parse()` and execute the same AST through `QueryAST()` or
`QueryJSONASTWithMode()`. `Render()` provides canonical DSL text when an AST
must be forwarded to another process or service.

**MutationContext convenience methods** reduce handler boilerplate:

```go
func myHandler(ctx agentquery.MutationContext[Item]) (any, error) {
    id := ctx.PositionalArg()           // first keyless arg (e.g. "item-1" from "update(item-1, ...)")
    status, err := ctx.RequireArg("status") // named arg, error if missing
    priority := ctx.ArgDefault("priority", "medium") // named arg with default
    // ...
}
```

**Schema introspection** includes separate `mutations` and `mutationMetadata` sections, so agents clearly see the read/write boundary:

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

## AI Agent Skill Setup

This repo is a skill for AI coding agents (Claude Code, Codex CLI, and similar tools).

```bash
# Clone
git clone <repo-url> ~/src/skill-agent-facing-api

# Symlink for Claude Code
mkdir -p ~/.claude/skills
ln -s ~/src/skill-agent-facing-api ~/.claude/skills/agent-facing-api

# Symlink for Codex CLI
mkdir -p ~/.codex/skills
ln -s ~/src/skill-agent-facing-api ~/.codex/skills/agent-facing-api
```

## Articles

Research on agent-facing output optimization:

| Article | Summary |
|---------|---------|
| [Field Name Aliases in Schema-Once Output: Do They Save Tokens?](articles/field-alias-compression-study.md) | Fixture-backed historical measurements show a fixed 5-token alias-header saving at the tested scales. A separate session model is conditional on its hard-coded discovery cost, query mix, and eviction assumptions. The article rejects aliases for this formatter on that bounded evidence; no repository-local MCP benchmark establishes a transport-wide token winner. |

## References

| File | Description |
|------|-------------|
| `SKILL.md` | Full pattern specification: DSL design, grep scoping, output modes, implementation guide |
| `assets/dsl-parser.go` | Reference implementation: tokenizer + recursive descent parser + AST |
| `assets/field-selector.go` | Reference implementation: field projection with presets |
| `assets/scoped-grep.go` | Reference implementation: scoped regex search with context lines |
| `assets/query-patterns.md` | Query catalog: inputs, expected JSON, anti-patterns |
| `references/comparison-example.md` | Real-world token measurement: MCP vs DSL vs Grep on a 346-element task board |
| `.spec/composable-query-expressions.md` | Normative architecture for composable predicates and query modifiers |
| `.spec/composable-query-expression-vectors.json` | Machine-checkable public identifiers, limits, shapes, and conformance vectors |

<!-- relux-ecosystem:start -->

## About Relux Works

This project is part of the open-source ecosystem of
[Relux Works](https://relux.works), an AI-native software development studio.
We build fixed-price MVPs, rescue vibe-coded apps, run local AI inference, and
train teams to work with coding agents. Much of the infrastructure behind that
work is open source.

- Full catalog: [relux.works/en/open-source](https://relux.works/en/open-source/)
- Agentic enablement: [agent harnesses & team training](https://relux.works/en/agentic-enablement/)
- Hire us the agent-native way: point your assistant at `https://api.relux.works/mcp`
- Contact: ivan@relux.works

<!-- relux-ecosystem:end -->

## License

MIT
