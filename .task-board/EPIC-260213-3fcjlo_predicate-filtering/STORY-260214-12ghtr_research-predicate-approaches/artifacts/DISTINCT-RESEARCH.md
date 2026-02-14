# Distinct/Unique Helper Research

## Date: 2026-02-14
## Status: Final

---

## Part 1: Use Cases

### 1.1 Core Use Cases for Agents

Agents need distinct values for **discovery** -- understanding what values a field can take before constructing filtered queries. This is the "what are my options?" question.

**Scenario A: Value discovery (the primary driver)**
An agent needs to filter tasks by status but doesn't know the valid statuses. Today it must `list() { status }` and scan all results to deduce unique values. With distinct:

```
distinct(status)  →  ["backlog", "analysis", "development", "done"]
```

The agent now knows exactly which values to use in `list(status=done)`.

**Scenario B: Filtered distinct (scoped discovery)**
An agent wants to know which assignees have done tasks:

```
distinct(assignee, status=done)  →  ["alice", "carol"]
```

This composes naturally with the existing filtering infrastructure. The pipeline: Items -> Filter -> Distinct. If `FilterableField` registrations exist, `ctx.Predicate` handles the filter; distinct operates on the filtered set.

**Scenario C: Distribution/histogram (group-by count)**
An agent wants to understand workload distribution:

```
distinct(status) { count }  →  {"backlog": 3, "in-progress": 3, "done": 2}
distinct(assignee) { count }  →  {"alice": 2, "bob": 2, "carol": 2, "dave": 1, "": 1}
```

This is closely related to the existing `summary` operation in the example, which manually does `counts[task.Status]++`. The pattern is common enough to generalize.

**Scenario D: Multi-field distinct (unique combinations)**
Less common, but occasionally useful -- unique pairs:

```
distinct(status, assignee)  →  [{"status":"done","assignee":"alice"}, {"status":"done","assignee":"carol"}, ...]
```

This is the SQL `SELECT DISTINCT status, assignee` pattern. More complex, lower priority.

### 1.2 Frequency Estimate

Based on the existing `board-cli` consumer (the primary consumer of agentquery):

| Use case | Frequency | Currently solved by |
|----------|-----------|-------------------|
| Value discovery | High (agents do this before every filtered query series) | `list()` + manual scanning, or `summary()` for status only |
| Filtered distinct | Medium (scoped exploration) | Manual: `list(status=done) { assignee }` + scan |
| Distribution/count | Medium (understanding data shape) | `summary()` hard-coded for status only |
| Multi-field distinct | Low (rarely needed) | Not solved |

The first three are real pain points. Multi-field distinct is nice-to-have.

### 1.3 Token Efficiency Argument

An agent today does:
```
list() { status }
→ [{"status":"todo"},{"status":"done"},{"status":"todo"},{"status":"in-progress"},{"status":"done"},{"status":"done"},{"status":"todo"},{"status":"in-progress"}]
```

That's 8 objects for 3 unique values. With `distinct`:
```
distinct(status)
→ ["todo","done","in-progress"]
```

For a board with 200 tasks and 5 statuses, that's 200 objects vs. 5 strings. The token savings are substantial and directly relevant to the "agent-facing API" philosophy -- minimizing tokens is a core design goal.

---

## Part 2: Industry Patterns

### 2.1 SQL

```sql
-- Simple distinct
SELECT DISTINCT status FROM tasks;

-- Distinct with filter
SELECT DISTINCT assignee FROM tasks WHERE status = 'done';

-- Group-by with count
SELECT status, COUNT(*) FROM tasks GROUP BY status;

-- Multi-field distinct
SELECT DISTINCT status, assignee FROM tasks;
```

SQL treats `DISTINCT` as a modifier on `SELECT`, and `GROUP BY` as a separate clause. They're conceptually related but syntactically separate.

### 2.2 MongoDB

```javascript
// Simple distinct -- returns array of unique values
db.tasks.distinct("status")
// → ["todo", "done", "in-progress"]

// Distinct with filter
db.tasks.distinct("assignee", { status: "done" })
// → ["alice", "carol"]

// Group-by with count (aggregation pipeline)
db.tasks.aggregate([
  { $group: { _id: "$status", count: { $sum: 1 } } }
])
```

MongoDB's `distinct()` is a first-class method that returns an array. Group-by requires the aggregation pipeline -- a much heavier abstraction. The simple `distinct()` API is the model closest to what agentquery needs.

### 2.3 Elasticsearch

```json
// Terms aggregation (distinct + count combined)
{
  "aggs": {
    "statuses": {
      "terms": { "field": "status" }
    }
  }
}
// → { "buckets": [{"key": "done", "doc_count": 5}, {"key": "todo", "doc_count": 3}] }
```

ES always gives counts with distinct values -- there's no "distinct without count" because the term aggregation inherently counts. This is worth noting: in practice, counts are almost always useful alongside distinct values.

### 2.4 Django ORM

```python
# Simple distinct
Task.objects.values_list('status', flat=True).distinct()
# → ['todo', 'done', 'in-progress']

# With filter
Task.objects.filter(status='done').values_list('assignee', flat=True).distinct()

# Group-by with count
Task.objects.values('status').annotate(count=Count('id'))
# → [{'status': 'todo', 'count': 3}, {'status': 'done', 'count': 5}]
```

Django chains `distinct()` as a queryset modifier. The `values().annotate(Count())` pattern is the group-by equivalent.

### 2.5 Prisma

```typescript
// Distinct on findMany
const statuses = await prisma.task.findMany({
  distinct: ['status'],
  select: { status: true }
})

// No native group-by (use groupBy)
const counts = await prisma.task.groupBy({
  by: ['status'],
  _count: true
})
```

Prisma separates `distinct` (on findMany) from `groupBy` (separate method). `distinct` is a modifier, `groupBy` is its own operation.

### 2.6 GraphQL (Hasura)

```graphql
# Distinct
query {
  tasks(distinct_on: status) {
    status
  }
}

# Aggregate (group-by count)
query {
  tasks_aggregate {
    aggregate {
      count
    }
    nodes {
      status
    }
  }
}
```

Hasura's `distinct_on` is a query modifier, not a standalone operation.

### 2.7 Pattern Synthesis

| System | Distinct is... | Group-by is... | Always includes count? |
|--------|---------------|----------------|----------------------|
| SQL | SELECT modifier | Separate clause | No (explicit) |
| MongoDB | First-class method | Aggregation pipeline | No |
| Elasticsearch | Terms aggregation | Same as distinct | Yes (always) |
| Django | Queryset modifier | `.annotate(Count())` | No (explicit) |
| Prisma | findMany modifier | Separate method | No (explicit) |
| GraphQL/Hasura | Query modifier | Aggregate query | No (explicit) |

**Key insight:** Most systems separate "get unique values" from "count per value". Only Elasticsearch merges them. For agentquery, the simplest approach is a helper that returns unique values, and a separate helper (or the same helper with a different return type) for counted groups.

---

## Part 3: API Design Options

### Option A: Pure Helper Functions (like FilterItems)

```go
// Returns unique values extracted by accessor, in first-seen order.
func Distinct[T any](items []T, accessor func(T) string) []string

// Returns a count per unique value.
func DistinctCount[T any](items []T, accessor func(T) string) map[string]int
```

**Analysis:**

| Criterion | Assessment |
|-----------|-----------|
| "Helpers not mandates" philosophy | Perfect fit. Same pattern as FilterItems, CountItems. |
| Type safety | Accessor is `func(T) string` -- type-safe at compile time. |
| Composability with filtering | Excellent: `Distinct(FilterItems(items, pred), accessor)`. |
| Needs its own registration? | No. Consumer provides accessor inline. |
| Schema introspection | None -- invisible to `schema()`. |
| DSL syntax | None -- not an operation. Used inside custom operation handlers. |

**Pros:**
- Zero framework coupling. Works standalone.
- Consumer picks which accessor to use. No field name resolution needed.
- Easiest to implement (~15 LOC per function).
- Composable with the entire pipeline manually.

**Cons:**
- Agent can't call `distinct(status)` from the DSL. The consumer must wire up a custom operation.
- No introspection -- agent doesn't know which fields support distinct.
- Every consumer reimplements the "distinct" operation handler (similar to the boilerplate problem that FilterableField solves for filtering).

### Option B: Built-in Schema Operation

```go
// Auto-registered operation, uses FieldAccessor accessors from schema
// DSL: distinct(status), distinct(assignee, status=done)
```

Implementation: Register a `"distinct"` operation in `NewSchema` that:
1. Takes the first positional arg as the field name
2. Looks up the field's `FieldAccessor[T]`
3. Calls it on each item, collects unique `any` values
4. Returns `[]any`

**Analysis:**

| Criterion | Assessment |
|-----------|-----------|
| "Helpers not mandates" philosophy | Borderline. It's a built-in operation, like `schema`. |
| Type safety | Weaker. Uses `FieldAccessor[T]` which returns `any`. Comparison becomes `fmt.Sprintf("%v", val)` or reflection. |
| Composability with filtering | Works if FilterableField is implemented (uses ctx.Predicate). Otherwise, must re-parse filter args. |
| Needs its own registration? | No. Reuses existing Field() registrations. |
| Schema introspection | Automatic -- it's an operation, appears in `schema()`. |
| DSL syntax | `distinct(status)`, `distinct(assignee, status=done)`. |

**Pros:**
- Agent calls it directly from DSL -- zero consumer wiring for basic cases.
- Reuses existing `FieldAccessor` registrations.
- Appears in `schema()` automatically.

**Cons:**
- `FieldAccessor` returns `any`. To collect distinct values, the library must convert `any` to a comparable type. `fmt.Sprintf("%v", val)` works for most cases (strings, ints, bools) but is fragile for complex types (slices, maps, structs).
- Pushes toward a "framework" mentality rather than "library". The `schema` built-in is justified because it's introspection -- the library knows its own structure. A `distinct` built-in is the library doing data analysis, which is the domain layer's job.
- What about fields that shouldn't support distinct? (e.g., `description` -- distinct on free text is nonsensical). The library can't know this.
- If the consumer wants custom grouping (e.g., distinct on the first letter of name, or distinct on a derived field), the built-in doesn't help.

### Option C: Hybrid -- Helper Functions + Optional DistinctableField Registration

```go
// Pure helpers (always available, used in any operation handler)
func Distinct[T any](items []T, accessor func(T) string) []string
func DistinctCount[T any](items []T, accessor func(T) string) map[string]int

// Optional registration for agent-discoverable distinct
// (standalone function due to Go generics constraint, same pattern as FilterableField)
func DistinctableField[T any](schema *Schema[T], name string, accessor func(T) string)
```

When `DistinctableField` is registered:
- `schema()` output includes `"distinctableFields": ["status", "assignee", "type"]`
- The library auto-registers a `"distinct"` operation that uses the registered accessors
- Agent can call `distinct(status)` directly

When no `DistinctableField` is registered:
- No `"distinct"` operation
- Helpers are still available for custom operations

**Analysis:**

| Criterion | Assessment |
|-----------|-----------|
| "Helpers not mandates" philosophy | Best fit. Helpers are always there. Registration is opt-in. |
| Type safety | `func(T) string` -- fully type-safe. No `any` comparison. |
| Composability with filtering | `Distinct(FilterItems(items, pred), accessor)` for manual. Auto-filtered via ctx.Predicate for registered. |
| Needs its own registration? | Optional. Helpers work without it. |
| Schema introspection | Only when registered. |
| DSL syntax | `distinct(fieldname)` when registered. |

**Pros:**
- Covers all use cases: manual helper for custom operations, auto-registered for common patterns.
- `func(T) string` accessor eliminates the `any` comparison problem cleanly.
- Agent discoverability when opted into.
- Follows the FilterableField precedent exactly.

**Cons:**
- Three registration calls per field (Field, FilterableField, DistinctableField) if a field supports all three. Registration verbosity grows.
- The `string` accessor means the consumer must stringify numeric fields. Same trade-off as FilterableField.

### Option D: Merge with FilterableField -- Fields that are filterable are automatically distinctable

```go
// FilterableField already registers a func(T) string accessor.
// The same accessor works for distinct. Filtering and distinct share the same registration.
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })

// This single registration enables:
// - Filtering: list(status=done)
// - Distinct: distinct(status)
// - Distribution: distinct(status) { count }
```

**Analysis:**

| Criterion | Assessment |
|-----------|-----------|
| "Helpers not mandates" philosophy | Good. Opt-in via FilterableField. |
| Type safety | Same as FilterableField: `func(T) string`. |
| Composability | Perfect -- filtered distinct uses the same registered filters. |
| Needs its own registration? | No! Reuses FilterableField. |
| Schema introspection | `filterableFields` already planned. Distinct capability is implicit. |
| DSL syntax | `distinct(status)`, `distinct(assignee, status=done)`. |

**Pros:**
- Zero additional registration burden. If it's filterable, it's distinctable.
- The `func(T) string` accessor registered for filtering is exactly what's needed for distinct.
- Introspection for free -- `filterableFields` already tells the agent which fields can be filtered, and by extension, which fields can be "distincted".
- Perfectly composable: `distinct(assignee, status=done)` filters by status using the registered filter, then extracts unique assignees.

**Cons:**
- Conflates two concepts. A field might be filterable but not meaningfully distinctable (e.g., a `description` field filtered by keyword match but where distinct values are all unique -- though the consumer wouldn't register `description` as FilterableField anyway since it's equality-based).
- Less explicit. The agent doesn't know `distinct` exists until it sees the operation (but the operation appears in `schema()` as soon as any FilterableField is registered).
- What about fields that should be distinctable but NOT filterable? Unusual but possible. (In practice, any field you'd want distinct values for, you'd also want to filter by.)

---

## Part 4: GroupBy

### 4.1 Is GroupBy separate from Distinct?

Conceptually:
- `Distinct` answers: "What are the unique values?"
- `GroupBy` answers: "How are items distributed across unique values?"
- `DistinctCount` is GroupBy + Count -- the most common aggregation.

In SQL terms:
- `SELECT DISTINCT status` -- Distinct
- `SELECT status, COUNT(*) GROUP BY status` -- GroupBy + Count
- `SELECT status, array_agg(name) GROUP BY status` -- GroupBy + projection

### 4.2 Helper Design

```go
// GroupBy groups items by a key function. Returns a map of key -> items.
func GroupBy[T any](items []T, keyFn func(T) string) map[string][]T
```

This is the most general form. `DistinctCount` is `GroupBy` + `len()` on each group:

```go
func DistinctCount[T any](items []T, keyFn func(T) string) map[string]int {
    groups := GroupBy(items, keyFn)
    counts := make(map[string]int, len(groups))
    for k, v := range groups {
        counts[k] = len(v)
    }
    return counts
}
```

But there's an efficiency argument: if you only need counts, `DistinctCount` avoids allocating the intermediate slices. Direct implementation:

```go
func DistinctCount[T any](items []T, keyFn func(T) string) map[string]int {
    counts := make(map[string]int)
    for _, item := range items {
        counts[keyFn(item)]++
    }
    return counts
}
```

This is the same optimization rationale as `CountItems` vs. `len(FilterItems(...))`.

### 4.3 GroupBy + Field Projection

The interesting question: `distinct(status) { count }` implies that the field projection block changes semantics when used with distinct. Normally `{ overview }` means "project these fields from each item". With distinct, `{ count }` means "aggregate each group".

This is a **grammar semantics overload** that I'd avoid. Instead:

**Option 1: Separate operation names**
```
distinct(status)                →  ["todo", "done", "in-progress"]
distribution(status)            →  {"todo": 3, "done": 2, "in-progress": 3}
```

**Option 2: Parameter-based mode**
```
distinct(status)                →  ["todo", "done", "in-progress"]
distinct(status, count=true)    →  {"todo": 3, "done": 2, "in-progress": 3}
```

**Option 3: Field block changes output**
```
distinct(status)                →  ["todo", "done", "in-progress"]
distinct(status) { count }      →  {"todo": 3, "done": 2, "in-progress": 3}
```

Option 3 is actually clean if `count` is treated as a special aggregate keyword, not a field projection. But it overloads the `{ }` block semantics, which currently always means "field projection". This would be a conceptual break.

**Recommendation:** Option 1 or Option 2. Both are clean. Option 2 is more compact and doesn't require a new operation name.

### 4.4 GroupBy Composability with Field Projection

Full GroupBy with projection (SQL's `SELECT status, array_agg(name) GROUP BY status`) is a powerful feature but heavyweight. In agentquery's scope, the primary use cases are:

1. Unique values (Distinct) -- `[]string`
2. Value counts (DistinctCount) -- `map[string]int`
3. Grouped items (GroupBy) -- `map[string][]T`

Case 3 with projection would produce `map[string][]map[string]any` -- grouped items with field selection applied. This is doable but niche. Recommend keeping it as a helper only, not as a DSL operation.

### 4.5 GroupBy Summary

| Function | Return type | DSL operation? | Complexity |
|----------|------------|----------------|-----------|
| `Distinct[T]` | `[]string` | Yes | Trivial |
| `DistinctCount[T]` | `map[string]int` | Yes (via parameter) | Trivial |
| `GroupBy[T]` | `map[string][]T` | No (helper only) | Trivial |

`GroupBy` stays as a pure helper for custom operation handlers. `Distinct` and `DistinctCount` get DSL exposure.

---

## Part 5: Recommendation

### 5.1 Approach: Option D (merge with FilterableField) + Pure Helpers

**Rationale:**

1. **The accessor already exists.** FilterableField registers `func(T) string` -- that's exactly what Distinct needs. No additional registration.

2. **Zero marginal cost for consumers.** If you've registered FilterableField (which you're doing anyway for filtering), distinct comes for free.

3. **Natural composition.** `distinct(assignee, status=done)` -- the `status=done` filter uses the registered FilterableField. The `assignee` distinct uses the same kind of accessor. It's all the same data.

4. **Schema introspection is already there.** `filterableFields` in `schema()` output tells the agent which fields can be filtered AND distincted.

5. **Helpers exist independently.** `Distinct[T]` and `DistinctCount[T]` are pure helper functions. Custom operation handlers use them directly without any registration, just like `FilterItems` and `CountItems`.

### 5.2 Concrete Function Signatures

#### Pure helpers (in `helpers.go` or new `distinct.go`)

```go
// Distinct returns unique values from items, extracted by keyFn, in first-seen order.
// Preserving insertion order is important for deterministic output.
func Distinct[T any](items []T, keyFn func(T) string) []string {
    seen := make(map[string]bool)
    var result []string
    for _, item := range items {
        key := keyFn(item)
        if !seen[key] {
            seen[key] = true
            result = append(result, key)
        }
    }
    return result
}

// DistinctCount returns a count per unique value, extracted by keyFn.
// More efficient than GroupBy + len when only counts are needed.
func DistinctCount[T any](items []T, keyFn func(T) string) map[string]int {
    counts := make(map[string]int)
    for _, item := range items {
        counts[keyFn(item)]++
    }
    return counts
}

// GroupBy groups items by a key function.
// Returns a map of key -> items, where each group preserves the original item order.
func GroupBy[T any](items []T, keyFn func(T) string) map[string][]T {
    groups := make(map[string][]T)
    for _, item := range items {
        key := keyFn(item)
        groups[key] = append(groups[key], item)
    }
    return groups
}
```

#### Built-in `distinct` operation (auto-registered when FilterableFields exist)

Registered in `NewSchema` (or lazily on first FilterableField registration). The handler:

```go
func (s *Schema[T]) distinctHandler(ctx OperationContext[T]) (any, error) {
    // First positional arg = field name
    if len(ctx.Statement.Args) == 0 || ctx.Statement.Args[0].Key != "" {
        return nil, &Error{
            Code:    ErrValidation,
            Message: "distinct requires a field name as first argument",
        }
    }

    fieldName := ctx.Statement.Args[0].Value
    accessor, ok := s.filterAccessors[fieldName]
    if !ok {
        return nil, &Error{
            Code:    ErrValidation,
            Message: fmt.Sprintf("field %q is not registered as filterable/distinctable", fieldName),
            Details: map[string]any{"field": fieldName, "available": s.filterOrder},
        }
    }

    items, err := ctx.Items()
    if err != nil {
        return nil, err
    }

    // Apply filters (remaining keyword args, excluding the distinct field itself)
    filtered := FilterItems(items, ctx.Predicate)

    // Check for count mode
    wantCount := false
    for _, arg := range ctx.Statement.Args {
        if arg.Key == "count" && (arg.Value == "true" || arg.Value == "1") {
            wantCount = true
        }
    }

    if wantCount {
        return DistinctCount(filtered, accessor), nil
    }

    return Distinct(filtered, accessor), nil
}
```

#### OperationMetadata for introspection

```go
OperationMetadata{
    Description: "Get unique values of a filterable field, optionally with counts",
    Parameters: []ParameterDef{
        {Name: "field", Type: "string", Optional: false, Description: "Field name to get distinct values for (positional)"},
        {Name: "count", Type: "bool", Optional: true, Default: false, Description: "Include count per value"},
        // Plus all FilterableField params as optional filters
    },
    Examples: []string{
        "distinct(status)",
        "distinct(assignee, status=done)",
        "distinct(status, count=true)",
    },
}
```

### 5.3 Pipeline Position

```
Items -> Filter -> [Sort] -> Paginate -> Project -> Serialize
                     ^
                     |
Items -> Filter -> Distinct/DistinctCount  (branch off before sort/paginate/project)
```

Distinct operates on the **filtered** set but does NOT go through sort, paginate, or project. The output of distinct is not a list of domain items -- it's a list of unique strings or a count map. Pagination and projection don't apply.

This means `distinct` is a **terminal operation** -- it produces a final result, not intermediate items. Similar to `count()` in the current design.

### 5.4 When to Register the Built-in

Two options:

**Option A: Register on first FilterableField call**
```go
func FilterableField[T any](schema *Schema[T], name string, accessor func(T) string) {
    // ... existing filter registration ...

    // Auto-register distinct operation on first filterable field
    if len(schema.filterAccessors) == 1 {
        schema.operations["distinct"] = schema.distinctHandler
        schema.operationMetadata["distinct"] = distinctMeta(schema)
    }
}
```

**Option B: Always register, fail gracefully**
Always register `distinct` in `NewSchema`. If no FilterableFields exist, it returns an error explaining that no filterable fields are registered.

**Recommendation: Option A.** Keeps `schema()` output clean when distinct isn't applicable. The operation only appears when the consumer has opted into FilterableField. This matches the "schema introspection shows what's actually available" principle.

**Complication with Option A:** Since `FilterableField` is a standalone function (not a Schema method), it can modify the schema's internals but the lazy registration of operations is slightly awkward. However, `FilterableField` already must modify schema internals to register filters, so adding an operation registration is the same pattern.

### 5.5 Implementation Complexity

| Component | LOC estimate | Notes |
|-----------|-------------|-------|
| `Distinct[T]` helper | ~12 | Straightforward |
| `DistinctCount[T]` helper | ~8 | Straightforward |
| `GroupBy[T]` helper | ~10 | Straightforward |
| `distinct` operation handler | ~40 | Field lookup, filter compose, count mode |
| Auto-registration in FilterableField | ~10 | One-time op+meta registration |
| Schema introspection update | ~5 | Already covered by filterableFields |
| Tests for helpers | ~80 | Empty, single, duplicates, order preservation |
| Tests for operation | ~60 | DSL integration, filtered, count mode, errors |
| **Total** | **~225 LOC** | |

### 5.6 Edge Cases

| Edge case | Behavior |
|-----------|----------|
| Empty items | `Distinct` returns `[]string{}`, `DistinctCount` returns `map[string]int{}` |
| All same value | `Distinct` returns `["value"]`, count returns `{"value": N}` |
| Empty string values | Treated as a valid distinct value (e.g., unassigned tasks with `Assignee=""`) |
| Field not registered | Error: "field X is not registered as filterable/distinctable" |
| No positional arg | Error: "distinct requires a field name as first argument" |
| Distinct + pagination args | Pagination args are ignored (distinct is a terminal operation, not a list) |
| Distinct + field projection `{ overview }` | Ignored (output is values/counts, not projected items) |

### 5.7 DSL Examples (Agent Perspective)

After `schema()` reveals `filterableFields: ["status", "assignee", "type"]`:

```bash
# What statuses exist?
q 'distinct(status)'
→ ["backlog","in-progress","done"]

# Who's working on done tasks?
q 'distinct(assignee, status=done)'
→ ["alice","carol"]

# How are tasks distributed by status?
q 'distinct(status, count=true)'
→ {"backlog":3,"done":2,"in-progress":3}

# Batch: get all filter options at once
q 'distinct(status); distinct(assignee); distinct(type)'
→ [["backlog","in-progress","done"], ["alice","bob","carol","dave",""], ["epic","story","task"]]

# Combine with other operations in batch
q 'distinct(status); count(); list(status=done) { minimal }'
→ [["backlog","in-progress","done"], {"count":8}, [{"id":"task-3","status":"done"}, ...]]
```

### 5.8 Compact Output Format

For `--format compact`:

```
# distinct(status)
status
backlog
in-progress
done

# distinct(status, count=true)
status,count
backlog,3
in-progress,3
done,2
```

The compact formatter already handles `[]string` (falls back to JSON) and `map[string]int` (falls back to JSON). We may want to add specific formatting for these types in `FormatCompact` to produce the tabular output shown above, but JSON fallback is acceptable for v1.

### 5.9 Dependency on FilterableField

This design **depends on FilterableField being implemented first** (Phase 1 of the predicate filtering work). The distinct operation reuses:
- The `filterAccessors` map (registered by `FilterableField`)
- The `ctx.Predicate` mechanism (for filtered distinct)
- The `filterableFields` introspection (for discoverability)

If implemented before FilterableField, the helpers (`Distinct`, `DistinctCount`, `GroupBy`) can land independently -- they're pure functions with no framework dependency. The built-in `distinct` operation requires FilterableField.

### 5.10 Migration Path

1. **Phase 1a (independent):** Land `Distinct`, `DistinctCount`, `GroupBy` helpers. Zero dependencies. Consumers use them in custom operations immediately.
2. **Phase 1b (after FilterableField):** Auto-register `distinct` operation. Consumers get DSL-level distinct for free.
3. **Phase 2 (if needed):** Add `distribution` or `histogram` as a separate operation name for grouped counts, if the `count=true` parameter feels awkward.

---

## Appendix A: Rejected Alternatives

| Alternative | Why rejected |
|-------------|-------------|
| `DistinctableField` as separate registration | Adds registration burden for zero benefit. FilterableField's accessor is identical. |
| Built-in `distinct` using `FieldAccessor[T]` (returns `any`) | Comparison on `any` requires `fmt.Sprintf` or reflection. Fragile. FilterableField's `func(T) string` is strictly better. |
| `{ count }` field block for count mode | Overloads field projection semantics. `count=true` parameter is cleaner and doesn't change grammar meaning. |
| `groupby` as a DSL operation | Too heavy for the DSL scope. GroupBy producing `map[string][]T` with projection would need a new output structure. Helper-only is sufficient. |
| Distinct returning `[]any` | Loses type information. `[]string` is safer and matches the string accessor. |
| Multi-field distinct in v1 | Low priority use case, high complexity (unique tuples → need composite key or struct). Defer. |

## Appendix B: Related Research

| Document | Relevance |
|----------|-----------|
| `REPORT.md` (consolidated predicate research) | FilterableField design that distinct builds on |
| `SORTING-RESEARCH.md` | Pipeline position reference (Filter → Sort → Paginate) |
