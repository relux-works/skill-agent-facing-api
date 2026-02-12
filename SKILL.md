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

A design pattern for building CLI tools that AI agents query efficiently. Two read layers — structured DSL and full-text grep — plus CLI commands for writes.

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
│  Structured read ──► DSL  (compact JSON)    │
│  Text search     ──► Grep (scoped ripgrep)  │
│  Write/mutate    ──► CLI  (regular commands) │
└─────────────────────────────────────────────┘
```

Three access modes, one CLI binary. No extra processes, no session overhead, no tool definitions beyond the Bash tool the agent already has.

### Why not MCP?

MCP loads tool definitions into the agent's context at session start. For a typical tool with 10-15 operations, that's ~2,000-3,000 tokens of dead weight per session. The DSL approach costs zero — the agent calls Bash with a query string.

Per-query, MCP and DSL produce identical output when backed by the same field selection engine. But DSL supports batching (multiple queries in one call), while MCP requires separate tool calls. MCP never breaks even for typical agent sessions.

See [references/comparison-example.md](references/comparison-example.md) for a real-world measurement on a 346-element board.

---

## Layer 1: Mini-Query DSL

The primary read interface for agents. A single CLI subcommand that accepts a compact query string and returns structured JSON.

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

- **Operation** — what to query: `get`, `list`, `summary`, `agents`, etc.
- **Params** — filters: `type=task, status=done`, `stale=60`, an ID, etc.
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

# Summary (no field projection needed)
mytool q 'summary()'
# → {"epics":5,"stories":12,"tasks":48,"done":31,"in_progress":10,"blocked":2}
```

### Output Modes

The DSL supports two output modes:

| Mode | Flag | Format | Use case |
|------|------|--------|----------|
| **HumanReadable** (default) | `--format json` | Standard JSON | Human inspection, piping to `jq` |
| **LLMReadable** | `--format compact` | Tabular text | Agent consumption — fewer tokens |

Configure the default at schema construction:

```go
schema := agentquery.NewSchema[Task](
    agentquery.WithOutputMode(agentquery.LLMReadable),
)
```

Override per-invocation with `--format`:

```bash
# Force compact output regardless of schema default
mytool q 'list(status=todo) { overview }' --format compact

# Force JSON even if schema defaults to compact
mytool q 'list()' --format json
```

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

**When to use which:** If your tool is exclusively consumed by agents, set `LLMReadable` as the schema default. If humans also inspect output, keep `HumanReadable` as default and let agents pass `--format compact` when they want savings.

### Token Budget

Typical per-query costs (input + output + ~80 tok framing):

| Query type | Tokens | Notes |
|------------|--------|-------|
| Element lookup (minimal) | ~110 | ID + 2-3 fields |
| Element lookup (full) | ~380 | All fields |
| Filtered list (overview) | ~150-300 | Scales with result count |
| Summary | ~450 | Fixed size, scales with board |
| Batch of 3 (status only) | ~140 | Single call, 3 results |

Compare to CLI text output for the same data: 1.5-5x more tokens (ANSI codes, formatting, verbose labels).

---

## Layer 2: Scoped Grep

Full-text search across the data store. For when the agent needs to find content by text pattern rather than by structured fields.

### Design Principles

1. **Scoped to the data directory** — don't search the whole filesystem
2. **Regex support** — agents build patterns dynamically
3. **File filter** — narrow search to specific file types (`--file progress.md`)
4. **Case-insensitive flag** — `-i`
5. **Context lines** — `-C N` for surrounding context
6. **Compact output** — `file:line:content` format (like ripgrep)

### Implementation Checklist

- [ ] Subcommand: `mytool grep <pattern>` (or `mytool search`)
- [ ] Scope: only search within the tool's data directory
- [ ] Flags: `--file <glob>`, `-i`, `-C <N>`
- [ ] Output: `relative/path:line_number:matched_line`
- [ ] No JSON needed — grep output is inherently line-oriented

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

## Layer 3: CLI Commands (Writes)

All mutations stay as regular CLI commands. No DSL needed for writes — they're infrequent and the confirmation output is small.

```bash
mytool create item --name "something" --description "..."
mytool update ITEM-42 --status done
mytool link ITEM-42 --blocked-by ITEM-41
```

Write commands return human-readable confirmation (~100 tokens). This is fine — writes are rare compared to reads.

---

## Decision Table

```
Need data from the tool?
  Structured query (status, filter, lookup)?  ──► DSL
  Text search across content?                 ──► Grep
  Mutation (create, update, delete)?          ──► CLI command
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
| JSON output to LLM agents | JSON keys repeated per item, quoting overhead | Use `LLMReadable` mode — tabular format with schema-once header |

---

## Implementation Guide

### Recommended Stack

- **Language:** Go (single binary, fast startup, good for CLI tools)
- **CLI framework:** Cobra (standard Go CLI library)
- **DSL parser:** Hand-written recursive descent (queries are simple, no need for parser generators)
- **Field selection:** Shared package used by DSL (and MCP if you ever add one)
- **Grep:** Shell out to `grep -rn` or implement with Go's `regexp` + `filepath.Walk`

### Architecture

```
cmd/
├── root.go          # Main CLI entry
├── query.go         # DSL subcommand: parses query string, dispatches to operations
├── grep.go          # Grep subcommand: scoped regex search
├── create.go        # Write: create elements
├── update.go        # Write: update elements
└── ...

internal/
├── domain/          # Core data model
├── fields/          # Field selection & projection (shared between DSL and any future MCP)
│   ├── selector.go  # Field whitelist, preset definitions
│   └── project.go   # Apply projection to domain objects
├── query/           # DSL parser & executor
│   ├── parser.go    # Tokenizer + recursive descent
│   ├── ops.go       # Operation handlers (get, list, summary, etc.)
│   └── batch.go     # Semicolon splitting, multi-query execution
└── search/          # Grep implementation
    └── grep.go      # Scoped regex search
```

Key: `internal/fields/` is the shared package. If you later add MCP, it imports the same field selection logic — guaranteeing identical output.

### Parser Tips

The DSL grammar is trivial — don't over-engineer it:

```
query     = operation "(" params ")" [ "{" fields "}" ]
params    = param ("," param)*
param     = key "=" value | value   (positional for IDs)
fields    = field+ | preset
batch     = query (";" query)*
```

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
