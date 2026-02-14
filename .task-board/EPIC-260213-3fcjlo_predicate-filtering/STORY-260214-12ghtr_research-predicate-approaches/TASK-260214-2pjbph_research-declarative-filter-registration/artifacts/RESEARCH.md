# Research: Declarative Filter Registration API

## Status: Complete
## Date: 2026-02-14

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Current State Analysis](#current-state-analysis)
3. [Go Generics Constraints](#go-generics-constraints)
4. [Approach 1: FilterableField Registration](#approach-1-filterablefield-registration)
5. [Approach 2: Filter Middleware / Interceptor](#approach-2-filter-middleware--interceptor)
6. [Approach 3: Predicate Builder on OperationContext](#approach-3-predicate-builder-on-operationcontext)
7. [Approach 4: Hybrid Auto-filter + Custom Override](#approach-4-hybrid-auto-filter--custom-override)
8. [Comparative Analysis](#comparative-analysis)
9. [Recommendation](#recommendation)

---

## Problem Statement

Every consumer of `agentquery` that supports filtering must manually:

1. Extract filter values from `[]Arg` by switching on arg keys
2. Build a predicate function `func(T) bool` from those values
3. Call `FilterItems()` or `CountItems()` with the predicate
4. Duplicate this logic across operations (`list`, `count`, etc.)

The `taskFilterFromArgs` pattern in `example/main.go` is ~20 lines. In real-world consumers (like `task-board`), this balloons to 50-100+ lines as more filterable fields are added. The filtering logic is:
- Repeated per domain type
- Error-prone (typos in arg keys, forgotten case-insensitivity)
- Invisible to `schema()` introspection (agents can't discover which fields are filterable)
- Not validated (unknown filter params are silently ignored)

**Goal:** Eliminate boilerplate while keeping the library's philosophy of helpers-not-mandates.

---

## Current State Analysis

### How filtering works today

```
DSL input:  list(status=done, assignee=alice) { overview }
                  ↓
Parser:     Statement.Args = [{Key:"status", Value:"done"}, {Key:"assignee", Value:"alice"}]
                  ↓
Handler:    taskFilterFromArgs(args) → func(Task) bool
                  ↓
Library:    FilterItems(items, pred) → filtered items
                  ↓
Handler:    PaginateSlice(filtered, args) → page
                  ↓
Handler:    Selector.Apply(item) for each → results
```

### Key observations

1. **Args are stringly-typed** — `Arg.Value` is always `string`. Type conversion (to int, bool, etc.) happens in user code.
2. **Comparison is always case-insensitive equality** in practice. The library doesn't enforce this.
3. **`skip`/`take` are already extracted by the library** via `PaginateSlice`/`ParseSkipTake`. Filters could follow the same pattern.
4. **Fields are already registered** with `FieldAccessor[T] = func(T) any`. Filter registration could reuse or extend this.
5. **Schema introspection** (`schema()`) exposes fields, operations, parameters — but not which fields are filterable or what comparison operators they support.

### The FieldAccessor problem

Current field accessors return `any`:
```go
schema.Field("status", func(t Task) any { return t.Status })
```

For filtering, we need typed comparison. A `func(T) any` accessor forces us into runtime type assertions for every comparison. This is the central tension in the design.

---

## Go Generics Constraints

### What Go generics can and cannot do (relevant to this design)

**Can do:**
- Parameterize on the domain type `T` (already done: `Schema[T any]`)
- Parameterize standalone functions on value types: `func Compare[V comparable](a, b V) bool`
- Use `comparable` constraint for equality checks
- Use `constraints.Ordered` (from `golang.org/x/exp/constraints` or `cmp` stdlib) for `<`, `>`, etc.

**Cannot do:**
- Have a method on `Schema[T]` that introduces a second type parameter. Go methods cannot have their own type parameters. `func (s *Schema[T]) FilterableField[V comparable](...)` is **illegal**.
- Store heterogeneously-typed filter definitions in a single map without type erasure.

**Consequence:** Any approach that wants typed filter accessors (`func(T) string` instead of `func(T) any`) must use standalone generic functions (not methods on Schema) for registration, or erase the type at storage time.

### The type erasure trade-off

We can either:
- **Option A:** Store typed accessors, requiring standalone registration functions → more type safety, more API surface
- **Option B:** Store `func(T) any` and do runtime type assertions → simpler API, runtime errors possible
- **Option C:** Store `func(T) string` only (strings are the DSL's native type anyway) → pragmatic, covers 95% of cases

Option C is worth considering seriously: the DSL's args are always strings. Comparing `arg.Value` against `accessor(item)` where both are strings eliminates all type conversion. Non-string fields (int, bool) would need `fmt.Sprintf` or explicit string conversion in the accessor — which is a reasonable trade-off given the DSL's nature.

---

## Approach 1: FilterableField Registration

### Core idea

Add a new registration method alongside `Field()` that marks a field as filterable and provides a typed string accessor for comparison.

### API Surface

```go
// FilterableField registers a field that can be used as a filter parameter in operations.
// The accessor extracts the comparable string value from the domain item.
// Comparison is case-insensitive equality by default.
//
// This is a top-level function (not a method) because Go methods cannot introduce
// additional type parameters.
func FilterableField[T any](s *Schema[T], name string, accessor func(T) string)

// Usage:
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })

// The field's display accessor is registered separately (existing API, unchanged):
schema.Field("status", func(t Task) any { return t.Status })
```

**Alternative: Combined registration**

```go
// FilterField registers a field for both display and filtering.
// displayAccessor is used for field projection (returns any for JSON).
// filterAccessor extracts the string value used for filter comparison.
func FilterField[T any](s *Schema[T], name string, displayAccessor func(T) any, filterAccessor func(T) string)

// Usage:
agentquery.FilterField(schema, "status",
    func(t Task) any { return t.Status },     // display
    func(t Task) string { return t.Status },   // filter
)
```

**Alternative: Single accessor with string conversion**

```go
// FilterableField registers a field as both displayable and filterable.
// The accessor returns a string; for display it's wrapped as any automatically.
func FilterableField[T any](s *Schema[T], name string, accessor func(T) string)

// Internally: schema.fields[name] = func(t T) any { return accessor(t) }
// AND:        schema.filters[name] = accessor
```

This is the cleanest but forces all filterable fields to have string display values. For a field like "priority" (int), you'd need `func(t Task) string { return strconv.Itoa(t.Priority) }` which is awkward.

### Schema changes

```go
type Schema[T any] struct {
    fields             map[string]FieldAccessor[T]
    fieldOrder         []string
    presets            map[string][]string
    defaultFields      []string
    operations         map[string]OperationHandler[T]
    operationMetadata  map[string]OperationMetadata
    loader             func() ([]T, error)
    searchProvider     SearchProvider

    // NEW
    filters            map[string]func(T) string   // filterable field accessors
    filterOrder        []string                     // filterable field names in registration order
}
```

### Predicate building

```go
// BuildPredicate creates a predicate function from registered filters and query args.
// Unknown args (not registered as filters) are silently ignored (or optionally error).
// Comparison is case-insensitive string equality.
func BuildPredicate[T any](s *Schema[T], args []Arg) func(T) bool {
    type filterPair struct {
        accessor func(T) string
        value    string
    }
    var pairs []filterPair

    for _, arg := range args {
        if arg.Key == "" || arg.Key == "skip" || arg.Key == "take" {
            continue // skip positional args and pagination
        }
        if accessor, ok := s.filters[arg.Key]; ok {
            pairs = append(pairs, filterPair{accessor: accessor, value: arg.Value})
        }
        // else: unknown filter arg — silently ignored (or warn/error)
    }

    if len(pairs) == 0 {
        return MatchAll[T]()
    }

    return func(item T) bool {
        for _, p := range pairs {
            if !strings.EqualFold(p.accessor(item), p.value) {
                return false
            }
        }
        return true
    }
}
```

### Handler usage (reduced boilerplate)

```go
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    if err != nil {
        return nil, err
    }

    // OLD: filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))
    // NEW:
    filtered := agentquery.FilterItems(items, ctx.BuildPredicate())

    page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
    if err != nil {
        return nil, err
    }

    results := make([]map[string]any, 0, len(page))
    for _, task := range page {
        results = append(results, ctx.Selector.Apply(task))
    }
    return results, nil
}
```

Wait — `BuildPredicate` needs access to `Schema.filters`, but `OperationContext` doesn't hold a Schema reference. Two options:

**Option A: Add BuildPredicate to OperationContext** (requires Schema to inject it)
```go
type OperationContext[T any] struct {
    Statement      Statement
    Selector       *FieldSelector[T]
    Items          func() ([]T, error)
    BuildPredicate func() func(T) bool  // NEW: injected by Schema
}
```

**Option B: Standalone function**
```go
pred := agentquery.BuildPredicate(schema, ctx.Statement.Args)
```
This requires the handler to have a reference to schema — which it doesn't in the current architecture (handlers are plain functions, not methods on a struct that holds schema).

**Option A is better.** The Schema already constructs `OperationContext` in `executeStatement()`.

### Schema introspection

```go
func (s *Schema[T]) introspect() map[string]any {
    // ... existing code ...

    // NEW: include filterable fields
    if len(s.filters) > 0 {
        filterableFields := make([]string, len(s.filterOrder))
        copy(filterableFields, s.filterOrder)
        result["filterableFields"] = filterableFields
    }

    return result
}
```

Output of `schema()`:
```json
{
  "operations": ["count", "get", "list", "schema", "summary"],
  "fields": ["id", "name", "status", "assignee", "description"],
  "filterableFields": ["status", "assignee"],
  "presets": { ... },
  "defaultFields": ["default"]
}
```

### Type handling

| Type | Accessor | Comparison |
|------|----------|------------|
| string | `func(t T) string { return t.Status }` | `strings.EqualFold(a, b)` |
| int | `func(t T) string { return strconv.Itoa(t.Priority) }` | `strings.EqualFold(a, b)` — comparing string representations |
| bool | `func(t T) string { return strconv.FormatBool(t.Done) }` | `strings.EqualFold(a, b)` — "true"/"false" |
| enum | same as string | same as string |

**Limitation:** String-only comparison means no `priority>3` or range queries. This is by design — the DSL uses `=` only. If operators are needed later, this is a separate extension (see Approach 4 discussion).

### Backwards compatibility

- **Fully backwards compatible.** `FilterableField` is additive. Existing `Field()`, `Operation()`, etc. unchanged.
- Operations that don't call `ctx.BuildPredicate()` work exactly as before.
- `BuildPredicate` on `OperationContext` can be `nil` when no filters are registered (handlers should check, or library provides a no-op default).

### Boilerplate reduction

**Before (example/main.go):**
```go
// 20 lines: taskFilterFromArgs function
// + per-handler: taskFilterFromArgs(ctx.Statement.Args)
```

**After:**
```go
// 2 lines: registration
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })
// per-handler: ctx.BuildPredicate()  (replaces taskFilterFromArgs call)
```

Eliminates the entire `taskFilterFromArgs` function and its equivalents across consumers.

### Edge cases

1. **Unknown filter params:** `list(foo=bar)` where "foo" is not a registered filter. Options:
   - Silent ignore (current behavior — `taskFilterFromArgs` ignores unknown keys)
   - Warn in response
   - Return error
   Recommendation: silent ignore (backwards compat), with optional strict mode later.

2. **Positional args used as filters:** `list(done)` — positional args have `Key=""`. These are skipped by `BuildPredicate` (handled by operation handler if needed, e.g. `get(task-1)`).

3. **Pagination args:** `skip` and `take` are in the same args list. `BuildPredicate` must skip them. Hardcoding `skip`/`take` exclusion is fragile — better to only match registered filter names (positive match, not exclusion).

4. **Case sensitivity:** Always case-insensitive by default. Could add `FilterableFieldWithOptions(schema, name, accessor, FilterOptions{CaseSensitive: true})` later.

5. **Empty values:** `list(status=)` — arg.Value is empty string. If accessor returns non-empty, filter won't match. This is correct behavior.

---

## Approach 2: Filter Middleware / Interceptor

### Core idea

The library applies registered filters **before** the operation handler sees the items. `ctx.Items()` returns already-filtered items.

### API Surface

```go
// FilterableField — same registration as Approach 1
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })

// No additional API — filtering happens automatically in the middleware layer.
// Operations get pre-filtered items via ctx.Items().
```

### Schema changes

Same as Approach 1 for filter storage. The difference is in `executeStatement()`:

```go
func (s *Schema[T]) executeStatement(stmt Statement) (any, error) {
    handler, ok := s.operations[stmt.Operation]
    if !ok {
        return nil, &Error{Code: ErrNotFound, Message: "unknown operation: " + stmt.Operation}
    }

    selector, err := s.newSelector(stmt.Fields)
    if err != nil {
        return nil, err
    }

    // Build predicate from registered filters + args
    pred := s.buildPredicate(stmt.Args)

    // Wrap the loader to apply filtering
    filteredLoader := func() ([]T, error) {
        items, err := s.loader()
        if err != nil {
            return nil, err
        }
        if pred != nil {
            items = FilterItems(items, pred)
        }
        return items, nil
    }

    ctx := OperationContext[T]{
        Statement: stmt,
        Selector:  selector,
        Items:     filteredLoader,  // items come pre-filtered
    }

    return handler(ctx)
}
```

### Handler usage (maximum boilerplate reduction)

```go
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()  // already filtered!
    if err != nil {
        return nil, err
    }

    page, err := agentquery.PaginateSlice(items, ctx.Statement.Args)
    if err != nil {
        return nil, err
    }

    results := make([]map[string]any, 0, len(page))
    for _, task := range page {
        results = append(results, ctx.Selector.Apply(task))
    }
    return results, nil
}

func opCount(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()  // already filtered!
    if err != nil {
        return nil, err
    }
    return map[string]any{"count": len(items)}, nil
}
```

### Opt-out mechanism

Some operations should NOT auto-filter. For example:
- `summary()` needs all items to compute per-status counts
- `get(task-1)` uses a positional arg, not filter args
- Custom operations with non-standard filtering logic

**Option A: Per-operation opt-out flag**
```go
schema.OperationWithMetadata("summary", opSummary, agentquery.OperationMetadata{
    Description: "Return counts grouped by status",
    SkipAutoFilter: true,  // NEW field on OperationMetadata
})
```

**Option B: Raw items accessor on context**
```go
type OperationContext[T any] struct {
    Statement Statement
    Selector  *FieldSelector[T]
    Items     func() ([]T, error)     // filtered items
    RawItems  func() ([]T, error)     // unfiltered items (original loader)
}
```

**Option C: Marker interface or operation wrapper**
```go
// Register as unfiltered
schema.UnfilteredOperation("summary", opSummary)
```

Option B is the most flexible — operations that need both filtered and raw data can access either. But it complicates the API.

Option A is cleaner — a boolean flag on metadata. But it mixes behavior with documentation.

### Composition with PaginateSlice

Works naturally: `ctx.Items()` returns filtered items, then `PaginateSlice` applies after.

```
loader → filter (middleware) → handler gets items → PaginateSlice → field projection
```

This is the correct order: filter before paginate, paginate before project.

### Schema introspection

Same as Approach 1 — `filterableFields` in schema output.

### Backwards compatibility

**This is the risky one.** If a consumer has:

```go
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))
    // ...
}
```

And they add `FilterableField` registrations, now filtering happens twice: once in the middleware, once in the handler. The middleware extracts `status=done`, filters items. Then the handler's `taskFilterFromArgs` also extracts `status=done` and filters again. The result is correct (double-filtering with the same predicate is idempotent), but it's wasteful and confusing.

**Migration path:** Consumer must remove manual filtering when adopting `FilterableField`. This is not a breaking change (old code still works, just does redundant work), but it's an implicit contract change that could confuse users.

### Boilerplate reduction

**Maximum.** The `list` handler goes from:
```go
items, err := ctx.Items()
filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))
page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
```
to:
```go
items, err := ctx.Items()
page, err := agentquery.PaginateSlice(items, ctx.Statement.Args)
```

The `count` handler goes from:
```go
items, err := ctx.Items()
n := agentquery.CountItems(items, taskFilterFromArgs(ctx.Statement.Args))
```
to:
```go
items, err := ctx.Items()
return map[string]any{"count": len(items)}, nil
```

### Edge cases

1. **Double filtering:** As described above — not a bug but a waste. Documentation must be clear.
2. **Opt-out complexity:** Every approach to opt-out adds API surface and cognitive load.
3. **`CountItems` becomes useless:** If items are pre-filtered, `CountItems(items, pred)` is just `len(items)`. The helper is still useful for operations that want custom predicates beyond registered filters.
4. **`get(task-1)` with filters:** If someone writes `get(task-1, status=done)`, the middleware would filter items to only those with status=done, THEN `get` would search for task-1 in the filtered set. This could cause a "not found" error when the task exists but doesn't match the filter. Is this correct behavior? Arguably yes — but it's surprising.

---

## Approach 3: Predicate Builder on OperationContext

### Core idea

The library provides an auto-built predicate on `OperationContext`, but the handler explicitly calls `FilterItems` with it. No magic interception — handler stays in control.

### API Surface

```go
// Registration — same as Approach 1
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })

// OperationContext gets a new method/field
type OperationContext[T any] struct {
    Statement      Statement
    Selector       *FieldSelector[T]
    Items          func() ([]T, error)
    Predicate      func(T) bool           // NEW: auto-built from registered filters + args
}
```

### Schema changes

Same as Approach 1 for filter storage. The `Predicate` is built in `executeStatement()` and injected into `OperationContext`:

```go
func (s *Schema[T]) executeStatement(stmt Statement) (any, error) {
    handler, ok := s.operations[stmt.Operation]
    if !ok { ... }

    selector, err := s.newSelector(stmt.Fields)
    if err != nil { ... }

    ctx := OperationContext[T]{
        Statement: stmt,
        Selector:  selector,
        Items:     s.loader,
        Predicate: s.buildPredicate(stmt.Args),  // never nil — returns MatchAll if no filters
    }

    return handler(ctx)
}
```

### Handler usage

```go
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    if err != nil {
        return nil, err
    }

    // Handler explicitly applies the predicate — stays in control
    filtered := agentquery.FilterItems(items, ctx.Predicate)

    page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
    if err != nil {
        return nil, err
    }

    results := make([]map[string]any, 0, len(page))
    for _, task := range page {
        results = append(results, ctx.Selector.Apply(task))
    }
    return results, nil
}

func opCount(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    if err != nil {
        return nil, err
    }
    n := agentquery.CountItems(items, ctx.Predicate)
    return map[string]any{"count": n}, nil
}

// summary — doesn't use Predicate, just ignores it
func opSummary(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    if err != nil {
        return nil, err
    }
    counts := map[string]int{}
    for _, task := range items {
        counts[task.Status]++
    }
    return map[string]any{"total": len(items), "counts": counts}, nil
}
```

### Composition with PaginateSlice

Same as current approach — handler controls the pipeline:
```
ctx.Items() → FilterItems(items, ctx.Predicate) → PaginateSlice → Selector.Apply
```

### Schema introspection

Same as Approach 1.

### Backwards compatibility

**Fully backwards compatible.** Adding `Predicate` to `OperationContext` is a non-breaking struct field addition (Go structs are not positional — new fields don't affect existing code). Existing handlers that don't use `ctx.Predicate` continue working unchanged.

Handlers that currently build their own predicate can migrate at their own pace:
```go
// Before:
filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))

// After:
filtered := agentquery.FilterItems(items, ctx.Predicate)
```

### Boilerplate reduction

**Moderate.** The `taskFilterFromArgs` function is eliminated, but the handler still calls `FilterItems` explicitly. The per-handler change is a one-liner: replace `taskFilterFromArgs(ctx.Statement.Args)` with `ctx.Predicate`.

### Edge cases

1. **Combining predicates:** Handler wants both auto-filter AND custom logic:
   ```go
   pred := func(t Task) bool {
       return ctx.Predicate(t) && customCondition(t)
   }
   filtered := agentquery.FilterItems(items, pred)
   ```
   This composes cleanly because predicates are just functions.

2. **No registered filters:** `ctx.Predicate` returns `MatchAll` — handler code works without changes, no nil check needed.

3. **Handler ignores Predicate:** Perfectly fine — `summary` just doesn't use it. No opt-out mechanism needed.

---

## Approach 4: Hybrid Auto-filter + Custom Override

### Core idea

Combine Approaches 2 and 3: library auto-filters by default (like middleware), but operations can opt out and use `ctx.Predicate` manually, or add custom predicates on top.

### API Surface

```go
// Registration — same as Approach 1
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })

// Operations are auto-filtered by default.
// To opt out, use a flag:
schema.OperationWithMetadata("summary", opSummary, agentquery.OperationMetadata{
    Description:    "Return counts grouped by status",
    SkipAutoFilter: true,
})

// Or alternatively, register with a dedicated method:
schema.RawOperation("summary", opSummary)  // receives unfiltered items

// Context provides both:
type OperationContext[T any] struct {
    Statement      Statement
    Selector       *FieldSelector[T]
    Items          func() ([]T, error)     // filtered (if auto-filter on) or raw
    Predicate      func(T) bool            // always available for manual use
}
```

### Schema changes

```go
type Schema[T any] struct {
    // ... existing fields ...
    filters        map[string]func(T) string
    filterOrder    []string
    skipAutoFilter map[string]bool  // operations that opt out
}
```

### executeStatement with auto-filter

```go
func (s *Schema[T]) executeStatement(stmt Statement) (any, error) {
    handler, ok := s.operations[stmt.Operation]
    if !ok { ... }

    selector, err := s.newSelector(stmt.Fields)
    if err != nil { ... }

    pred := s.buildPredicate(stmt.Args)

    itemsFn := s.loader
    if !s.skipAutoFilter[stmt.Operation] && pred != nil {
        // Wrap loader with auto-filter
        originalLoader := s.loader
        itemsFn = func() ([]T, error) {
            items, err := originalLoader()
            if err != nil {
                return nil, err
            }
            return FilterItems(items, pred), nil
        }
    }

    ctx := OperationContext[T]{
        Statement: stmt,
        Selector:  selector,
        Items:     itemsFn,
        Predicate: pred,  // always available for manual use
    }

    return handler(ctx)
}
```

### Handler usage

```go
// list — auto-filtered, handler just paginates and projects
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()  // pre-filtered
    if err != nil {
        return nil, err
    }
    page, err := agentquery.PaginateSlice(items, ctx.Statement.Args)
    if err != nil {
        return nil, err
    }
    results := make([]map[string]any, 0, len(page))
    for _, task := range page {
        results = append(results, ctx.Selector.Apply(task))
    }
    return results, nil
}

// summary — opted out, uses raw items
func opSummary(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()  // raw, unfiltered
    if err != nil {
        return nil, err
    }
    counts := map[string]int{}
    for _, task := range items {
        counts[task.Status]++
    }
    return map[string]any{"total": len(items), "counts": counts}, nil
}
```

### Backwards compatibility

**Risky for the same reasons as Approach 2.** Auto-filter changes `ctx.Items()` behavior for existing operations. If a consumer upgrades the library and adds `FilterableField` registrations, their `list` handler now double-filters.

The opt-out mechanism (`SkipAutoFilter`) is an additional concept consumers must learn.

### Boilerplate reduction

Same as Approach 2 for auto-filtered operations. Opted-out operations have no reduction.

### Edge cases

All edge cases from Approach 2 plus:

1. **Which operations should default to auto-filter?** This is a design policy question. `get` usually shouldn't auto-filter (it finds by ID). `summary` shouldn't. `list` and `count` should. But the library can't know the semantics of user-registered operations.

2. **Default-on vs default-off:** If auto-filter is default-on, existing operations break. If default-off, the hybrid adds no value over Approach 3 (handler must explicitly opt in or use `ctx.Predicate` anyway).

---

## Comparative Analysis

### Decision Matrix

| Criterion | Approach 1 | Approach 2 | Approach 3 | Approach 4 |
|-----------|-----------|-----------|-----------|-----------|
| **API simplicity** | Medium (standalone func + context field) | Simple (registration only) | Medium (standalone func + context field) | Complex (registration + opt-out + context) |
| **Type safety** | Good (string accessor typed) | Good (same registration) | Good (same registration) | Good (same registration) |
| **Backwards compat** | Excellent | Poor (changes Items() behavior) | Excellent | Poor (same as 2) |
| **Boilerplate reduction** | Good (eliminates filter func, keeps FilterItems call) | Excellent (eliminates filter func AND FilterItems call) | Good (same as 1) | Excellent for auto-filtered ops |
| **Composability** | Excellent (predicates are functions) | Poor (hidden filtering, hard to compose) | Excellent (predicates are functions) | Medium (must understand two modes) |
| **Opt-out needed?** | No | Yes | No | Yes |
| **Schema introspection** | Easy | Easy | Easy | Easy |
| **Cognitive load** | Low | Medium (must understand middleware) | Low | High (two filtering modes) |
| **Aligns with library philosophy** | Yes (helpers, not mandates) | No (framework behavior) | Yes (helpers, not mandates) | Partially |

### Alignment with existing patterns

The library's philosophy is clear from CLAUDE.md:
> "Pagination and counting are library helpers, not framework mandates"
> "Output format is a transport concern, not a schema concern"

`PaginateSlice` is opt-in: the handler calls it explicitly. It doesn't auto-paginate. Filtering should follow the same pattern.

This strongly favors **Approach 3** (and Approach 1, which is essentially the same with a slightly different API shape).

### Approach 1 vs Approach 3: The real difference

Both use the same registration API. The difference is where `BuildPredicate` lives:

- **Approach 1:** `ctx.BuildPredicate()` — a method/function call that returns `func(T) bool`
- **Approach 3:** `ctx.Predicate` — a pre-built `func(T) bool` field on the context

Approach 3 is better because:
1. The predicate is built once per statement execution, not once per handler call (though in practice each handler calls it once anyway)
2. `ctx.Predicate` is a simple field access, not a function call — reads better
3. No nil-function risk — the schema always injects a valid predicate (MatchAll if no filters)
4. Consistent with `ctx.Selector` (also pre-built and injected)

---

## Recommendation

### Primary: Approach 3 (Predicate Builder on OperationContext)

Approach 3 is the clear winner because:

1. **Follows the library's philosophy.** It's a helper, not a mandate. Handlers opt in by using `ctx.Predicate`.
2. **Fully backwards compatible.** Adding a struct field to `OperationContext` is non-breaking. Existing code compiles and runs unchanged.
3. **Minimal API surface.** One standalone function for registration (`FilterableField`), one new field on `OperationContext` (`Predicate`).
4. **Composable.** Handlers can combine `ctx.Predicate` with custom predicates using standard function composition.
5. **No opt-out mechanism needed.** Operations that don't want filtering simply don't use `ctx.Predicate`.
6. **Consistent with PaginateSlice.** Same pattern: library builds it, handler decides whether to use it.

### Concrete implementation plan

**New types (in types.go):**
```go
// FilterAccessor extracts the filterable string value from a domain item.
// Used for case-insensitive equality matching against query args.
type FilterAccessor[T any] func(item T) string
```

**Schema changes (in schema.go):**
```go
type Schema[T any] struct {
    // ... existing fields ...
    filters     map[string]func(T) string  // filterable field string accessors
    filterOrder []string                    // registration order
}

// Initialize in NewSchema:
// filters: make(map[string]func(T) string),
```

**New standalone function (in a new file, filter.go):**
```go
package agentquery

import "strings"

// FilterableField registers a field as filterable.
// The accessor extracts the string value used for case-insensitive equality
// comparison against query arguments.
//
// This is a package-level function (not a Schema method) because Go methods
// cannot introduce additional type parameters.
//
// The field must also be registered with schema.Field() for display/projection.
// FilterableField only affects filtering behavior.
func FilterableField[T any](s *Schema[T], name string, accessor func(T) string) {
    if s.filters == nil {
        s.filters = make(map[string]func(T) string)
    }
    if _, exists := s.filters[name]; !exists {
        s.filterOrder = append(s.filterOrder, name)
    }
    s.filters[name] = accessor
}

// buildPredicate creates a predicate from registered filters and query args.
// Returns MatchAll if no args match registered filters.
// Skips positional args (Key="") and pagination args (skip, take).
func (s *Schema[T]) buildPredicate(args []Arg) func(T) bool {
    type pair struct {
        accessor func(T) string
        value    string
    }

    var pairs []pair
    for _, arg := range args {
        if arg.Key == "" {
            continue
        }
        if accessor, ok := s.filters[arg.Key]; ok {
            pairs = append(pairs, pair{accessor: accessor, value: arg.Value})
        }
    }

    if len(pairs) == 0 {
        return MatchAll[T]()
    }

    return func(item T) bool {
        for _, p := range pairs {
            if !strings.EqualFold(p.accessor(item), p.value) {
                return false
            }
        }
        return true
    }
}
```

**OperationContext change (in types.go):**
```go
type OperationContext[T any] struct {
    Statement Statement
    Selector  *FieldSelector[T]
    Items     func() ([]T, error)
    Predicate func(T) bool          // auto-built from registered filters + args; MatchAll if none
}
```

**executeStatement change (in query.go):**
```go
ctx := OperationContext[T]{
    Statement: stmt,
    Selector:  selector,
    Items:     s.loader,
    Predicate: s.buildPredicate(stmt.Args),  // NEW
}
```

**Schema introspection change (in schema.go):**
```go
if len(s.filters) > 0 {
    filterableFields := make([]string, len(s.filterOrder))
    copy(filterableFields, s.filterOrder)
    result["filterableFields"] = filterableFields
}
```

### Migration path for consumers

**Step 1: Add filter registrations (non-breaking)**
```go
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })
```

**Step 2: Replace manual filter functions (gradual)**
```go
// Before:
filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))

// After:
filtered := agentquery.FilterItems(items, ctx.Predicate)
```

**Step 3: Delete manual filter function**
```go
// Delete taskFilterFromArgs entirely
```

Steps 1 and 2 can happen in separate commits. Step 1 alone is safe — it just adds introspection data. Step 2 replaces the implementation. Step 3 is cleanup.

### Future extensions (non-breaking)

1. **Comparison operators:** `list(priority>3)` — would require DSL grammar changes and new filter registration API (`FilterableFieldWithOps`). Out of scope but the architecture supports it.
2. **Strict mode:** Return error for unknown filter args. Could be a schema-level option.
3. **Custom comparators:** `FilterableFieldWithCompare(schema, name, accessor, compareFn)` for non-equality matching.
4. **FilterableField that also registers the display field:** Convenience wrapper that calls both `schema.Field()` and `FilterableField()` in one call.
