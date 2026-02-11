# Research: Making the Recursive Descent DSL Parser Generic and Reusable

**Date:** 2026-02-11
**Scope:** Extracting the task-board query parser into a standalone Go package
**Source files analyzed:**
- `tools/board-cli/cmd/query_parser.go` — tokenizer + recursive descent parser
- `tools/board-cli/cmd/query_exec.go` — operation dispatch and execution
- `tools/board-cli/internal/fields/fields.go` — field validation and projection

---

## 1. Parser Configuration: Operations and Validation

### Current State

Operations are hardcoded in `query_parser.go`:

```go
var validOperations = map[string]bool{
    "get":     true,
    "list":    true,
    "summary": true,
    "plan":    true,
    "agents":  true,
}
```

Field validation is also hardcoded — `parseProjection()` directly calls into `fields.ValidFields` and `fields.Presets`:

```go
if preset, ok := fields.Presets[name]; ok {
    fieldNames = append(fieldNames, preset...)
    continue
}
if !fields.ValidFields[name] {
    return nil, fmt.Errorf("unknown field %q at position %d", name, tok.pos)
}
```

This couples the parser to the task-board domain. A generic library must not know about "TASK-12" or "overview" presets.

### Approach: Split Syntax Validation from Semantic Validation

**The parser should validate syntax only.** Semantic validation (is this operation known? is this field valid? does this preset exist?) belongs to the executor or a separate validation pass.

This matches how real-world parsers work:
- **PromQL** parser builds the AST first, then a separate `checkAST()` pass validates function signatures, label matchers, type compatibility
- **Go's own parser** parses any syntactically valid Go into an AST; semantic checking happens in `go/types`
- **LogQL** sanitizes identifiers post-parse, not during lexing

#### Proposed interface — operation registry:

```go
// ParserConfig controls which syntactic features are enabled.
// It does NOT validate semantics — that's the executor's job.
type ParserConfig struct {
    // Operations restricts which operation names the parser accepts.
    // If nil, any identifier is accepted as an operation name.
    Operations map[string]bool
}
```

Using `nil` to mean "accept anything" is the key insight. A fully generic parser accepts any operation name and lets the executor reject unknown ones. But some users want early rejection at parse time — the option should exist, not be forced.

#### Proposed interface — field/preset resolution as a callback:

```go
// FieldResolver is called during projection parsing to expand presets
// and validate field names. If nil, the parser accepts all identifiers
// as field names without expansion or validation.
type FieldResolver interface {
    // ResolveField checks if a name is a valid field, a preset, or unknown.
    // Returns:
    //   - ([]string{name}, nil)          for a valid field
    //   - ([]string{f1, f2, ...}, nil)   for a preset (expanded)
    //   - (nil, error)                   for an unknown name
    ResolveField(name string) ([]string, error)
}
```

This is better than passing raw maps because:
1. It allows computed presets (a preset that depends on context)
2. It allows case-insensitive matching without mutating state
3. It's testable — you can mock it
4. The parser stays import-free from domain packages

#### Should the parser know about presets?

**No.** The parser sees identifiers inside `{ }`. Whether those identifiers are fields, presets, or aliases is the resolver's problem. The parser's job is: "collect identifiers from the projection block, optionally pass each through a resolver." If no resolver is provided, the AST stores raw identifier strings and the executor expands them.

This is the Unix philosophy: the parser is a pipeline stage that transforms text into structure. It does not interpret the structure.

---

## 2. AST Design for Reusability

### Current AST

```go
type Query struct {
    Statements []Statement
}

type Statement struct {
    Operation string
    Args      []Arg
    Fields    []string // already resolved if presets were expanded
}

type Arg struct {
    Key   string // empty for positional
    Value string
}
```

### Analysis

The current AST is **minimal and sufficient** for the function-call DSL pattern: `operation(args...) { fields... }`. It handles:
- Single and batch queries (`;` separator)
- Positional and key-value arguments
- Optional field projection

### Do we need expression types?

**Not yet.** The DSL is intentionally simple — it's a query language for agents, not a programming language. Adding nested queries or boolean operators would:
1. Increase parser complexity dramatically (precedence, associativity)
2. Make the AST harder to serialize to JSON
3. Violate the "compact queries over a wire" design goal

If future needs arise (e.g., `list(status=dev OR status=review)`), the better path is **filter expressions inside argument values**, not full expression trees. The parser already supports string arguments: `list(filter="status in (dev, review)")` — the executor interprets the filter string.

### Proposed AST changes for the generic library

```go
// Query is the top-level AST node.
type Query struct {
    Statements []Statement
}

// Statement represents a single operation call.
type Statement struct {
    Operation string   // operation name (e.g., "get", "list")
    Args      []Arg    // positional and key-value arguments
    Fields    []string // raw field names from projection (NOT resolved)
    Pos       Pos      // position of the operation name in input
}

// Arg is a positional value or key=value pair.
type Arg struct {
    Key   string // empty for positional args
    Value string
    Pos   Pos    // position in input
}

// Pos represents a position in the input string.
type Pos struct {
    Offset int // byte offset from start of input
    Line   int // 1-based line number
    Column int // 1-based column number (bytes, not runes — keep it simple)
}
```

Key changes:
1. **`Fields` stores raw identifiers** — no preset expansion at parse time. The AST is a faithful representation of the input text.
2. **`Pos` on every node** — enables structured error reporting.
3. **No new node types** — keep the AST flat and JSON-serializable.

The `Pos` struct is intentionally minimal. PromQL uses `PositionRange` with Start/End, but for our single-token identifiers, a single position is enough. If someone needs range, they can compute `end = offset + len(token)`.

---

## 3. Identifier Format Flexibility

### Current Rules

```go
func isIdentStart(ch byte) bool {
    return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
           (ch >= '0' && ch <= '9') || ch == '_'
}

func isIdentChar(ch byte) bool {
    return isIdentStart(ch) || ch == '-'
}
```

This is **permissive by design** — allows identifiers like `TASK-260211-abc`, `status`, `3`, `my_field`. It's more permissive than most languages (digits can start identifiers) because element IDs like `T1` or `TASK-12` need to be first-class.

### How other DSLs handle this

| DSL | Identifier rules | Notes |
|-----|------------------|-------|
| **PromQL** | `[a-zA-Z_:][a-zA-Z0-9_:]*` | Standard metric name rules, no hyphens |
| **LogQL** | Same as PromQL for labels; log lines are arbitrary | Sanitizes extracted labels to `[a-zA-Z0-9_:]` |
| **SQL** | `[a-zA-Z_][a-zA-Z0-9_]*` | Backticks/quotes for special chars |
| **GraphQL** | `[a-zA-Z_][a-zA-Z0-9_]*` | Strict, no hyphens |
| **jq** | `.identifier` with `.[" "]` for special | Quoting for non-standard names |
| **Our DSL** | `[a-zA-Z0-9_][a-zA-Z0-9_-]*` | Permissive: digits start, hyphens allowed |

### Should it be configurable?

**The permissive default is good enough.** Here's why:

1. **Superset is safe.** Our rules accept everything PromQL/GraphQL accept, plus more. No one will have identifiers that our tokenizer rejects but needs.
2. **Configurable char sets add complexity for near-zero gain.** The tokenizer is ~20 lines. Making it configurable means function pointers or interfaces for character classification — engineering overhead for an unlikely use case.
3. **Quoted strings exist as an escape hatch.** If someone has identifiers with spaces or special chars, they use `"my weird field"`.

**One potential issue:** digits starting identifiers means `123` is a valid identifier, not a number literal. This is fine for our DSL (we have no arithmetic), but if the library were used for a DSL with numeric literals, the parser couldn't distinguish `get(42)` (number arg) from `get(status)` (ident arg) at the token level.

**Proposed approach:** Keep the permissive default. Add a single escape hatch:

```go
type ParserConfig struct {
    // ...
    // IdentChecker overrides the default identifier character rules.
    // If nil, the default permissive rules are used (letters, digits, _, -).
    IdentChecker IdentChecker
}

// IdentChecker controls identifier tokenization rules.
type IdentChecker interface {
    IsIdentStart(ch byte) bool
    IsIdentChar(ch byte) bool
}
```

Most users will never set this. But if someone builds a DSL where numbers must be distinct from identifiers, they can provide stricter rules.

---

## 4. Error Handling and Reporting

### Current State

Errors are plain `fmt.Errorf` strings with byte offset:

```go
return nil, fmt.Errorf("unknown operation %q at position %d", opTok.val, opTok.pos)
return nil, fmt.Errorf("expected %s at position %d, got %q", tokenTypeName(typ), tok.pos, tok.val)
```

This works for single-line queries but fails for multi-line input and agent consumption (parsing `"position 42"` to extract the number is fragile).

### Proposed: Structured ParseError Type

```go
// ParseError is a structured error returned by the parser.
type ParseError struct {
    Message  string `json:"message"`
    Pos      Pos    `json:"pos"`
    Got      string `json:"got,omitempty"`      // what was found
    Expected string `json:"expected,omitempty"` // what was expected
}

func (e *ParseError) Error() string {
    if e.Pos.Line > 0 {
        return fmt.Sprintf("%d:%d: %s", e.Pos.Line, e.Pos.Column, e.Message)
    }
    return fmt.Sprintf("offset %d: %s", e.Pos.Offset, e.Message)
}
```

Key design decisions:

1. **Implements `error` interface** — drop-in replacement, callers can use `errors.As()` to get the structured form.
2. **Line/column computed from offset** — the tokenizer tracks newlines as it scans. This is trivial: maintain a `lineOffsets []int` in the tokenizer, then `Pos` is computed from `offset` using binary search.
3. **JSON-serializable** — agents consuming the parser output can parse errors as JSON objects. The `json` tags on `ParseError` enable `json.Marshal(err)` directly.
4. **`Got`/`Expected` fields** — enable agents to programmatically understand what went wrong without parsing the message string.

### Multi-line support

The tokenizer already skips `\n` as whitespace. To add line tracking:

```go
type tokenizer struct {
    input       string
    pos         int
    tokens      []token
    lineStarts  []int  // byte offsets where each line starts
}

func (t *tokenizer) currentPos() Pos {
    line := sort.SearchInts(t.lineStarts, t.pos)
    col := t.pos - t.lineStarts[line-1] + 1
    return Pos{Offset: t.pos, Line: line, Column: col}
}
```

### Error accumulation vs fail-fast

The current parser fails on the first error. PromQL accumulates errors and returns all of them. For a mini-DSL consumed by agents, **fail-fast is correct**:
- Agents will fix one error and retry
- Accumulated errors in a 50-char query are confusing, not helpful
- Simpler implementation

---

## 5. Testing Strategy for a Generic Parser

### Table-Driven Tests (primary strategy)

Table-driven tests are the backbone. The key is parameterizing both the input AND the parser config:

```go
func TestParse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        config  *ParserConfig // nil = default
        want    *Query
        wantErr string
    }{
        {
            name:  "simple get",
            input: "get(T1) { status }",
            want: &Query{Statements: []Statement{{
                Operation: "get",
                Args:      []Arg{{Value: "T1"}},
                Fields:    []string{"status"},
            }}},
        },
        {
            name:    "unknown op rejected with config",
            input:   "foo()",
            config:  &ParserConfig{Operations: map[string]bool{"get": true}},
            wantErr: "unknown operation",
        },
        {
            name:  "unknown op accepted without config",
            input: "foo()",
            want:  &Query{Statements: []Statement{{Operation: "foo"}}},
        },
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cfg := tt.config
            if cfg == nil {
                cfg = DefaultConfig()
            }
            got, err := Parse(tt.input, cfg)
            if tt.wantErr != "" {
                if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
                    t.Errorf("want error containing %q, got %v", tt.wantErr, err)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got %+v, want %+v", got, tt.want)
            }
        })
    }
}
```

### Test Categories

1. **Tokenizer tests** — exercise character classification, string escaping, edge cases (empty input, only whitespace, unterminated strings)
2. **Parser tests** — valid queries across all syntactic forms (single, batch, with/without args, with/without fields, key-value vs positional, mixed)
3. **Config tests** — restricted operations, custom ident rules, field resolver callbacks
4. **Error tests** — every error path has a test confirming the error message and position
5. **Roundtrip tests** — parse then serialize back to string, compare (requires `String()` on AST)

### Fuzzing (critical for parser robustness)

Go's built-in fuzzing is perfect for parsers:

```go
func FuzzParse(f *testing.F) {
    // Seed corpus from real queries
    f.Add("get(T1) { status }")
    f.Add("list(type=task, status=dev) { overview }")
    f.Add("summary()")
    f.Add("get(T1) { status }; get(T2) { status }")
    f.Add("")
    f.Add(";;;")
    f.Add(`get("hello world")`)

    f.Fuzz(func(t *testing.T, input string) {
        // Must not panic — this is the primary invariant
        q, err := Parse(input, nil)
        if err != nil {
            return // errors are fine, panics are not
        }
        // If parsing succeeds, the AST must be non-nil with >0 statements
        if q == nil || len(q.Statements) == 0 {
            t.Error("Parse returned nil/empty query without error")
        }
    })
}
```

Fuzzing catches: panics on unexpected input, index-out-of-bounds in tokenizer, infinite loops on malformed input, UTF-8 edge cases.

### Testing with Different Registrations

Use test helper factories:

```go
func configWithOps(ops ...string) *ParserConfig {
    m := make(map[string]bool)
    for _, op := range ops {
        m[op] = true
    }
    return &ParserConfig{Operations: m}
}

func resolverFromMap(fields map[string]bool, presets map[string][]string) FieldResolver {
    return &mapResolver{fields: fields, presets: presets}
}
```

Then table tests reference these helpers to create different configurations per test case.

---

## 6. Reference: How Real Parsers Solve These Problems

### PromQL (Prometheus)

- **Parser:** yacc-generated with hand-written semantic actions
- **AST:** Rich node hierarchy with expression types, visitor pattern
- **Operations:** Function map injected via `newParserWithFunctions()`
- **Identifiers:** Strict `[a-zA-Z_:][a-zA-Z0-9_:]*`
- **Errors:** Structured `ParseErr` with `PositionRange`, accumulated in slice
- **Config:** `Options` struct with feature flags
- **Overkill for us:** Expression tree, operator precedence, type system

### `zalgonoise/parse` (Generic Go Parser Library)

- **Uses Go generics** (`[C comparable, T any]`) for token type and value type
- **StateFn pattern** (Rob Pike style): state functions return next state function
- **Produces a tree**, not a flat statement list
- **Overkill for us:** We don't need tree structure or generic type params — our DSL is too simple

### `alecthomas/participle` (Struct-Tag Grammar)

- **Defines grammar via Go struct tags** — parser reads tags and builds a parser automatically
- **Configurable lexer** via `participle.Lexer()` option
- **Token mapping** via `participle.Map()` for post-processing tokens
- **Interesting but wrong fit:** Our grammar is simple enough that struct-tag magic adds indirection without value

---

## Recommendations

### 1. Package structure

```
querykit/              # or whatever the module name is
    parse.go           # Parser, ParserConfig, Parse()
    token.go           # tokenizer, token types
    ast.go             # Query, Statement, Arg, Pos
    error.go           # ParseError
    resolve.go         # FieldResolver interface
    parse_test.go
    fuzz_test.go
```

Separate files by concern, not by "public vs internal." Everything in one package — no `internal/` for a library this small.

### 2. API surface (keep it tiny)

```go
// Parse parses the input string into a Query AST.
// If config is nil, uses permissive defaults (any operation, no field validation).
func Parse(input string, config *ParserConfig) (*Query, error)

// ParserConfig controls parser behavior.
type ParserConfig struct {
    // Operations restricts valid operation names. Nil = accept any.
    Operations map[string]bool

    // FieldResolver validates/expands field names in projections.
    // Nil = accept any identifier as a field name.
    FieldResolver FieldResolver

    // IdentChecker overrides identifier character rules.
    // Nil = default permissive rules (letters, digits, _, -).
    IdentChecker IdentChecker
}
```

One function. One config struct. Three optional extension points. That's it.

### 3. Field handling — parser stores raw, executor resolves

The parser must NOT import field definitions. The AST stores exactly what the user typed. If a `FieldResolver` is configured, the parser calls it during projection parsing and stores the resolved result. If not, raw identifiers pass through. This way:
- The package has zero domain imports
- The task-board CLI creates a `FieldResolver` from its `fields.ValidFields` and `fields.Presets` maps
- Other consumers create their own resolvers or skip validation entirely

### 4. Error handling — structured, JSON-ready, fail-fast

Use `ParseError` with `Pos` (offset + line + column). Implement `error` interface. Single error per parse — no accumulation. Agents can `json.Marshal` the error or read the `.Error()` string.

### 5. Identifiers — keep the permissive default, offer an escape hatch

The current rules (letters, digits, `_`, `-`) cover 99% of use cases. Provide `IdentChecker` interface for the 1% that needs stricter rules. Don't add it to v1 unless someone actually needs it — YAGNI. Document the default rules clearly.

### 6. AST — keep it flat, add Pos, don't expand fields

No expression types. No nested queries. No operator precedence. The `operation(args...) { fields... }` pattern is the entire grammar and should remain so. If complex filters are needed, they go inside argument values as strings — the parser does not interpret them.

### 7. Testing — table-driven + fuzz, test configs as first-class

Every test case can optionally specify a `ParserConfig`. Fuzz tests use `nil` config (most permissive) to find panics. Seed the fuzz corpus with all example queries from the current test suite. Add roundtrip tests once `Statement.String()` exists.

### 8. What NOT to do

- **Don't use code generation** (yacc, PEG, participle). The grammar is ~5 productions. Hand-written recursive descent is clearer, faster, and easier to debug.
- **Don't add generics.** The token type is always `string`. The AST nodes are concrete types. Go generics would add type parameter noise for zero benefit here.
- **Don't add a Visitor/Walker pattern.** With a flat AST (query -> statements -> args/fields), a `for` loop is the visitor. Adding `Accept(Visitor)` methods is overengineering.
- **Don't support comments in v1.** If needed later, `//` or `#` line comments are trivial to add to the tokenizer. Don't design for them now.

---

## Sources

- [PromQL Parser AST](https://github.com/prometheus/prometheus/blob/main/promql/parser/ast.go)
- [PromQL Parser Implementation](https://github.com/prometheus/prometheus/blob/main/promql/parser/parse.go)
- [zalgonoise/parse — Generic Parser Library](https://github.com/zalgonoise/parse)
- [alecthomas/participle — Struct-Tag Parser](https://github.com/alecthomas/participle)
- [A Practical Guide to Building a Parser in Go (2026)](https://gagor.pro/2026/01/a-practical-guide-to-building-a-parser-in-go/)
- [Go Fuzzing Documentation](https://go.dev/doc/security/fuzz/)
- [LogQL Syntax Package](https://pkg.go.dev/github.com/grafana/loki/v3/pkg/logql/syntax)
- [shivamMg/rd — Recursive Descent Parser Builder](https://github.com/shivamMg/rd)
