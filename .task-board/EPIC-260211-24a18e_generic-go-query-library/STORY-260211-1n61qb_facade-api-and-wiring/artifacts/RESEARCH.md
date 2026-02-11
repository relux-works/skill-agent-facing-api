# Research: Unified Facade for DSL Queries + Full-Text Search

## Context

This research explores API design for a Go library facade that combines two read methods — structured DSL queries and full-text regex search — under a single constructor. The facade is the sole import for all read operations against a data directory.

The reference implementation is `task-board` CLI: `cmd/query.go` + `cmd/query_exec.go` (DSL) and `cmd/grep.go` (search). The goal is to extract these into a generic, importable library where the user defines their own domain model, fields, operations, and data format.

---

## 1. Facade API Design

### 1.1 Constructor Patterns Evaluated

**Functional options (recommended):**

```go
package agentapi

type Engine struct {
    dataDir    string
    extensions []string
    fields     *FieldRegistry
    ops        *OperationRegistry
}

type Option func(*Engine)

func New(dataDir string, opts ...Option) (*Engine, error) {
    e := &Engine{
        dataDir:    dataDir,
        extensions: []string{".md"},
    }
    for _, opt := range opts {
        opt(e)
    }
    if err := e.validate(); err != nil {
        return nil, err
    }
    return e, nil
}

func WithExtensions(exts ...string) Option {
    return func(e *Engine) {
        e.extensions = exts
    }
}

func WithFields(reg *FieldRegistry) Option {
    return func(e *Engine) {
        e.fields = reg
    }
}

func WithOperations(reg *OperationRegistry) Option {
    return func(e *Engine) {
        e.ops = reg
    }
}
```

**Builder pattern (rejected):**

```go
engine, err := agentapi.NewBuilder().
    DataDir(".task-board").
    Extensions(".md", ".yaml").
    Fields(reg).
    Build()
```

Builders add a separate type (`Builder`) with no clear benefit over functional options for this case. The construction is not complex enough to warrant it — there are no conditional steps, no ordering constraints. Functional options are the standard Go idiom (see: `grpc.NewServer`, `zap.New`, `http.Server` pattern).

**Why not a plain struct literal:**

```go
engine := &agentapi.Engine{DataDir: ".task-board"}
```

Exported fields leak internal state. The Engine needs validation (is dataDir non-empty? do extensions have dots?). Constructor with options is the only way to guarantee a valid instance.

### 1.2 How Routers Handle It

**chi:** Single `chi.NewRouter()` returns `*Mux` that serves as both registration surface and executor. Routes and middleware are registered on the same struct, then `ServeHTTP` dispatches.

**gin:** Same pattern — `gin.Default()` returns `*Engine` that accumulates routes and middleware, then serves.

**Relevant insight:** Both use a single struct that accumulates registrations, then dispatches. The facade should follow this: `Engine` accumulates fields, presets, and operations, then `Query()` and `Search()` dispatch.

### 1.3 Proposed Top-Level API

```go
engine, err := agentapi.New(".task-board",
    agentapi.WithExtensions(".md"),
    agentapi.WithFields(fields),
    agentapi.WithOperations(ops),
)

// Structured DSL query — returns JSON-serializable result
result, err := engine.Query("get(TASK-42) { status assignee }")

// Full-text regex search — returns match list
matches, err := engine.Search("authentication", agentapi.SearchOpts{
    FileGlob:        "progress.md",
    CaseInsensitive: true,
    ContextLines:    2,
})

// Raw JSON bytes (convenience)
jsonBytes, err := engine.QueryJSON("list(type=task) { overview }")
```

Two methods, one facade. `Query` returns `interface{}` (the user's domain data, JSON-marshallable). `Search` returns `[]SearchResult` (always the same shape regardless of domain).

---

## 2. Search Result Types

### 2.1 Current Implementation

`cmd/grep.go` uses:

```go
type GrepMatch struct {
    Path    string `json:"path"`    // relative to board dir
    Line    int    `json:"line"`    // 1-indexed
    Content string `json:"content"` // matched line text
}
```

`assets/scoped-grep.go` (the reference template) uses the same shape but named `Match`.

### 2.2 Proposed Library Type

```go
// SearchResult represents a single full-text match.
type SearchResult struct {
    Source  Source `json:"source"`
    Content string `json:"content"`
    IsMatch bool   `json:"isMatch"` // false for context-only lines
}

// Source identifies where a match was found.
type Source struct {
    Path string `json:"path"` // relative to data directory
    Line int    `json:"line"` // 1-indexed
}
```

**Why split Source from Content:**
- Source is reusable metadata (can be used for linking, deduplication, sorting).
- `IsMatch` distinguishes actual matches from context lines in `-C N` output. The current implementation conflates them — when context is enabled, context lines appear as matches with no way to tell which line actually matched.

**Path is relative to data directory:** Absolute paths leak host filesystem details into agent context (wasted tokens, potentially sensitive). Relative paths are deterministic and compact.

**JSON-marshallable:** Yes. The `json` struct tags ensure `json.Marshal` works directly. DSL returns `interface{}` (maps/slices), Search returns `[]SearchResult` — both serialize cleanly.

### 2.3 Search Response Envelope

For consistency with DSL output (which wraps lists in `{"elements": [...], "count": N}`):

```go
// SearchResponse wraps search results with metadata.
type SearchResponse struct {
    Matches []SearchResult `json:"matches"`
    Count   int            `json:"count"`
    Pattern string         `json:"pattern"`
}
```

The `Search()` method returns `[]SearchResult` directly (the caller can wrap if needed). A convenience method `SearchJSON()` returns the envelope:

```go
func (e *Engine) Search(pattern string, opts SearchOpts) ([]SearchResult, error)
func (e *Engine) SearchJSON(pattern string, opts SearchOpts) (*SearchResponse, error)
```

---

## 3. Configuration Unification

### 3.1 Shared Data Directory

Both DSL operations and Search need the data directory path. It is set once at construction:

```go
engine, _ := agentapi.New("/path/to/.task-board", ...)
```

Operations receive it via the `Engine` (which they have access to through closures or an explicit context parameter). Search walks it directly.

### 3.2 File Extension Filters

Configured once:

```go
agentapi.WithExtensions(".md", ".yaml")
```

Search uses these as a whitelist for `filepath.WalkDir`. DSL operations typically do not walk the filesystem directly (they work with loaded domain objects), but if an operation needs to read auxiliary files (like `notes` reading from `progress.md`), it can access `engine.DataDir()`.

### 3.3 Should DSL Operations Access the Filesystem?

**Yes, but indirectly.** The reference implementation (`fields.go` lines 163-169) already does this:

```go
if fs.Include("notes") {
    pd, err := board.ParseProgressFile(elem.ProgressPath())
    // ...
}
```

The "notes" field lazily reads a file. In the generic library, this is the user's responsibility: their field accessor function can read files if needed. The library provides `engine.DataDir()` so the accessor knows where to look:

```go
fields.Register("notes", func(item interface{}) interface{} {
    elem := item.(*MyElement)
    data, _ := os.ReadFile(filepath.Join(elem.Dir, "notes.md"))
    return string(data)
})
```

The facade does not enforce filesystem isolation — it provides the data directory as configuration, and the user's code decides what to read.

---

## 4. Cobra Integration

### 4.1 The Problem

The library should not force Cobra as a dependency. Consumers who use a different CLI framework (or no framework at all — e.g., MCP server, HTTP handler) should not pull in `github.com/spf13/cobra`.

### 4.2 Separate Subpackage

```
agentapi/              # core library, zero external deps
agentapi/cobra/        # Cobra integration helpers
```

The `agentapi/cobra` package imports both `agentapi` and `cobra`:

```go
package cobra

import (
    "github.com/yourorg/agentapi"
    "github.com/spf13/cobra"
)

// QueryCommand returns a Cobra command that wires engine.Query().
func QueryCommand(engine *agentapi.Engine) *cobra.Command {
    return &cobra.Command{
        Use:   "q <query>",
        Short: "Query with DSL",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            result, err := engine.QueryJSON(args[0])
            if err != nil {
                return err // or JSON error, depending on output mode
            }
            _, err = cmd.OutOrStdout().Write(result)
            return err
        },
    }
}

// SearchCommand returns a Cobra command that wires engine.Search().
func SearchCommand(engine *agentapi.Engine) *cobra.Command {
    var fileGlob string
    var caseInsensitive bool
    var contextLines int

    cmd := &cobra.Command{
        Use:   "grep <pattern>",
        Short: "Full-text search",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            matches, err := engine.Search(args[0], agentapi.SearchOpts{
                FileGlob:        fileGlob,
                CaseInsensitive: caseInsensitive,
                ContextLines:    contextLines,
            })
            if err != nil {
                return err
            }
            return agentapi.PrintJSON(cmd.OutOrStdout(), matches)
        },
    }

    cmd.Flags().StringVar(&fileGlob, "file", "", "Filter by filename glob")
    cmd.Flags().BoolVarP(&caseInsensitive, "ignore-case", "i", false, "Case-insensitive")
    cmd.Flags().IntVarP(&contextLines, "context", "C", 0, "Context lines")

    return cmd
}
```

### 4.3 Single vs Separate Commands

**Separate commands** (recommended). Reasons:
- `q` and `grep` have fundamentally different argument signatures (DSL string vs regex pattern + flags).
- Combining them into one command would require subcommand syntax or mode flags, adding complexity.
- Separate commands match the existing `task-board` CLI pattern and the SKILL.md decision table (DSL for structured, grep for text).

### 4.4 Combo Helper

For quick wiring:

```go
// AddCommands adds both q and grep commands to a parent command.
func AddCommands(parent *cobra.Command, engine *agentapi.Engine) {
    parent.AddCommand(QueryCommand(engine))
    parent.AddCommand(SearchCommand(engine))
}
```

---

## 5. Error Handling Consistency

### 5.1 Current Pattern

`task-board` uses different error formats depending on context:
- CLI text mode: `fmt.Errorf` returned to Cobra (printed to stderr).
- JSON mode: `output.PrintError(os.Stderr, code, message, details)` with structured `{"error": {"code": "...", "message": "...", "details": {}}}`.
- Batch queries: per-statement error objects `{"error": {"message": "..."}}` mixed into the result array.

### 5.2 Library Error Type

The library should define its own error type that is JSON-serializable:

```go
// Error represents a structured query/search error.
type Error struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

func (e *Error) Error() string {
    return e.Message
}

// Predefined error codes
const (
    ErrParse      = "PARSE_ERROR"
    ErrNotFound   = "NOT_FOUND"
    ErrValidation = "VALIDATION_ERROR"
    ErrInternal   = "INTERNAL_ERROR"
)
```

This satisfies the `error` interface (so it works with standard Go error handling) and is also JSON-marshallable (so Cobra helpers can serialize it directly).

### 5.3 Batch Error Handling

Preserve the current pattern: in batch mode, errors are per-statement objects in the result array, and the batch as a whole succeeds. This matches `query_exec.go` lines 26-34:

```go
results[i] = map[string]interface{}{
    "error": map[string]interface{}{
        "message": err.Error(),
    },
}
```

The library's `Query()` for batch queries should return `[]interface{}` where each element is either the operation result or an `*Error` struct. The caller (or the `QueryJSON` convenience method) handles serialization.

### 5.4 Search Errors

Search is simpler — no batch semantics. Errors are straightforward:

```go
matches, err := engine.Search("invalid[regex", opts)
// err is *agentapi.Error{Code: "PARSE_ERROR", Message: "invalid regex: ..."}
```

---

## 6. Real-World Go Facade Precedents

### 6.1 database/sql

- `sql.Open(driver, dsn)` — single constructor, returns `*DB`.
- `*DB` is the facade: `.Query()`, `.Exec()`, `.Prepare()`, `.Begin()`.
- Driver registration is global (`sql.Register(name, driver)`) — our equivalent is operation/field registration.
- Key insight: `*DB` does not expose driver internals. Similarly, `*Engine` should not expose parser internals.

**Adaptation:** Our `New()` is like `sql.Open()`. Operations are like prepared statements — registered ahead of time, dispatched at query time.

### 6.2 net/http

- `http.NewServeMux()` returns `*ServeMux`.
- `.Handle()` / `.HandleFunc()` register routes.
- `.ServeHTTP()` dispatches.
- Registration and dispatch on the same type.

**Adaptation:** Our `Engine` accumulates operations (via `WithOperations()`), then `Query()` dispatches to them based on the parsed AST operation name.

### 6.3 Key Patterns Extracted

| Aspect | database/sql | net/http | Our Library |
|--------|-------------|----------|-------------|
| Constructor | `Open(driver, dsn)` | `NewServeMux()` | `New(dataDir, opts...)` |
| Registration | `Register(name, driver)` global | `.Handle(pattern, handler)` | `WithOperations(reg)` at construction |
| Dispatch | `db.Query(sql)` | `mux.ServeHTTP(w, r)` | `engine.Query(dsl)` |
| Error handling | `error` interface | `error` interface | `*Error` (implements error, JSON-serializable) |
| Extension | New drivers | New handlers | New operations |

### 6.4 Registration Timing

Both `database/sql` and `net/http` allow registration after construction. For simplicity, our library requires all registration at construction time (via options). This is a deliberate constraint:

- Operations and fields are static for the lifetime of the engine (they do not change at runtime).
- Registering after construction requires synchronization or builder-like "freeze" semantics — unnecessary complexity.
- The functional options pattern naturally enforces "configure, then use."

If we later need runtime registration (unlikely), we can add `engine.RegisterOperation()` with a mutex. But not in v1.

---

## 7. Package Structure

```
agentapi/
├── engine.go          # Engine struct, New(), Query(), Search()
├── options.go         # Functional options (WithExtensions, WithFields, etc.)
├── error.go           # Error type, error codes
├── search.go          # SearchResult, SearchOpts, search implementation
├── fields/
│   ├── registry.go    # FieldRegistry: Register(name, accessor), presets
│   └── selector.go    # Selector: NewSelector(fields) → Include(field) bool
├── query/
│   ├── parser.go      # Tokenizer + recursive descent (domain-agnostic)
│   ├── ast.go         # Query, Statement, Arg types
│   └── exec.go        # Dispatch: statement → operation handler → field projection
├── ops/
│   └── registry.go    # OperationRegistry, OperationHandler type, Register()
├── cobra/
│   └── commands.go    # QueryCommand(), SearchCommand(), AddCommands()
└── json.go            # PrintJSON, MarshalJSON helpers
```

**Zero external dependencies in the core.** Only `agentapi/cobra` imports `github.com/spf13/cobra`.

---

## 8. Full API Surface (Proposed Signatures)

```go
package agentapi

// --- Core ---

type Engine struct { /* unexported fields */ }

func New(dataDir string, opts ...Option) (*Engine, error)

func (e *Engine) Query(dsl string) (interface{}, error)
func (e *Engine) QueryJSON(dsl string) ([]byte, error)
func (e *Engine) Search(pattern string, opts SearchOpts) ([]SearchResult, error)
func (e *Engine) SearchJSON(pattern string, opts SearchOpts) ([]byte, error)
func (e *Engine) DataDir() string

// --- Options ---

type Option func(*Engine)

func WithExtensions(exts ...string) Option
func WithFields(reg *fields.Registry) Option
func WithOperations(reg *ops.Registry) Option

// --- Search ---

type SearchOpts struct {
    FileGlob        string
    CaseInsensitive bool
    ContextLines    int
}

type SearchResult struct {
    Source  Source `json:"source"`
    Content string `json:"content"`
    IsMatch bool   `json:"isMatch"`
}

type Source struct {
    Path string `json:"path"`
    Line int    `json:"line"`
}

// --- Errors ---

type Error struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

func (e *Error) Error() string

// --- Fields (subpackage) ---

package fields

type Accessor func(item interface{}) interface{}

type Registry struct { /* unexported */ }

func NewRegistry() *Registry
func (r *Registry) Register(name string, accessor Accessor)
func (r *Registry) Preset(name string, fieldNames []string)
func (r *Registry) NewSelector(requested []string) (*Selector, error)

type Selector struct { /* unexported */ }

func (s *Selector) Include(name string) bool
func (s *Selector) Apply(item interface{}) map[string]interface{}

// --- Operations (subpackage) ---

package ops

type Handler func(ctx *Context) (interface{}, error)

type Context struct {
    Args     []Arg
    Selector *fields.Selector
    DataDir  string
    // User injects their domain loader here
    Data     interface{} // e.g., *Board, *Config, whatever the user loaded
}

type Arg struct {
    Key   string
    Value string
}

type Registry struct { /* unexported */ }

func NewRegistry() *Registry
func (r *Registry) Register(name string, handler Handler)
func (r *Registry) Has(name string) bool

// --- Cobra (subpackage) ---

package cobra

func QueryCommand(engine *agentapi.Engine) *cobra.Command
func SearchCommand(engine *agentapi.Engine) *cobra.Command
func AddCommands(parent *cobra.Command, engine *agentapi.Engine)
```

---

## 9. Open Questions

### 9.1 Where Does Domain Data Loading Happen?

The `task-board` pattern loads the board on every query (`board.Load(boardDir)`). For the generic library, should the Engine reload on every `Query()` call, or should the user pass pre-loaded data?

**Option A: User loads, passes via operation context.** The Engine does not know how to load domain data. The user's operation handlers receive data through the `Context.Data` field, and the user sets it before calling `Query()`. Problem: clunky API — the user has to load-then-query every time.

**Option B: User registers a loader function.** The Engine calls `loader()` before each query to get fresh data:

```go
func WithLoader(fn func(dataDir string) (interface{}, error)) Option
```

This is cleaner. The Engine calls the loader inside `Query()`, passes the result to operation handlers. The user's operation handlers cast `ctx.Data` to their domain type.

**Recommendation: Option B.** It matches the `task-board` pattern (load fresh on every command) and keeps the API simple.

### 9.2 Generic Apply vs Type-Specific Apply

The reference `Selector.Apply()` takes `*board.Element` — a concrete type. The library needs a generic version. Two approaches:

**Approach A: Accessor functions (recommended).** Each field has a registered `func(item interface{}) interface{}`. The Selector calls them:

```go
func (s *Selector) Apply(item interface{}) map[string]interface{} {
    result := make(map[string]interface{})
    for name := range s.fields {
        if accessor, ok := s.registry.accessors[name]; ok {
            result[name] = accessor(item)
        }
    }
    return result
}
```

**Approach B: Reflection.** Auto-map struct fields to JSON keys. Too magic, error-prone with custom logic (like the "notes" field that reads a file), and prevents lazy evaluation.

### 9.3 Should Search Return Domain Objects?

Current `task-board` has two search commands:
- `grep` — returns raw `path:line:content` matches (file-level).
- `search` — returns domain-level matches (`ID, Type, Name, Status, MatchField, MatchContext`).

The library's `Search()` is the `grep` equivalent (file-level). A domain-aware search would be an operation registered by the user:

```go
ops.Register("search", func(ctx *Context) (interface{}, error) {
    // User's custom search that correlates grep matches with domain objects
})
```

This keeps the library's `Search()` simple and domain-agnostic.

---

## Recommendation

1. **Use functional options with `New(dataDir, opts...)`** — standard Go idiom, validated at construction, zero-dependency core.

2. **SearchResult with Source struct** — split `{Path, Line}` into a Source substruct. Add `IsMatch` bool to distinguish matches from context lines.

3. **Relative paths in results** — always relative to data directory. Compact, deterministic, agent-friendly.

4. **Separate `agentapi/cobra` subpackage** — Cobra integration as opt-in. `QueryCommand()` + `SearchCommand()` + `AddCommands()` combo helper.

5. **Custom `Error` type implementing `error` interface** — JSON-serializable, consistent between Query and Search. Batch errors are per-statement in the result array.

6. **Loader function option** — `WithLoader(fn)` so the Engine can load domain data on each `Query()` call. Search does not need it (operates on files directly).

7. **Accessor-based field projection** — `func(item interface{}) interface{}` per field. No reflection. Lazy evaluation for expensive fields.

8. **Two public methods on Engine** — `Query(dsl)` and `Search(pattern, opts)`. Plus `QueryJSON()` and `SearchJSON()` convenience methods that return `[]byte`.

9. **Operations registered at construction time** — no runtime mutation. Keeps the API simple and thread-safe without mutexes.
