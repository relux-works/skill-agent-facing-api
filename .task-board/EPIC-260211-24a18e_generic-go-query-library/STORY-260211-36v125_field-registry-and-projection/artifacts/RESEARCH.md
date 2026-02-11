# Research: Designing a Generic Go Library for Agent-Facing Query APIs

Research date: 2026-02-11
Context: Extracting the mini-query DSL from task-board CLI into a reusable Go library.

---

## 1. API Design Patterns in Go for Registries/Plugins

### How Go Libraries Let Users Register Handlers

Three main patterns dominate the Go ecosystem:

#### Pattern A: Function Types (chi, graphql-go)

Chi uses plain `http.HandlerFunc` for handler registration. The registration API is method-based on a router struct:

```go
// chi pattern: method on struct, accepts function type
r := chi.NewRouter()
r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
    // handler logic
})
```

graphql-go uses a `map[string]*Field` where each field has a `Resolve` function:

```go
// graphql-go: map of name -> struct with function field
type FieldResolveFn func(p ResolveParams) (interface{}, error)

type Fields map[string]*Field

fields := graphql.Fields{
    "hello": &graphql.Field{
        Type:    graphql.String,
        Resolve: func(p graphql.ResolveParams) (interface{}, error) {
            return "world", nil
        },
    },
}
```

#### Pattern B: Struct Methods / AddCommand (Cobra)

Cobra uses a tree of `Command` structs with `AddCommand()` for registration:

```go
// cobra: struct with AddCommand method
rootCmd.AddCommand(&cobra.Command{
    Use:   "serve",
    Short: "Start the server",
    RunE:  func(cmd *cobra.Command, args []string) error { ... },
})
```

This is a two-step approach: define a struct, then register it on a parent. The struct carries both metadata (Use, Short, Long) and behavior (RunE).

#### Pattern C: Constructor Functions / Providers (fx)

Uber's fx uses `fx.Provide()` and `fx.Invoke()` with plain Go constructor functions:

```go
// fx: register plain Go functions, DI resolves parameters
fx.New(
    fx.Provide(NewUserRepository),
    fx.Provide(NewUserService),
    fx.Invoke(StartServer),
)
```

No interfaces, no struct tags -- just functions whose parameter types serve as the dependency graph.

#### Pattern D: Functional Options (expr-lang)

expr-lang uses functional options for configuration:

```go
// expr: functional options on Compile
type Option func(c *conf.Config)

program, err := expr.Compile(input,
    expr.Env(myStruct{}),
    expr.Function("customFn", myFunc),
    expr.AllowUndefinedVariables(),
)
```

### What Works Best for Our Use Case

Our use case has two registration axes:
1. **Operations** (get, list, summary, custom) -- each with a handler function
2. **Fields** (id, name, status) -- each with an accessor function from domain type to value

**Recommendation: Function types in a map, registered via methods on a Schema struct.**

Reasoning:
- Operations are analogous to chi routes or cobra commands -- named handlers. A `map[string]OperationHandler` is the natural Go pattern.
- Fields are analogous to graphql-go field resolvers -- named accessors. A `map[string]FieldAccessor` is clean and explicit.
- Functional options are overkill here. Our configuration is not about tweaking behavior of a single operation; it is about registering many distinct operations and fields.
- The Cobra-style `AddCommand` struct approach adds unnecessary ceremony for our simple handler functions.

---

## 2. Field Projection Approaches

### Three Approaches Compared

#### Approach A: map[string]any with Function Accessors

```go
// Each field is a named function that extracts a value from any item
type FieldAccessor[T any] func(item T) any

type FieldRegistry[T any] struct {
    accessors map[string]FieldAccessor[T]
    presets   map[string][]string
}

func (r *FieldRegistry[T]) Register(name string, fn FieldAccessor[T]) {
    r.accessors[name] = fn
}
```

This is what our existing implementation does (without generics). The `fields.Selector.Apply()` method is a giant if-chain that maps field names to struct field reads:

```go
// Current implementation (domain-coupled)
if fs.Include("id")     { result["id"] = elem.ID() }
if fs.Include("status") { result["status"] = string(elem.Status) }
// ...20 more fields
```

**Pros:** Simple, zero-reflection, fast. Output is `map[string]any` which serializes directly to JSON.
**Cons:** The accessor is not type-safe at the value level. Every accessor returns `any`.

#### Approach B: Go Generics with Type Parameters

```go
type FieldDef[T any, V any] struct {
    Name     string
    Accessor func(item T) V
}
```

**Pros:** Full type safety at the accessor level.
**Cons:** Go generics don't support heterogeneous collections well. You can't put `FieldDef[User, string]` and `FieldDef[User, int]` in the same slice/map without erasing the V parameter to `any`, which defeats the purpose. The Go type system simply doesn't support this kind of heterogeneous field registry with full type safety.

#### Approach C: Reflection (reflect package)

```go
// Automatically derive fields from struct tags
type User struct {
    ID     string `query:"id"`
    Name   string `query:"name"`
    Status string `query:"status"`
}
```

**Pros:** Zero boilerplate for simple cases. User just defines a struct.
**Cons:** 18-20x slower than generics/direct access. No compile-time safety. Breaks with unexported fields. Hard to support computed fields (like `isBlocked` which requires board context). Fragile when structs change.

### How graphql-go Handles This

graphql-go uses `map[string]*Field` where each Field has a `Resolve func(ResolveParams) (interface{}, error)`. This is essentially Approach A with a richer params struct. The resolver function receives the "source" object and returns `interface{}`.

gqlgen (code generation approach) generates typed resolvers from a schema, achieving type safety through code generation rather than runtime generics. This is powerful but too heavy for our use case.

### Trade-Off Analysis

| Aspect | map[string]func -> any | Generics | Reflection |
|--------|----------------------|----------|------------|
| Type safety (input) | Via generic T | Full | None |
| Type safety (output) | None (any) | Full but unusable | None |
| Performance | ~0.25 ns/op | ~0.25 ns/op | ~4.5 ns/op |
| Computed fields | Easy | Easy | Hard |
| Boilerplate | Medium | Medium | Low |
| Heterogeneous registry | Natural | Impossible without erasure | Natural |

### Recommendation

**Use Approach A: `map[string]FieldAccessor[T]` where `type FieldAccessor[T any] func(item T) any`.**

The generic type parameter `T` gives type safety on the input side (the domain type). The output being `any` is acceptable because:
1. JSON serialization needs `any` anyway
2. A heterogeneous field registry _requires_ erased output types
3. The existing implementation already works this way and is proven in production

This matches the graphql-go pattern exactly, adapted with Go 1.18+ generics for the input type.

---

## 3. Making the Parser Generic

### Should Operations Be Hardcoded or Registered?

The current parser has:

```go
var validOperations = map[string]bool{
    "get": true, "list": true, "summary": true, "plan": true, "agents": true,
}
```

**Recommendation: Registered, not hardcoded.**

The parser should receive the set of valid operations from the Schema at parse time. This is trivial -- just pass the map:

```go
// Generic parser accepts valid operations from caller
func (p *parser) parseStatement(validOps map[string]bool) (*Statement, error) {
    op := strings.ToLower(opTok.val)
    if !validOps[op] {
        names := sortedKeys(validOps)
        return nil, fmt.Errorf("unknown operation %q (valid: %s)", op, strings.Join(names, ", "))
    }
    // ...
}
```

The current parser code is already structured this way -- `validOperations` is a package-level variable that could trivially become a parameter.

### Should Field Names Be Validated by Parser or Executor?

Currently, field validation happens in the parser (during `parseProjection`):

```go
// Current: parser validates fields
if !fields.ValidFields[name] {
    return nil, fmt.Errorf("unknown field %q at position %d", name, tok.pos)
}
```

**Recommendation: Validate in the parser.** Reasons:
1. **Fail fast.** Bad field names should error before any execution happens.
2. **Better error messages.** The parser has position information for error reporting.
3. **The parser already receives the field registry** (it needs it for preset expansion anyway).
4. **No performance concern.** Validation is a map lookup.

The parser should receive the field registry (valid names + presets) as configuration, and validate during parsing. This is the approach used by expr-lang (compile-time validation of variable references against the Env type).

### How to Handle Domain-Specific Identifier Formats

The current tokenizer treats identifiers liberally:

```go
// Current: identifiers include letters, digits, underscores, hyphens
func isIdentChar(ch byte) bool {
    return isIdentStart(ch) || ch == '-'
}
```

This works for `TASK-260211-abc123` but would also accept `---` or `123-456` as identifiers.

**Recommendation: Keep the tokenizer liberal, validate identifiers in the executor.**

The tokenizer's job is to split input into tokens, not to validate domain semantics. Different domains will have different ID formats (UUIDs, slugs, numeric IDs, etc.). The parser should produce an AST with string values; the executor (which knows the domain) validates whether `TASK-260211-abc123` is a valid ID.

The tokenizer should support a configurable set of "extended identifier characters" to handle domains that use dots, slashes, or other separators:

```go
type ParserConfig struct {
    ValidOperations map[string]bool
    ValidFields     map[string]bool
    Presets         map[string][]string
    IdentChars      func(ch byte) bool  // optional, defaults to letters/digits/underscores/hyphens
}
```

---

## 4. Facade Pattern in Go

### Single Struct vs Package-Level Functions

Two approaches for the top-level API:

#### Option A: Single struct with methods (chi, cobra)

```go
// Struct-based
s := agentquery.New[MyElement]()
s.RegisterField("id", func(e MyElement) any { return e.ID })
s.RegisterOperation("get", myGetHandler)
result, err := s.Query("get(123) { id name }")
result, err := s.Search("pattern")
```

#### Option B: Package-level functions (expr-lang)

```go
// Package-level
schema := agentquery.NewSchema[MyElement](
    agentquery.Field("id", func(e MyElement) any { return e.ID }),
    agentquery.Operation("get", myGetHandler),
)
result, err := agentquery.Query(schema, "get(123) { id name }")
```

**Recommendation: Struct with methods (Option A).**

Reasons:
1. The Schema holds mutable state (registered fields, operations, presets). A struct naturally encapsulates this.
2. `Query()` and `Search()` are methods on the same type -- this makes sense conceptually ("query this schema" / "search this schema").
3. Multiple schemas can coexist (e.g., for testing, or for different domain models).
4. Package-level functions are better for stateless operations (like `expr.Compile`). Our schema is inherently stateful.

### Builder vs Functional Options vs Simple Constructor

| Pattern | Example | Best For |
|---------|---------|----------|
| Simple constructor | `New[T]()` then `Register*()` | Mutable configuration, many registrations |
| Functional options | `New[T](WithField(...), WithOp(...))` | Immutable configuration, few options |
| Builder | `New[T]().Field("id", fn).Op("get", fn).Build()` | Complex construction with validation at end |

**Recommendation: Simple constructor with Register methods.**

Functional options would require wrapping every field and operation registration in a closure -- verbose and unnatural when you have 20+ fields to register. The builder pattern adds ceremony (`.Build()`) for no real benefit since our schema doesn't need a "frozen" state.

```go
// Recommended API shape
schema := agentquery.NewSchema[board.Element]()

// Register fields
schema.Field("id", func(e *board.Element) any { return e.ID() })
schema.Field("name", func(e *board.Element) any { return e.Name })
schema.Field("status", func(e *board.Element) any { return string(e.Status) })

// Register presets
schema.Preset("minimal", "id", "status")
schema.Preset("default", "id", "name", "status")

// Register operations
schema.Operation("get", myGetHandler)
schema.Operation("list", myListHandler)

// Use
result, err := schema.Query("get(TASK-1) { minimal }")
matches, err := schema.Search("pattern", searchOpts)
```

### How to Expose Both Query and Search from One Type

Both are methods on Schema. Query parses a DSL string and dispatches to registered operations. Search does regex matching across a data source.

```go
type Schema[T any] struct {
    fields     map[string]FieldAccessor[T]
    presets    map[string][]string
    operations map[string]OperationHandler[T]
    searcher   Searcher  // pluggable search backend
}

func (s *Schema[T]) Query(input string) (any, error)  { ... }
func (s *Schema[T]) Search(pattern string, opts ...SearchOption) ([]SearchMatch, error) { ... }
```

The Search method is independent of the DSL -- it is a utility for full-text grep across whatever data source the schema is configured with. This keeps the two concerns (structured queries vs. full-text search) cleanly separated but accessible from the same entry point.

---

## 5. Go Module Structure

### Package Layout for a Library

Following the official Go module layout guidance:

```
agentquery/                    # Root package: public API (Schema, Query, Search)
    schema.go                  # Schema[T] type, Field(), Preset(), Operation()
    query.go                   # Query() method, top-level execution
    search.go                  # Search() method
    types.go                   # Public types: Statement, Arg, FieldAccessor, etc.

    parser/                    # Subpackage: tokenizer + parser (internal or public)
        parser.go              # ParseQuery() with configurable valid ops/fields
        token.go               # Token types, tokenizer
        parser_test.go

    selector/                  # Subpackage: field selection/projection
        selector.go            # Selector type, NewSelector(), Apply()
        selector_test.go

    cobraext/                  # Separate subpackage: Cobra integration helper
        command.go             # func NewQueryCommand(schema) *cobra.Command
```

### Internal vs Public Subpackages

**Parser:** Should be `internal/parser` or just inline in root package.
- Users don't need to call the parser directly. They call `schema.Query()`.
- Keeping it internal allows refactoring the parser without breaking API.

**Selector:** Should be `internal/selector` or inline in root package.
- Same reasoning. The selector is an implementation detail of projection.

**Alternative: Everything in root package.**
For a focused library, a flat structure is often best:

```
agentquery/
    schema.go         # Schema[T], registration methods
    parser.go         # Tokenizer + parser
    selector.go       # Field selector/projection
    query.go          # Query execution
    search.go         # Full-text search
    types.go          # Shared types (Statement, Arg, SearchMatch, etc.)

    cobraext/          # ONLY subpackage: Cobra helper (separate import path)
        command.go
```

This is the pattern recommended by the official Go docs: "Start simple, add structure only when needed."

### How to Include a Cobra Helper Without Forcing Cobra as a Dependency

**Use a separate subpackage with its own import path.**

When `cobraext/` imports `github.com/spf13/cobra`, that dependency is only pulled in when a user imports `github.com/yourmodule/agentquery/cobraext`. Users who only import `github.com/yourmodule/agentquery` never touch Cobra.

```go
// cobraext/command.go
package cobraext

import (
    "github.com/spf13/cobra"
    "github.com/yourmodule/agentquery"
)

// NewQueryCommand creates a Cobra command that uses the given schema.
func NewQueryCommand[T any](schema *agentquery.Schema[T]) *cobra.Command {
    return &cobra.Command{
        Use:   "q <query>",
        Short: "Query with DSL",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            result, err := schema.Query(args[0])
            if err != nil {
                return err
            }
            return json.NewEncoder(os.Stdout).Encode(result)
        },
    }
}
```

**Important: This is a single go.mod, NOT a multi-module repo.** Go modules only pulls dependencies of imported packages. Since `cobraext/` is a subpackage (not a submodule), its dependency on Cobra is listed in the root `go.mod` but only downloaded/compiled when someone imports `cobraext/`.

The key point: **having `cobra` in `go.mod` is fine if only `cobraext/` uses it.** Users who don't import `cobraext/` will have Cobra in their `go.sum` but it won't be compiled or linked. For stricter isolation (not even in `go.sum`), use a multi-module setup with a separate `go.mod` in `cobraext/`, but this adds complexity and is usually unnecessary.

### Versioning and go.mod Considerations

- Use semantic versioning: `v0.x.y` during initial development, `v1.0.0` when API stabilizes.
- Module path: `github.com/yourorg/agentquery`
- For v2+: append `/v2` to module path per Go convention.
- Minimum Go version: `go 1.21` or higher (generics require 1.18+, but recent versions have better generic support).
- Keep dependencies minimal. The core library should only need the standard library. Cobra is the only external dep, isolated in `cobraext/`.

---

## Recommendation: Proposed API Design

Based on all research above, here is the recommended public API for the library:

```go
package agentquery

// --- Core types ---

// FieldAccessor extracts a field value from a domain item.
type FieldAccessor[T any] func(item T) any

// OperationHandler handles a parsed query statement against a data source.
// The handler receives the parsed statement and returns structured results.
type OperationHandler[T any] func(ctx OperationContext[T]) (any, error)

// OperationContext is passed to operation handlers.
type OperationContext[T any] struct {
    Statement  Statement           // Parsed statement (operation, args, field list)
    Selector   *FieldSelector[T]   // Pre-built field selector from the statement's projection
    // Domain-specific data source is accessed via closure in the handler.
}

// FieldSelector applies field projection to domain items.
type FieldSelector[T any] struct { /* internal */ }

func (fs *FieldSelector[T]) Apply(item T) map[string]any { /* ... */ }
func (fs *FieldSelector[T]) Include(field string) bool   { /* ... */ }

// --- Schema ---

// Schema defines the query API for a domain type T.
type Schema[T any] struct { /* internal */ }

func NewSchema[T any]() *Schema[T]

// Registration
func (s *Schema[T]) Field(name string, accessor FieldAccessor[T])
func (s *Schema[T]) Preset(name string, fields ...string)
func (s *Schema[T]) DefaultFields(fields ...string)
func (s *Schema[T]) Operation(name string, handler OperationHandler[T])

// Query execution
func (s *Schema[T]) Query(input string) (any, error)

// Field selector creation (for use by operation handlers)
func (s *Schema[T]) NewSelector(fields []string) (*FieldSelector[T], error)

// Search (pluggable)
type SearchMatch struct {
    Path    string
    Line    int
    Content string
    Context []string // optional surrounding lines
}

type Searcher func(pattern string, opts SearchOptions) ([]SearchMatch, error)

type SearchOptions struct {
    FileGlob        string
    CaseInsensitive bool
    ContextLines    int
}

func (s *Schema[T]) SetSearcher(fn Searcher)
func (s *Schema[T]) Search(pattern string, opts ...SearchOption) ([]SearchMatch, error)
```

### Why This Design

1. **Generic on `T` (domain type):** Type-safe accessors. A `Schema[*board.Element]` only accepts `FieldAccessor[*board.Element]` functions.

2. **Operations are just functions:** No interface to implement, no struct to define. Matches the chi/graphql-go pattern.

3. **FieldSelector is pre-built from the statement:** Operation handlers don't parse projection -- they receive a ready-to-use selector. This centralizes validation and preset expansion.

4. **Search is pluggable via `SetSearcher`:** Different domains have different storage (filesystem, database, API). The library provides the Search method signature; the user provides the implementation.

5. **Parser is internal:** Users call `schema.Query(string)`, never `ParseQuery()` directly. The parser is an implementation detail.

6. **Minimal dependencies:** Core package uses only stdlib. Cobra helper in `cobraext/` subpackage.

### Migration Path from Current Implementation

The existing code maps cleanly to this design:

| Current (task-board) | Generic library |
|---------------------|-----------------|
| `fields.ValidFields` map | `schema.Field()` registrations |
| `fields.Presets` map | `schema.Preset()` registrations |
| `validOperations` map | `schema.Operation()` registrations |
| `fields.Selector.Apply()` | `FieldSelector[T].Apply()` |
| `ParseQuery()` | Internal, called by `schema.Query()` |
| `execStatement()` switch | Dispatch to registered `OperationHandler` |
| `grepBoard()` | User-provided `Searcher` function |

---

## Sources

- [go-chi/chi - Router interface](https://github.com/go-chi/chi)
- [spf13/cobra - Command registration](https://github.com/spf13/cobra)
- [graphql-go/graphql - Field definitions and resolvers](https://pkg.go.dev/github.com/graphql-go/graphql)
- [expr-lang/expr - Functional options, Env integration](https://github.com/expr-lang/expr)
- [uber-go/fx - Provider registration pattern](https://github.com/uber-go/fx)
- [Go modules layout](https://go.dev/doc/modules/layout)
- [Functional options pattern in Go](https://www.sohamkamani.com/golang/options-pattern/)
- [When to use generics](https://go.dev/blog/when-generics)
- [Generics vs. reflection performance](https://blog.stackademic.com/generics-and-reflection-in-go-a-comparative-guide-1648cdd46381)
