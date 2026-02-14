# DSL Grammar Extensions for Predicate Filtering

## Context

The agentquery parser (`agentquery/parser.go`) implements a recursive-descent parser with a tokenizer. The current grammar supports only `key=value` equality args inside operation parentheses. This document evaluates four approaches to extend the grammar to support richer predicates (not-equals, comparison, contains/regex).

### Current State

**Grammar:**
```
batch     = query (";" query)*
query     = operation "(" params ")" [ "{" fields "}" ]
params    = param ("," param)*
param     = key "=" value | value
fields    = identifier+
```

**Token types (from parser.go:47-58):**
`tokenIdent`, `tokenString`, `tokenLParen`, `tokenRParen`, `tokenLBrace`, `tokenRBrace`, `tokenEquals`, `tokenComma`, `tokenSemicolon`, `tokenEOF`

**AST Arg type (from ast.go:17-21):**
```go
type Arg struct {
    Key   string `json:"key,omitempty"` // empty for positional args
    Value string `json:"value"`
    Pos   Pos    `json:"pos"`
}
```

**How args are consumed today (from example/main.go:189-208):**
Operation handlers manually iterate `ctx.Statement.Args`, match on `arg.Key`, and use `arg.Value` for equality checks. The `taskFilterFromArgs` function in the example does `strings.EqualFold(t.Status, filterStatus)` -- pure equality, hardcoded per field.

**Key constraint:** The `=` token is a single character emitted at tokenizer level (parser.go:144). Characters like `!`, `>`, `<`, `~` currently hit the `default` branch of the tokenizer switch and trigger "unexpected character" errors since they fail `isIdentStart`.

---

## Approach A: Extended Operator Syntax

### Concept

Add new comparison operator tokens to the tokenizer and grammar. Args become `key operator value` instead of just `key = value`.

```
list(status != done)
list(priority > 3)
list(priority >= 3)
list(name ~= "auth")
```

### Grammar Changes

```
param     = key operator value | key "=" value | value
operator  = "=" | "!=" | ">" | "<" | ">=" | "<=" | "~="
```

### Token Changes

New token types needed:

```go
tokenNotEquals    // !=
tokenGT           // >
tokenLT           // <
tokenGTE          // >=
tokenLTE          // <=
tokenContains     // ~=
```

The tokenizer switch needs to handle multi-character operators. This requires lookahead in the tokenizer:

```go
case '!':
    if t.pos+1 < len(t.input) && t.input[t.pos+1] == '=' {
        t.tokens = append(t.tokens, token{typ: tokenNotEquals, val: "!=", pos: t.pos})
        t.pos += 2
    } else {
        // error: standalone '!' is invalid
    }
case '>':
    if t.pos+1 < len(t.input) && t.input[t.pos+1] == '=' {
        t.tokens = append(t.tokens, token{typ: tokenGTE, val: ">=", pos: t.pos})
        t.pos += 2
    } else {
        t.emit(tokenGT, ">")
    }
case '<':
    if t.pos+1 < len(t.input) && t.input[t.pos+1] == '=' {
        t.tokens = append(t.tokens, token{typ: tokenLTE, val: "<=", pos: t.pos})
        t.pos += 2
    } else {
        t.emit(tokenLT, "<")
    }
case '~':
    if t.pos+1 < len(t.input) && t.input[t.pos+1] == '=' {
        t.tokens = append(t.tokens, token{typ: tokenContains, val: "~=", pos: t.pos})
        t.pos += 2
    } else {
        // error: standalone '~' is invalid
    }
```

### AST Changes

The `Arg` struct needs an operator field:

```go
type Arg struct {
    Key      string `json:"key,omitempty"`
    Operator string `json:"operator,omitempty"` // "=", "!=", ">", "<", ">=", "<=", "~="
    Value    string `json:"value"`
    Pos      Pos    `json:"pos"`
}
```

For positional args, `Operator` remains empty (same as `Key`). For `key=value`, `Operator` is `"="`. This is a semantic shift: currently `key=value` args have `Key` set and `Operator` is implicit `=`. The new `Operator` field makes it explicit.

### Parser Changes

The `parseArg` method (parser.go:417-457) currently does:
1. Advance a token (must be ident or string)
2. If next token is `=`, consume it and read the value
3. Otherwise, treat as positional

With Approach A, step 2 becomes: "if next token is any operator (`=`, `!=`, `>`, `<`, `>=`, `<=`, `~=`), consume it and read the value". This is a modest change -- the `if p.peek().typ == tokenEquals` check becomes a check against a set of operator token types.

```go
func isOperatorToken(t tokenType) bool {
    switch t {
    case tokenEquals, tokenNotEquals, tokenGT, tokenLT, tokenGTE, tokenLTE, tokenContains:
        return true
    }
    return false
}
```

### Backwards Compatibility

**100% backwards compatible.** Existing queries like `list(status=done)` parse identically. The `=` token remains `=`. The only difference is that `Arg.Operator` would now be populated (either as a new field, or by reusing the existing `=` semantics). Existing handler code that reads `arg.Key` and `arg.Value` continues to work unchanged -- it just ignores the new `Operator` field.

However, there's a subtle issue: handlers that receive `status!=done` but only check `arg.Key == "status"` and `arg.Value` would incorrectly treat it as equality. We need handler-side awareness. This is manageable but means the library contract changes -- handlers that previously assumed all args are equality must now check `arg.Operator`.

### Parse Error Quality

Excellent. Since operators are distinct tokens with known positions, errors are precise:
- `list(status!done)` -- standalone `!` triggers "unexpected character `!`" at exact position
- `list(priority>)` -- "expected value after `>`" with correct position
- `list(>=3)` -- "expected argument" (since `>=` is an operator, not an ident)

The tokenizer produces position-tracked tokens, so all errors carry line/column info.

### Implementation Complexity

**Medium.** Changes hit two layers:

1. **Tokenizer:** ~30 lines of new case branches for multi-character operator scanning. Straightforward, no ambiguity. The only tricky bit is that `>` and `>=` share a prefix (1-char lookahead), but that's standard tokenizer fare.

2. **Parser:** ~10 lines changed in `parseArg`. Replace the `tokenEquals` check with a set check.

3. **AST:** Add one field to `Arg`. All JSON serialization remains valid.

4. **Tests:** Each new operator needs tokenizer tests + parser tests + error tests. ~15-20 new test cases.

Total estimate: ~80-100 lines of new/changed code + ~100 lines of tests.

### Agent Ergonomics

**Very natural.** LLMs are trained on SQL, GraphQL, and programming languages where `!=`, `>`, `<` are standard. An agent seeing `schema()` output that says `status != done` is supported will immediately know how to use it. The syntax is unambiguous and doesn't require quoting or escaping.

The DSL reads like a mini-SQL WHERE clause inside parens, which is a pattern LLMs handle extremely well.

**One concern:** agents might try operators that don't exist (like `LIKE`, `IN`, `NOT IN`). The parser would give clear errors for these, but we'd need good schema introspection to tell agents which operators are available per field.

---

## Approach B: Operator-as-Value Convention

### Concept

No grammar changes. Operators are encoded as prefixes inside value strings:

```
list(status="!done")       -- not-equals (! prefix)
list(priority=">3")        -- greater-than (> prefix)
list(priority=">=3")       -- greater-or-equal (>= prefix)
list(name="~auth")         -- contains/regex (~ prefix)
```

### Grammar Changes

**None.** The grammar remains exactly:
```
param = key "=" value | value
```

### Token Changes

**None.** No new tokens needed.

### AST Changes

**None.** `Arg` struct stays identical. The operator is embedded in `Arg.Value`.

### Where the Logic Lives

All interpretation happens in the **registration/handler layer**, not the parser. A helper function would parse value prefixes:

```go
type ParsedPredicate struct {
    Operator string // "=", "!=", ">", "<", ">=", "<=", "~"
    Value    string // the actual value after stripping the prefix
}

func ParsePredicate(rawValue string) ParsedPredicate {
    if strings.HasPrefix(rawValue, "!=") { // wait, this doesn't work...
        // The "!" is part of the value string, "=" was already consumed by the parser
        // So the value is "!done", not "!=done"
    }
    // ...
}
```

Actually, since the parser already splits on `=`, the value for `status="!done"` would be `!done`. So the prefix convention would be:
- `!done` -> not-equals "done"
- `>3` -> greater-than 3
- `>=3` -> greater-or-equal 3
- `<3` -> less-than 3
- `<=3` -> less-or-equal 3
- `~auth` -> contains "auth"

### Backwards Compatibility

**Mostly compatible, with a subtle ambiguity.** If a legitimate value starts with `!`, `>`, `<`, or `~`, it would be misinterpreted as an operator. Example: a task named `!important` filtered as `name="!important"` would be parsed as not-equals "important" instead of equals "!important".

Escaping could solve this (e.g., `\\!important` for literal `!`), but that adds complexity and is error-prone for agents.

For the current codebase, this is unlikely to be a real problem -- status values are things like "done", "todo", "in-progress", and IDs are like "task-1". But it's a design smell that limits future use cases.

### Parse Error Quality

**Poor for predicate errors.** Since the parser doesn't know about operators, it can't produce errors like "invalid operator". If someone writes `list(status="!!done")`, the parser happily accepts it -- the error (if any) surfaces at handler execution time, not parse time. Error messages would be generic runtime errors, not positional parse errors.

The error quality gap is significant for agents. With Approach A, `list(status!!done)` produces a clear parse error at the `!!` position. With Approach B, the query parses fine and the handler must figure out what `!!done` means.

### Implementation Complexity

**Low for the parser (zero changes), medium for the ecosystem.**

1. **Parser/Tokenizer/AST:** Zero changes.
2. **New helper:** ~30-40 lines for `ParsePredicate`.
3. **Handler changes:** Every handler that wants to support operators must call `ParsePredicate` on each value. This is boilerplate that every consumer repeats.
4. **Documentation:** Must clearly document the value prefix convention.

The "simplicity" is deceptive: the complexity shifts from the parser (which handles it once, centrally) to every handler (which handles it N times, ad-hoc).

### Agent Ergonomics

**Awkward.** Requiring quotes around operator-prefixed values is unnatural:

```
list(status="!done")   -- quotes required because ! isn't an ident char
list(priority=">3")    -- quotes required because > isn't an ident char
```

Without quotes, `list(status=!done)` would be a tokenizer error since `!` isn't valid in an identifier. So agents MUST remember to quote these values. This is a friction point.

Worse, there's an inconsistency: `list(status=done)` works without quotes (equality), but `list(status="!done")` requires quotes (not-equals). Agents will trip on this asymmetry.

The convention is also not self-documenting. An agent seeing `status="!done"` might think the value is literally `!done`, not "not equals done". The semantic overloading of the value field is confusing.

---

## Approach C: Filter Function Syntax

### Concept

Introduce a nested `where()` function-like expression for structured filtering:

```
list(where(status != done, priority > 3)) { overview }
list(where(name ~= "auth")) { full }
```

Or alternatively as a separate clause:

```
list() where(status != done, priority > 3) { overview }
```

### Grammar Changes (variant 1: where as arg)

```
param     = "where" "(" predicates ")" | key "=" value | value
predicates = predicate ("," predicate)*
predicate  = key operator value
operator   = "=" | "!=" | ">" | "<" | ">=" | "<=" | "~="
```

### Grammar Changes (variant 2: where as clause)

```
query     = operation "(" params ")" [ "where" "(" predicates ")" ] [ "{" fields "}" ]
```

### Token Changes

Same as Approach A (new operator tokens), plus `where` becomes either a keyword or is parsed contextually.

**Keyword approach:** Add `tokenWhere` token type. The tokenizer would need to recognize `where` as a keyword vs identifier. This is a significant semantic change -- currently ALL identifiers are equal. Making `where` special means it can't be used as an operation name or field name.

**Contextual approach:** `where` is parsed as a regular identifier, and the parser checks for `ident("where") + (` as a special pattern. More complex parsing but avoids reserving a keyword.

### AST Changes

Significant. Either:

1. **Arg gets a nested structure:**
```go
type Arg struct {
    Key        string      `json:"key,omitempty"`
    Value      string      `json:"value,omitempty"`
    Predicates []Predicate `json:"predicates,omitempty"` // for where(...)
    Pos        Pos         `json:"pos"`
}
type Predicate struct {
    Key      string `json:"key"`
    Operator string `json:"operator"`
    Value    string `json:"value"`
    Pos      Pos    `json:"pos"`
}
```

2. **Statement gets a new field (variant 2):**
```go
type Statement struct {
    Operation  string      `json:"operation"`
    Args       []Arg       `json:"args,omitempty"`
    Predicates []Predicate `json:"predicates,omitempty"` // from where(...)
    Fields     []string    `json:"fields,omitempty"`
    Pos        Pos         `json:"pos"`
}
```

### Parser Changes

**Substantial.** The parser needs:

1. All the operator tokenization from Approach A.
2. A new `parseWhere` or `parsePredicates` method.
3. In variant 1: `parseArg` must detect `where(` and branch into predicate parsing.
4. In variant 2: `parseStatement` must check for `where` between `)` and `{`.

The recursive descent stays clean but the grammar is deeper. The parser goes from 2 levels of nesting (query > args) to 3 (query > where > predicates).

### Backwards Compatibility

**Compatible for variant 2** (where as a separate clause). Existing queries without `where` parse identically.

**Risky for variant 1** (where as arg). If any existing operation has a positional arg or key named "where", it breaks. In the current codebase this isn't the case, but it's a latent risk for consumers.

**Breaking if `where` becomes a reserved keyword.** Any consumer using "where" as a field name, operation name, or value would break.

### Parse Error Quality

**Good, but complex.** Errors inside `where(...)` would have position info (from Approach A's operator tokens). But the error messages need more context: "parse error inside where clause at ..." vs "parse error in args at ...". Users need to understand the nesting.

### Implementation Complexity

**High.** This is the most complex option:

1. All of Approach A's tokenizer changes.
2. New parser methods for `where` and predicates (~40-60 lines).
3. New AST types (`Predicate` or expanded `Arg`).
4. All handler logic must understand the new `Predicates` field.
5. Schema introspection needs to expose which operations support `where`.
6. Tests for the combinatorial explosion: where with operators, where with existing args, where in batches, where with projections, nested errors, etc.

Total estimate: ~200-250 lines of new/changed code + ~200+ lines of tests.

### Agent Ergonomics

**Mixed.** On one hand, `where(...)` is an explicit, SQL-like construct that agents understand well. On the other hand, it adds verbosity:

```
# Approach A (concise):
list(status!=done, priority>3) { overview }

# Approach C (verbose):
list(where(status!=done, priority>3)) { overview }
```

The extra nesting adds ~10 characters per query. For LLM agents optimizing for token efficiency, this matters. The `where` keyword also introduces a concept that must be learned -- agents need to know that filters go in `where()`, not directly as args.

The separation between "operation parameters" (skip, take) and "filter predicates" (status!=done) is cleaner conceptually, but adds cognitive load.

---

## Approach D: Keep Grammar As-Is, Handle in Registration Layer

### Concept

The grammar stays exactly as it is. The library provides helper functions and patterns for consumers to build their own filtering logic using `key=value` args.

```
list(status=done) { overview }     -- works today
list(priority=3) { overview }      -- works today (handler interprets)
```

For anything beyond equality, handlers implement custom logic:

```go
// Consumer code
func opSearch(ctx agentquery.OperationContext[Task]) (any, error) {
    query := ""
    for _, arg := range ctx.Statement.Args {
        if arg.Key == "q" {
            query = arg.Value
        }
    }
    // Custom contains/regex logic
    filtered := agentquery.FilterItems(items, func(t Task) bool {
        return strings.Contains(t.Name, query)
    })
    // ...
}
```

The library might add a `PredicateBuilder` or `FilterRegistry` that maps field names to typed comparators, but this stays in the handler layer:

```go
// Possible library helper (not grammar change)
type FilterField[T any] struct {
    Name    string
    Extract func(T) string
}

func BuildFilter[T any](fields []FilterField[T], args []Arg) func(T) bool {
    // matches args against fields using equality
}
```

### Grammar Changes

**None.**

### Token/AST Changes

**None.**

### Parser Changes

**None.**

### Backwards Compatibility

**100% compatible.** Nothing changes.

### Parse Error Quality

**Same as today.** For equality filters, parse errors are already good. For anything beyond equality -- there are no parse errors because there's no syntax for it. Errors come from handlers at runtime.

### Implementation Complexity

**Minimal in the library.** Maybe ~30-50 lines for helper functions.

**High for consumers** who need anything beyond equality. Each consumer reimplements operator parsing from values, or builds custom filter logic. The `taskFilterFromArgs` pattern (example/main.go:189-208) shows this -- it's already 20 lines for two simple equality filters. Adding not-equals, comparison, and contains would roughly triple that per consumer.

### Agent Ergonomics

**Limited.** Agents can only express equality. For anything else, the agent must know consumer-specific conventions:

```
# These might work differently in different tools:
list(q="auth")           -- does this mean equals, contains, or regex?
list(priority=high)      -- is "high" a value or a comparison?
```

Without standardized operators, agents can't generalize across tools built on agentquery. The whole point of the DSL is to give agents a consistent interface -- if filtering operators vary by tool, agents lose that consistency.

**The schema introspection problem:** Even with `OperationMetadata`, there's no way to express "this parameter supports not-equals" in the current `ParameterDef` structure. The `Type` field is `"string"`, `"int"`, `"bool"` -- it doesn't encode supported operators.

---

## Comparison Matrix

| Criterion | A: Extended Operators | B: Value Convention | C: Filter Function | D: As-Is |
|---|---|---|---|---|
| **Grammar changes** | New operator tokens + parser branch | None | New tokens + where clause + predicates | None |
| **AST changes** | Add `Operator` field to `Arg` | None | New `Predicate` type or nested `Arg` | None |
| **Backwards compatible** | Yes (additive) | Mostly (value ambiguity risk) | Yes (additive, unless `where` reserved) | Yes |
| **Parse error quality** | Excellent (positional, typed) | Poor (runtime only) | Good (but more complex messages) | Same as today |
| **Implementation effort** | ~80-100 LOC + tests | ~30-40 LOC helper | ~200-250 LOC + tests | ~0-50 LOC |
| **Consumer handler effort** | Check `arg.Operator` (trivial) | Call `ParsePredicate` (medium) | Read `Predicates` field (medium) | Full custom logic (high) |
| **Agent ergonomics** | Natural (SQL-like) | Awkward (quoting, convention) | Verbose but clear | Limited (equality only) |
| **Extensibility** | Easy to add new operators | Easy (new prefixes) | Easy (new predicates) | Limited |
| **Token efficiency** | High (concise syntax) | Medium (quotes add tokens) | Lower (where keyword overhead) | N/A (can't express) |
| **Consistency across tools** | High (operators are standard) | Low (convention varies) | High (structured) | Low (handler-specific) |

---

## Analysis and Implications

### Why Approach A Is the Strongest Candidate

1. **Natural extension of existing grammar.** The current grammar already has `key=value`. Extending to `key op value` is the minimal generalization. The parser structure barely changes -- `parseArg` gets a wider operator check.

2. **Best agent ergonomics.** `list(status!=done, priority>3)` reads naturally to any agent trained on programming languages. No quoting quirks, no extra keywords, no nesting.

3. **Parse-time validation stays intact.** The library's design decision to validate at parse time (documented in CLAUDE.md) is preserved. Unknown operators trigger parse errors with position info. With Approach B, validation shifts to runtime, breaking this invariant.

4. **Minimal AST disruption.** Adding one `Operator` field to `Arg` is surgical. All existing code that reads `Key` and `Value` continues to work. The `Operator` field can default to `"="` for backwards compatibility.

5. **Schema introspection integration.** `OperationMetadata` can add supported operators per `ParameterDef` naturally:
   ```go
   type ParameterDef struct {
       // ... existing fields ...
       Operators []string `json:"operators,omitempty"` // ["=", "!=", ">", "<"]
   }
   ```

### Why Approach B Is Tempting but Flawed

Zero parser changes is appealing, but:
- The value-prefix convention is not self-documenting. An agent can't discover it from `schema()`.
- Quoting requirements create an asymmetry (`status=done` vs `status="!done"`) that agents will stumble on.
- Error quality degrades from parse-time to runtime.
- Every consumer reimplements the prefix parsing.

Approach B is a "hack" -- it works, but it pushes complexity to the wrong places.

### Why Approach C Is Overengineered

The `where()` clause adds a SQL-like structure that's conceptually clean but practically unnecessary at this scale. The DSL is already simple (one operation, flat args, optional projection). Adding nested predicates inside a where clause introduces a concept that doesn't earn its weight.

The key insight: in the current DSL, there's no ambiguity between "filter parameters" and "control parameters" (like skip/take) -- they're all args. Approach C creates a formal distinction that wasn't needed and adds verbosity to every filtered query.

If the DSL ever grows to support JOINs, subqueries, or multi-entity operations, Approach C would make sense. For filtering within a single operation on a single entity type, it's overkill.

### Why Approach D Is Insufficient

The whole motivation for agentquery is consistency across tools. If every tool implements its own filtering conventions, agents lose the ability to generalize. "I know how to use agentquery DSL" should mean the agent can filter any agentquery-powered tool, not just the ones it's been specifically trained on.

Approach D is where we are today and it's the pain point that motivated this research.

### Risks and Mitigations for Approach A

**Risk: Handlers must become operator-aware.**
Currently, handlers like `taskFilterFromArgs` assume all args are equality. With Approach A, a handler receiving `status!=done` must check `arg.Operator`. If it doesn't, it silently treats `!=` as `=`.

**Mitigation:** The library can provide a `PredicateFromArgs` helper that returns typed predicates for common operators, so handlers don't implement operator dispatch manually:

```go
// Library helper
type Predicate[T any] struct {
    Field    string
    Operator string
    Value    string
}

func PredicatesFromArgs(args []Arg) []Predicate {
    // extract args with operators into Predicate structs
}
```

**Risk: Operator proliferation.**
Once we support `!=`, `>`, `<`, `>=`, `<=`, `~=`, agents might expect `IN`, `NOT IN`, `BETWEEN`, `IS NULL`, etc.

**Mitigation:** Keep the operator set small and fixed. Document it in schema introspection. The DSL is intentionally simpler than SQL -- it's a focused query interface for agents, not a general-purpose query language.

**Risk: Identifier ambiguity with `>` and `<`.**
The characters `>` and `<` are unambiguous in the tokenizer (they can't appear in identifiers). But if a future extension wants `<` for something else (like XML-style syntax), we'd have a conflict.

**Mitigation:** Extremely unlikely. The DSL has no XML-like constructs and no plans for them.

---

## Recommendation

**Approach A (Extended Operator Syntax)** is the clear winner. It offers the best balance of:
- Minimal grammar/parser changes
- Excellent parse error quality
- Natural agent ergonomics
- Full backwards compatibility
- Clean schema introspection integration

The implementation can be phased:
1. **Phase 1:** Add `!=` only (most requested operator, simplest to implement)
2. **Phase 2:** Add `>`, `<`, `>=`, `<=` (comparison operators)
3. **Phase 3:** Add `~=` (contains/regex, needs careful semantics)

Each phase is independently useful and backwards compatible.

### Proposed Operator Semantics

| Operator | Name | Semantics | Example |
|---|---|---|---|
| `=` | equals | Exact match (case-insensitive, as today) | `status=done` |
| `!=` | not-equals | Negated exact match | `status!=done` |
| `>` | greater-than | Numeric or lexicographic comparison | `priority>3` |
| `<` | less-than | Numeric or lexicographic comparison | `priority<5` |
| `>=` | greater-or-equal | Numeric or lexicographic comparison | `priority>=3` |
| `<=` | less-or-equal | Numeric or lexicographic comparison | `priority<=5` |
| `~=` | contains | Substring match or regex | `name~="auth"` |

**Note on semantics:** The parser defines syntax; semantics are handler-defined. `priority>3` is parsed to `{Key:"priority", Operator:">", Value:"3"}`. Whether the handler does numeric comparison, string comparison, or something else is up to the handler. The library can provide typed comparison helpers, but the parser stays semantic-free.
