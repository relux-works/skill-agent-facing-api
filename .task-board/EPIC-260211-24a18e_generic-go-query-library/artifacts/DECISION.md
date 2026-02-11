# Architectural Decision: Generic Go Query Library

**Date:** 2026-02-11
**Status:** Approved
**Research inputs:**
- STORY-260211-36v125/artifacts/RESEARCH.md — API patterns, field projection, module structure
- STORY-260211-1krn1n/artifacts/RESEARCH.md — Parser configuration, AST design, error handling, testing
- STORY-260211-1n61qb/artifacts/RESEARCH.md — Facade API, search results, Cobra integration, error handling

---

## 1. Core Type: `Schema[T any]`

Single generic type that serves as both the registration surface and the execution facade.

```go
package agentquery

type Schema[T any] struct { /* unexported fields */ }

func NewSchema[T any](opts ...Option) *Schema[T]
```

**Why generic on `T`:** Type-safe field accessors. A `Schema[*board.Element]` only accepts `func(item *board.Element) any` — compile-time guarantee that accessors match the domain type.

**Why `any` output on accessors:** JSON serialization needs `any`. A heterogeneous field registry (string fields, int fields, slice fields) requires erased output types. This matches the graphql-go pattern.

**Why one type, not Engine + separate registries:** Avoids the generics-across-packages problem. If `Engine` is non-generic but `FieldRegistry[T]` is generic, you need type assertions at the boundary. Keeping everything on `Schema[T]` preserves type safety throughout.

---

## 2. Public API Surface

### 2.1 Registration (methods on Schema)

```go
// Field registration
type FieldAccessor[T any] func(item T) any

func (s *Schema[T]) Field(name string, accessor FieldAccessor[T])
func (s *Schema[T]) Preset(name string, fields ...string)
func (s *Schema[T]) DefaultFields(fields ...string)

// Operation registration
type OperationHandler[T any] func(ctx OperationContext[T]) (any, error)

type OperationContext[T any] struct {
    Statement Statement
    Selector  *FieldSelector[T]
    Items     func() ([]T, error)  // lazy loader, called only if operation needs data
}

func (s *Schema[T]) Operation(name string, handler OperationHandler[T])
```

**Registration happens after construction, before first query.** No functional options for fields/operations — direct methods are cleaner when you have 20+ fields.

### 2.2 Functional Options (for non-registration config)

```go
type Option func(*config)

func WithDataDir(dir string) Option          // root data directory
func WithExtensions(exts ...string) Option   // file extensions for search (.md default)
func WithLoader[T any](fn func() ([]T, error)) Option  // domain data loader
```

**Note:** `WithLoader` needs to be a method on Schema (not a standalone function) because Go doesn't support type parameters on functions used as functional options to a generic type. Alternative: `schema.SetLoader(fn)`.

### 2.3 Query and Search (methods on Schema)

```go
// Structured DSL query — returns JSON-serializable result
func (s *Schema[T]) Query(input string) (any, error)

// Full-text regex search scoped to data directory
func (s *Schema[T]) Search(pattern string, opts SearchOptions) ([]SearchResult, error)

// JSON convenience methods
func (s *Schema[T]) QueryJSON(input string) ([]byte, error)
func (s *Schema[T]) SearchJSON(pattern string, opts SearchOptions) ([]byte, error)
```

### 2.4 Field Selector (created internally, exposed to operation handlers)

```go
type FieldSelector[T any] struct { /* internal */ }

func (fs *FieldSelector[T]) Apply(item T) map[string]any
func (fs *FieldSelector[T]) Include(field string) bool
func (fs *FieldSelector[T]) Fields() []string
```

Operation handlers receive a pre-built `FieldSelector` from the parsed projection. They call `selector.Apply(item)` per element to produce the response.

---

## 3. Parser Design

### 3.1 Configuration

```go
type ParserConfig struct {
    Operations    map[string]bool  // nil = accept any operation
    FieldResolver FieldResolver    // nil = accept any field name
    IdentChecker  IdentChecker     // nil = default permissive rules
}

type FieldResolver interface {
    ResolveField(name string) ([]string, error)
}

type IdentChecker interface {
    IsIdentStart(ch byte) bool
    IsIdentChar(ch byte) bool
}
```

**Key decision:** Parser validates syntax. `FieldResolver` is the bridge for semantic validation — the Schema constructs one from its field registry and passes it to the parser. If nil, parser accepts all identifiers.

### 3.2 AST

```go
type Query struct {
    Statements []Statement
}

type Statement struct {
    Operation string
    Args      []Arg
    Fields    []string  // raw identifiers (preset expansion happens at resolver)
    Pos       Pos
}

type Arg struct {
    Key   string  // empty for positional
    Value string
    Pos   Pos
}

type Pos struct {
    Offset int
    Line   int
    Column int
}
```

**Flat AST.** No expression types, no nested queries. `Fields` stores raw identifiers — preset expansion happens in the resolver, not the parser.

### 3.3 Error Handling

```go
type ParseError struct {
    Message  string `json:"message"`
    Pos      Pos    `json:"pos"`
    Got      string `json:"got,omitempty"`
    Expected string `json:"expected,omitempty"`
}

func (e *ParseError) Error() string
```

**Fail-fast.** Single error per parse. Implements `error` interface. JSON-serializable.

### 3.4 Identifiers

Default: `[a-zA-Z0-9_][a-zA-Z0-9_-]*` (permissive, covers UUIDs, slugs, TASK-12 style IDs). Override via `IdentChecker` for stricter rules. Don't implement `IdentChecker` in v1 unless actually needed — document the default clearly.

---

## 4. Search Design

```go
type SearchResult struct {
    Source  Source `json:"source"`
    Content string `json:"content"`
    IsMatch bool   `json:"isMatch"`
}

type Source struct {
    Path string `json:"path"`  // relative to data directory
    Line int    `json:"line"`  // 1-indexed
}

type SearchOptions struct {
    FileGlob        string
    CaseInsensitive bool
    ContextLines    int
}
```

**`IsMatch` field:** Distinguishes actual regex matches from context-only lines when `-C N` is used. Current task-board implementation conflates them.

**Paths always relative** to data directory — compact, deterministic, no host filesystem leakage.

---

## 5. Error Types

```go
type Error struct {
    Code    string         `json:"code"`
    Message string         `json:"message"`
    Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

const (
    ErrParse      = "PARSE_ERROR"
    ErrNotFound   = "NOT_FOUND"
    ErrValidation = "VALIDATION_ERROR"
    ErrInternal   = "INTERNAL_ERROR"
)
```

**Batch queries:** per-statement errors in the result array. Batch as a whole succeeds. Matches current task-board behavior.

---

## 6. Package Structure

```
agentquery/                  # root package: Schema[T], Query, Search, types
    schema.go                # Schema[T] struct, NewSchema, Field, Preset, Operation
    query.go                 # Query(), QueryJSON() — parse + dispatch + project
    search.go                # Search(), SearchJSON() — scoped grep
    selector.go              # FieldSelector[T], Apply, Include
    parser.go                # tokenizer + recursive descent
    ast.go                   # Query, Statement, Arg, Pos
    error.go                 # ParseError, Error, error codes
    types.go                 # SearchResult, Source, SearchOptions, OperationContext

    cobraext/                # separate subpackage: Cobra integration
        command.go           # QueryCommand, SearchCommand, AddCommands
```

**Flat root package.** No internal/ subpackages for a library this size. Parser, selector, search — all in the same package. Only `cobraext/` is separate (to isolate the Cobra dependency).

**Module path:** `github.com/<org>/agentquery` (or wherever it's hosted). Minimum Go version: `go 1.21`.

**Dependencies:** Core package = stdlib only. `cobraext/` imports `github.com/spf13/cobra`.

---

## 7. Data Loading

Schema accepts a loader function:

```go
schema.SetLoader(func() ([]*board.Element, error) {
    return board.Load(boardDir)
})
```

The loader is called inside `Query()` before dispatch. Operation handlers receive data via `ctx.Items()` (lazy — only called if the operation needs the full dataset). Search does NOT use the loader — it operates on files directly.

**Why loader, not pre-loaded data:** Matches the task-board pattern (fresh load per query). Keeps the API simple — user doesn't have to load-then-query manually.

---

## 8. Cobra Integration

Separate `cobraext/` subpackage:

```go
package cobraext

func QueryCommand[T any](schema *agentquery.Schema[T]) *cobra.Command
func SearchCommand[T any](schema *agentquery.Schema[T]) *cobra.Command
func AddCommands[T any](parent *cobra.Command, schema *agentquery.Schema[T])
```

Users who don't use Cobra never import `cobraext/` and never pull the dependency.

---

## 9. Dependency Graph (Revised)

```
STORY-36v125: field-registry-and-projection
    → types, FieldAccessor[T], FieldSelector[T], presets, Apply/Include

STORY-1krn1n: dsl-parser-engine          (can run in PARALLEL with 36v125)
    → tokenizer, recursive descent, AST, ParseError, ParserConfig
    → defines FieldResolver interface (parser package owns it)

STORY-165ncj: operation-framework         (blocked by 36v125 + 1krn1n)
    → OperationHandler[T], OperationContext, dispatch, batch execution

STORY-e06u45: unified-search-facade       (blocked by 36v125)
    → SearchResult, Source, scoped grep, context lines, IsMatch

STORY-1n61qb: facade-api-and-wiring       (blocked by 165ncj + e06u45)
    → Schema[T], NewSchema, Query(), Search(), loader, JSON helpers

STORY-267ra3: example-project             (blocked by 1n61qb)
    → example/ directory, sample domain, README
```

**Key change:** Parser (1krn1n) can run in parallel with field registry (36v125). Parser defines its own `FieldResolver` interface; field registry implements it. No import dependency.

---

## 10. What We're NOT Doing

- **No reflection.** Explicit field registration only.
- **No code generation.** Hand-written parser, ~400 lines.
- **No expression types in AST.** Flat statements. Complex filters go inside argument strings.
- **No runtime registration.** All fields/operations registered before first query. No mutexes.
- **No MCP server.** DSL over Bash is strictly better (see COMPARISON.md).
- **No Visitor pattern on AST.** `for` loop is the visitor.
- **No IdentChecker in v1.** Document default rules, add later if needed.
- **No multi-module repo.** Single go.mod. Cobra in `go.sum` is fine for non-importers.
