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
./example/taskdemo grep "TODO" --format json

# Compact output for LLM agents
./example/taskdemo q 'list() { overview }' --format compact
./example/taskdemo q 'get(task-1) { overview }' --format compact
./example/taskdemo grep "TODO" --format compact
```

No linter is configured. No CI/CD pipeline exists.

## Architecture

### Library (`agentquery/`)

The core is a generic `Schema[T]` type parameterized on the domain item type. Users register fields, presets, operations, and a data loader on the schema, then call `Query()` or `Search()`.

**Data flow:** Input string → `Parse()` (tokenizer + recursive descent) → `Query` AST → `Schema.executeStatement()` → `OperationHandler` receives `OperationContext` with parsed statement, `FieldSelector`, and lazy item loader → returns JSON-serializable result → serialized as JSON (default) or compact tabular (via `--format compact` or `*WithMode()` API).

Key files in `agentquery/`:

- **`schema.go`** — Central `Schema[T]` type. Registers fields, presets, operations, loader. Builds `ParserConfig`. Has a built-in `schema` introspection operation. `QueryJSONWithMode()` and `SearchJSONWithMode()` for caller-specified output format.
- **`parser.go`** — Tokenizer + recursive descent parser. Grammar: `operation(params) { fields }` with `;` batching. Validates operations and resolves fields (including preset expansion) at parse time via `ParserConfig`.
- **`selector.go`** — `FieldSelector[T]` applies field projection to domain items. Created internally by Schema from parsed field lists. Lazy — only calls accessors for selected fields. `ApplyValues()` returns ordered values without keys (used by compact formatter).
- **`query.go`** — `Schema.Query()` and `QueryJSON()`. Executes parsed statements, handles batching (single result unwrapped, multiple → `[]any`). Per-statement errors don't abort the batch. `formatLLMReadable()` handles compact output for single and batch queries.
- **`format.go`** — `FormatCompact()` tabular formatter. Lists → CSV-style header + value rows. Single objects → key:value pairs. Falls back to JSON for complex/error types. Handles CSV escaping and value formatting.
- **`search.go`** — `Search()` does recursive regex search with file extension filtering, glob filtering, case-insensitive flag, and context lines. Independent of Schema but also available as `Schema.Search()`. `FormatSearchCompact()` produces grouped-by-file text output.
- **`ast.go`** — AST types: `Query`, `Statement`, `Arg`.
- **`types.go`** — Shared types: `FieldAccessor[T]`, `OperationHandler[T]`, `OperationContext[T]`, `SearchResult`, `SearchOptions`, `OutputMode` (`HumanReadable`, `LLMReadable`).
- **`error.go`** — `ParseError` (syntax) and `Error` (runtime, with code/message/details).
- **`cobraext/command.go`** — Cobra command factories (`QueryCommand`, `SearchCommand`, `AddCommands`). Isolated sub-package so non-Cobra users don't import it. `--format` flag on both commands (`json` default, `compact`/`llm` for token-efficient output).

### Example (`example/`)

Separate Go module (`example/go.mod`) with a `taskdemo` CLI that wires up a `Task` domain type against the library. Shows how to register fields, presets, operations (`get`, `list`, `summary`), and use `cobraext.AddCommands`.

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

## Key Design Decisions

- **Generics over interfaces**: `Schema[T]` is parameterized on the domain type. Field accessors are `func(T) any`, not reflection-based.
- **Parse-time validation**: Operations and fields are validated during parsing (not execution), producing early errors with position info.
- **Preset expansion at parse time**: Presets (like `overview`) expand to field lists in the parser via `FieldResolver` interface (implemented by Schema).
- **Lazy item loading**: `OperationContext.Items` is a `func() ([]T, error)` — only called if the operation needs the dataset.
- **Batch error isolation**: In a batch query `a(); b(); c()`, if `b()` fails, `a()` and `c()` still return results. The error is inlined as `{"error": {"message": "..."}}`.
- **cobraext is optional**: The Cobra integration lives in a sub-package. Core library has zero dependencies beyond stdlib.
- **Output format is a transport concern, not a schema concern**: Schema stays format-agnostic. `QueryJSON()` / `SearchJSON()` always produce JSON. `QueryJSONWithMode()` / `SearchJSONWithMode()` accept an explicit `OutputMode` for callers that need compact output. The `--format` CLI flag (`json` default, `compact`/`llm`) puts the format decision where it belongs — at the call site. Same CLI tool serves humans (JSON), TUI apps (JSON), and LLM agents (`--format compact`).
