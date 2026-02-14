# Research: Sorting/Ordering Patterns for agentquery

## Context

agentquery is a Go generic library (`Schema[T]`) providing a mini-query DSL for agent-optimized CLI tools. The DSL grammar:

```
operation(key=value, key=value) { field_projection }
```

The library already has:

- **Field projection**: `{ id name status }` with presets like `{ overview }`
- **Filtering**: Currently manual via `FilterItems[T](items, predicate)`. About to get declarative registration via `FilterableField[T]` with auto-built `ctx.Predicate`
- **Pagination**: `PaginateSlice[T](items, args)` extracts `skip`/`take` from args

**Missing: Sorting/Ordering.** The target pipeline: Items -> Filter -> **Sort** -> Paginate -> Project

### Current Architecture (Relevant)

**`FieldAccessor[T]`**: `func(item T) any` -- extracts a field value from a domain item for display/serialization.

**`Arg`**: `{ Key string, Value string }` -- parsed from `key=value` in query args. Parser preserves arg order in `[]Arg` and allows duplicate keys.

**`PaginateSlice[T]`**: The model for how sorting should work -- a generic helper function that operation handlers opt into. It: (1) parses `skip`/`take` from `[]Arg`, (2) returns sliced items, (3) validates with descriptive errors, (4) is NOT framework-mandated -- handlers choose to call it.

**Identifier rules**: Letters, digits, underscores, hyphens. Values can also be quoted strings. No dots, no `+`/`-` as standalone tokens. The `-` character is NOT valid as the first character of an identifier (`isIdentStart` does not include it).

---

## Part 1: Industry Patterns for Sorting in Query DSLs

### 1. GraphQL (Hasura, Prisma)

#### Hasura

**Syntax**: Object-based `order_by` argument with field-to-direction enum mapping.

```graphql
query {
  tasks(order_by: [{ name: desc }, { created_at: asc_nulls_first }]) {
    id
    name
  }
}
```

**Direction enums**: `asc`, `desc`, `asc_nulls_first`, `asc_nulls_last`, `desc_nulls_first`, `desc_nulls_last`

**Multi-field**: The `order_by` argument takes an **array of objects**, each mapping a single field to a direction. Order in the array determines sort priority.

**Sortable field registration**: Fields are declared in the GraphQL schema as properties of the entity type. Hasura auto-generates `order_by` Input Types from the database schema. Every non-array column is sortable by default. Sortability can be restricted via permissions.

**Default direction**: `asc` (ascending).

**Null handling**: Explicit via `asc_nulls_first`, `asc_nulls_last`, `desc_nulls_first`, `desc_nulls_last` -- four-variant enum. Default: `asc` puts nulls at end, `desc` puts nulls at start (following PostgreSQL convention).

**Type awareness**: Sorting uses the database's native comparison for the column type. String sorting follows database collation. Numeric and date fields use their natural order.

**Composability**: `order_by`, `where`, `limit`, `offset` are independent top-level arguments. Fully orthogonal.

#### Prisma

**Syntax**: Object or array of objects for `orderBy`.

```typescript
// Single field
const tasks = await prisma.task.findMany({
  orderBy: { name: 'desc' }
})

// Multi-field (array of single-key objects -- order matters)
const tasks = await prisma.task.findMany({
  orderBy: [
    { priority: 'desc' },
    { name: 'asc' }
  ]
})

// With null handling
const tasks = await prisma.task.findMany({
  orderBy: {
    updatedAt: { sort: 'asc', nulls: 'last' }
  }
})
```

**Direction values**: `'asc'`, `'desc'`

**Multi-field**: Array of single-key objects. Order in array = priority. Must use separate objects per field (not `{ priority: 'desc', name: 'asc' }` -- that's a single object with unordered keys in JS).

**Sortable field registration**: Auto-generated from `schema.prisma` model. Every scalar field is sortable. Relation fields support sorting by relation's fields (`orderBy: { author: { email: 'asc' } }`).

**Default direction**: `asc`.

**Null handling**: `nulls: 'first'` or `nulls: 'last'` as a sub-object. Only for optional fields. Default: `nulls: 'first'` (nulls come first).

**Type awareness**: Full compile-time type safety. The generated client ensures direction values are valid.

**Composability**: `where`, `orderBy`, `select`, `skip`, `take` are independent top-level options. Fully orthogonal.

---

### 2. OData

**Syntax**: `$orderby` query parameter with comma-separated `field direction` pairs (space-separated).

```
GET /tasks?$orderby=name asc
GET /tasks?$orderby=priority desc,name asc
GET /tasks?$orderby=search.score() desc,rating desc
```

**Direction keywords**: `asc`, `desc` (space-separated after field name).

**Multi-field**: Comma-separated list. Up to 32 sort clauses. Left-to-right priority.

```
$orderby=priority desc, name asc, created_at desc
```

**Sortable field registration**: Any property in the Entity Data Model (EDM) is sortable unless annotated as non-sortable (`Org.OData.Capabilities.V1.SortRestrictions`). The `$metadata` endpoint exposes which properties support sorting.

**Default direction**: `asc` (ascending). If direction is omitted, ascending is assumed.

**Null handling**: Not standardized in the core OData spec. Implementation-dependent. Azure AI Search: `asc` = nulls last, `desc` = nulls first (SQL convention).

**Type awareness**: Types come from the EDM. Server validates that sort fields are of sortable types. Functions like `search.score()` and `geo.distance()` can appear in `$orderby`.

**Composability**: `$filter`, `$select`, `$orderby`, `$top`, `$skip`, `$count` are all independent system query options:

```
GET /tasks?$filter=status eq 'done'&$select=id,name&$orderby=priority desc&$top=10&$skip=20
```

---

### 3. Django ORM

**Syntax**: `.order_by()` with positional string arguments. Minus prefix (`-`) for descending.

```python
Task.objects.order_by('name')                    # ascending (default)
Task.objects.order_by('-priority')               # descending (- prefix)
Task.objects.order_by('status', '-priority')     # multi-field, mixed directions
Task.objects.order_by('?')                       # random order
```

**Direction indicator**: `-` prefix on field name = descending. No prefix = ascending.

**Multi-field**: Multiple positional arguments. Left-to-right priority. Each field independently specifies direction.

```python
Task.objects.filter(status='done').order_by('-priority', 'name')[:10]
```

**Sortable field registration**: Any model field is sortable. No separate declaration needed. Related fields via double-underscore: `order_by('author__name')`. Expressions and annotations also sortable: `order_by(F('priority').desc(nulls_last=True))`.

**Default direction**: Ascending (no prefix).

**Null handling**: Via `F()` expressions with `nulls_first=True` or `nulls_last=True`:

```python
from django.db.models import F
Task.objects.order_by(F('priority').desc(nulls_last=True))
```

Not available in the simple string syntax.

**Type awareness**: Runtime. Django uses the database's native sorting.

**Composability**: `.filter()`, `.order_by()`, `.values()`, `[start:stop]` chain independently.

---

### 4. PostgREST

**Syntax**: `order` query parameter with comma-separated `column.direction` entries (dot-separated).

```
GET /tasks?order=name.asc
GET /tasks?order=priority.desc,name.asc
GET /tasks?order=age.desc,height.asc
```

**Direction suffixes**: `.asc`, `.desc` -- dot-separated after column name.

**Multi-field**: Comma-separated. Left-to-right priority.

**Sortable field registration**: Every column in the PostgreSQL table/view is sortable. No separate declaration.

**Default direction**: `.asc` (ascending). If no direction suffix, ascending is assumed:

```
GET /tasks?order=name              # same as order=name.asc
```

**Null handling**: `.nullsfirst`, `.nullslast` modifiers. Can combine with direction:

```
GET /tasks?order=age.desc.nullslast
GET /tasks?order=age.nullsfirst
```

**Type awareness**: PostgreSQL-native. Sorting uses the column's type and collation.

**Composability**: `select`, filtering, `order`, `limit`, `offset` are all independent:

```
GET /tasks?status=eq.done&select=id,name,status&order=priority.desc&limit=10&offset=20
```

---

### 5. Strapi

**Syntax**: `sort` query parameter with `:asc` / `:desc` suffix.

```
GET /api/tasks?sort=name:asc
GET /api/tasks?sort[0]=priority:desc&sort[1]=name:asc
```

**Direction suffixes**: `:asc` (default, can be omitted), `:desc`.

**Multi-field**: Array syntax `sort[0]=...&sort[1]=...` or comma-separated.

**Sortable field registration**: Every field in the content-type schema is sortable by default.

**Default direction**: Ascending (`:asc` can be omitted).

**Null handling**: Not documented. Follows database behavior.

**Composability**: `filters`, `fields`, `sort`, `pagination` are all independent.

---

### 6. Directus

**Syntax**: `sort` query parameter with `-` prefix for descending. Comma-separated.

```
GET /items/tasks?sort=name                    # ascending (default)
GET /items/tasks?sort=-priority               # descending (- prefix)
GET /items/tasks?sort=-priority,name           # multi-field
GET /items/tasks?sort=author.name              # related field (dot notation)
```

**Direction indicator**: `-` prefix on field name = descending. No prefix = ascending. **Same convention as Django.**

**Multi-field**: Comma-separated string. Left-to-right priority.

**Sortable field registration**: Every field in the collection schema is sortable. No separate declaration.

**Default direction**: Ascending (no prefix).

**Composability**: `filter`, `fields`, `sort`, `limit`, `offset` are all independent.

---

### Cross-Cutting Analysis

#### Direction Syntax Comparison

| System | Ascending | Descending | Multi-field separator | Example |
|--------|-----------|------------|----------------------|---------|
| **Hasura** | `asc` (enum) | `desc` (enum) | Array of objects | `[{name: desc}, {id: asc}]` |
| **Prisma** | `'asc'` | `'desc'` | Array of objects | `[{priority: 'desc'}, {name: 'asc'}]` |
| **OData** | `asc` (keyword) | `desc` (keyword) | Comma + space | `priority desc, name asc` |
| **Django** | no prefix | `-` prefix | Multiple args | `'-priority', 'name'` |
| **PostgREST** | `.asc` (suffix) | `.desc` (suffix) | Comma | `priority.desc,name.asc` |
| **Strapi** | `:asc` (suffix) | `:desc` (suffix) | Array params | `sort[0]=priority:desc` |
| **Directus** | no prefix | `-` prefix | Comma | `-priority,name` |

#### Two Dominant Families

**Family 1: Prefix-based** (Django, Directus)
- `-field` = descending, `field` = ascending
- Most compact syntax
- Easy to parse: check first character

**Family 2: Suffix-based** (PostgREST, Strapi, OData)
- `field.desc` / `field:desc` / `field desc`
- More explicit and self-documenting
- Requires a separator convention

#### Universal Defaults

- **Default direction**: Ascending (every single system).
- **Default null position**: Implementation-dependent; most follow SQL convention.
- **Multi-field priority**: Left-to-right (first field is primary sort key).

#### Sortable Field Declaration Approaches

| Approach | Systems | Notes |
|----------|---------|-------|
| **All fields sortable by default** | Django, PostgREST, Directus, Strapi | Simplest |
| **Schema-derived** | Prisma, Hasura | Auto-generated from data model |
| **Metadata annotation** | OData | Can annotate fields as non-sortable |

---

## Part 2: DSL Syntax Options for agentquery

### Constraints

1. **Grammar**: Args are `key=value` pairs. Keys are identifiers (letters, digits, underscores, hyphens). Values are identifiers or quoted strings.
2. **Token characters**: The tokenizer treats `+` and `-` as unexpected characters (not in `isIdentStart` or `isIdentChar`). A leading `-` in a value would fail tokenization unless quoted.
3. **Backwards compatibility**: `list(status=done, skip=2, take=5) { overview }` must keep working unchanged.
4. **Target audience**: LLM agents that benefit from compact, predictable syntax.
5. **Existing reserved args**: `skip` and `take` are already parsed by `PaginateSlice`. The upcoming filter system will use registered field names as arg keys.

---

### Option A: Django-style Prefix (Single `sort` Arg)

```
list(sort=name) { overview }                       # asc (default)
list(sort=-priority) { overview }                  # desc (- prefix) -- TOKENIZER ISSUE
list(sort="-priority") { overview }                # desc (quoted, works today)
list(sort="priority,-name") { overview }           # multi-sort, comma-separated in quoted string
list(status=done, sort="-priority") { overview }   # filter + sort
```

**Grammar impact**: `sort=-priority` (unquoted) fails because `-` is not `isIdentStart`. Two options:
  - **Require quoting for descending**: `sort="-priority"` -- works today, no parser changes
  - **Extend tokenizer**: Allow `-` as first char of value when preceded by `=` -- targeted parser change

**Brevity / token efficiency**: Excellent for single-field ascending (2 tokens: `sort=name`). Quoted multi-sort adds overhead.

**Multi-sort ergonomics**: Requires packing multiple fields into a single quoted value with internal comma separation. The library needs a secondary parsing step to split the comma-separated list. This is a "string within a string" pattern.

**Error messages**: "invalid sort field '-foo': unknown field 'foo'" -- the `-` prefix makes it clear what direction was intended. Good.

**Composability with filters**: Clean -- `sort` is just another arg key alongside `status`, `skip`, `take`:
```
list(status=done, sort="-priority,name", skip=0, take=5) { overview }
```

**Verdict**: Strong for single-field. Awkward for multi-sort (string packing). Tokenizer issue with `-` for desc.

---

### Option B: Separate sort/order Args

```
list(sort=name, order=asc) { overview }
list(sort=priority, order=desc) { overview }
list(sort="priority,name", order="desc,asc") { overview }   # parallel arrays
```

**Grammar impact**: None. All values are plain identifiers or quoted strings.

**Brevity / token efficiency**: Poor. Two args for what could be one.

**Multi-sort ergonomics**: **Bad.** Parallel arrays (`sort="a,b,c"`, `order="desc,asc,desc"`) are error-prone. A mismatch in count creates confusing errors. What does `sort="priority,name", order=desc` mean?

**Error messages**: "sort has 3 fields but order has 2 directions" -- clear but indicates a footgun.

**Verdict**: Avoid. The parallel-array problem is a known anti-pattern.

---

### Option C: PostgREST-style Dot Notation

```
list(sort=name.asc) { overview }
list(sort=priority.desc) { overview }
list(sort="priority.desc,name.asc") { overview }
```

**Grammar impact**: **Major.** Dot (`.`) is not a valid identifier character. `name.asc` would either tokenize as three tokens (failing) or require adding `.` to the identifier character set (conflicts with future nested field access like `author.name`).

Quoting works: `sort="priority.desc"`, but requiring quotes for every sort arg is ugly.

**Verdict**: Elegant syntax but requires non-trivial parser changes. Not worth it for sorting alone.

---

### Option D: Sort-Prefixed Arg Keys (agentquery-native)

```
list(sort_name=asc) { overview }                    # sort by name ascending
list(sort_priority=desc) { overview }               # sort by priority descending
list(sort_priority=desc, sort_name=asc) { overview } # multi-sort: separate args
list(status=done, sort_priority=desc, skip=0, take=5) { overview }  # full pipeline
```

**Grammar impact**: **None.** All keys are valid identifiers (underscores allowed). All values are plain identifiers (`asc`, `desc`).

**Brevity / token efficiency**: Moderate. `sort_priority=desc` is 19 chars vs `sort=-priority` at 15. For an agent-facing DSL where the primary consumer reads schema examples, the 4 extra characters buy complete explicitness.

**Multi-sort ergonomics**: **Excellent.** Each sort field is a separate arg. No string packing, no parallel arrays. Multi-sort priority is determined by arg order (left-to-right), which the parser preserves in `[]Arg`.

```
list(sort_priority=desc, sort_name=asc, sort_created=desc) { overview }
```

**Error messages**: "unknown sort field 'sort_foo': 'foo' is not a registered sortable field" -- very clear. The `sort_` prefix makes it obvious what the user intended.

**Composability with filters**: Excellent. Sort args sit naturally alongside filter args and pagination args. No ambiguity:

```
list(status=done, sort_priority=desc, sort_name=asc, skip=0, take=5) { overview }
```

The library identifies sort args by the `sort_` prefix, filter args by matching registered filterable fields, and pagination args by reserved `skip`/`take` keys. Clean three-way separation.

**Parsing**: Trivial -- `strings.HasPrefix(arg.Key, "sort_")`. Extract field name as `arg.Key[5:]`. Value must be `asc` or `desc`.

**Schema introspection**: Naturally surfaces as "sortableFields" in `schema()` output. The `sort_` prefix convention is self-documenting for agents.

**Verdict**: Most natural fit for agentquery's grammar. Zero parser changes. Explicit multi-sort. Clear error messages. Slightly more verbose but most robust.

---

### Option E: Single `sort` Arg with Direction Suffix

```
list(sort=name) { overview }                        # asc (default)
list(sort=priority_desc) { overview }               # desc via _desc suffix
list(sort="priority_desc,name_asc") { overview }    # multi-sort
```

**Problem**: How does the library distinguish `priority_desc` (field "priority", direction "desc") from a field literally named "priority_desc"? Only by checking whether the suffix is `_asc` or `_desc` AND the remaining prefix is a registered sortable field. This creates ambiguity.

**Verdict**: Ambiguous. Fragile. Avoid.

---

### Option F: Repeated `sort` Key with Inline Direction

```
list(sort=name, sort="-priority") { overview }      # repeated keys
```

Current parser allows duplicate keys in `[]Arg`. This would technically work. But the `-` still requires quoting, and repeated keys feel non-standard.

**Verdict**: Awkward.

---

### Syntax Comparison Matrix

| Option | Parser changes | Multi-sort | Brevity | Error clarity | Composability | Recommendation |
|--------|---------------|------------|---------|---------------|---------------|----------------|
| **A: Django prefix** | Quoting for desc | Packed string | Best single-field | Good | Good | Viable |
| **B: Separate sort/order** | None | Parallel arrays | Worst | OK | Fragile | Avoid |
| **C: PostgREST dot** | Major | Packed string | Good | Good | Good | Avoid (parser cost) |
| **D: sort_ prefix** | None | Separate args | Moderate | Best | Best | **Recommended** |
| **E: Suffix direction** | None | Packed string | Good | Ambiguous | OK | Avoid |
| **F: Repeated sort key** | None | Repeated keys | Good | Good | Unusual | Avoid |

---

## Part 3: Registration API Design

### 3.1 The Comparator/Closure Pattern

The fundamental tension in sort field registration for a generic library: `FieldAccessor[T]` returns `any`, which means the library would need type-switching or `reflect` to compare values -- ugly, fragile, and incomplete (what about custom enums? priority orderings? date formats?).

Three possible approaches:

| Approach | Registration Signature | Who Owns Comparison Logic |
|----------|----------------------|---------------------------|
| **String accessor** | `func(T) string` | Library (lexicographic) |
| **Typed accessor** | `func(T) int` / `func(T) string` / ... | Library (per-type) |
| **Comparator closure** | `func(a, b T) int` | Consumer |

The comparator closure is the most powerful because it **eliminates the type tension entirely** -- the consumer closure captures the domain knowledge internally.

### 3.2 Go Standard Library Precedent

Go's own standard library settled on this pattern in Go 1.21:

**`slices.SortFunc`**: `func SortFunc[S ~[]E, E any](x S, cmp func(a, b E) int)`

The comparator returns negative if `a < b`, zero if equal, positive if `a > b`. This is the `cmp` convention, matching C's `qsort` and Java's `Comparator<T>`.

**`cmp.Compare`**: Auto-generates comparators for `cmp.Ordered` types (int, float64, string, etc.):

```go
slices.SortFunc(tasks, func(a, b Task) int {
    return cmp.Compare(a.Priority, b.Priority)
})
```

**`cmp.Or`**: Multi-key sort composition using "first non-zero wins":

```go
slices.SortFunc(tasks, func(a, b Task) int {
    return cmp.Or(
        cmp.Compare(a.Status, b.Status),
        cmp.Compare(a.Priority, b.Priority),
    )
})
```

This is directly relevant: our multi-field sort implementation will use this exact pattern internally.

### 3.3 Core Registration Types

```go
// SortComparator defines ordering between two items for a specific field.
// Returns negative if a sorts before b, zero if equal, positive if a sorts after b.
type SortComparator[T any] func(a, b T) int

// SortSpec represents a parsed sort directive: field + direction.
type SortSpec struct {
    Field     string
    Direction SortDirection
}

// SortDirection indicates ascending or descending order.
type SortDirection int

const (
    Asc  SortDirection = iota  // default
    Desc
)
```

### 3.4 Primary Registration: `SortField` with Comparator

```go
// SortField registers a named sortable field with its comparator.
// The comparator defines the "natural" ascending order.
// Descending direction is handled by the framework (negating the result).
func (s *Schema[T]) SortField(name string, cmp SortComparator[T]) {
    s.sortFields[name] = cmp
    if _, exists := s.sortFieldOrder[name]; !exists {
        s.sortFieldNames = append(s.sortFieldNames, name)
    }
}
```

Consumer registration:

```go
// Numeric field -- natural ordering
schema.SortField("priority", func(a, b Task) int {
    return cmp.Compare(a.Priority, b.Priority)
})

// String field -- lexicographic
schema.SortField("name", func(a, b Task) int {
    return cmp.Compare(a.Name, b.Name)
})

// Date field -- time comparison
schema.SortField("created", func(a, b Task) int {
    return a.CreatedAt.Compare(b.CreatedAt)
})

// Custom enum ordering -- consumer defines the rank
schema.SortField("status", func(a, b Task) int {
    rank := map[string]int{"blocked": 0, "todo": 1, "in-progress": 2, "done": 3}
    return rank[a.Status] - rank[b.Status]
})
```

### 3.5 Convenience: `SortFieldOf` -- Auto-Generating Comparators

For the common case where sort order is "compare field values naturally":

```go
// SortFieldOf creates a sort comparator from a typed accessor.
// Works for any cmp.Ordered type (int, string, float64, etc.).
func SortFieldOf[T any, V cmp.Ordered](accessor func(T) V) SortComparator[T] {
    return func(a, b T) int {
        return cmp.Compare(accessor(a), accessor(b))
    }
}
```

Usage:

```go
schema.SortField("name", agentquery.SortFieldOf(func(t Task) string { return t.Name }))
schema.SortField("priority", agentquery.SortFieldOf(func(t Task) int { return t.Priority }))
```

This is the **hybrid approach**: convenience for simple fields, full control for complex ones. Both produce the same `SortComparator[T]` type.

### 3.6 Free-Standing Typed Registration (Alternative)

To match the planned `FilterableField[T]` pattern:

```go
// SortableField registers a field as sortable with a typed accessor.
// Uses cmp.Ordered constraint for compile-time type safety.
func SortableField[T any, V cmp.Ordered](s *Schema[T], name string, accessor func(T) V) {
    s.SortField(name, SortFieldOf[T, V](accessor))
}

// SortableFieldFunc registers a field as sortable with a custom comparator.
func SortableFieldFunc[T any](s *Schema[T], name string, compare func(a, b T) int) {
    s.SortField(name, compare)
}
```

Usage:

```go
agentquery.SortableField(schema, "name", func(t Task) string { return t.Name })
agentquery.SortableField(schema, "priority", func(t Task) int { return t.Priority })
agentquery.SortableFieldFunc(schema, "status", func(a, b Task) int {
    rank := map[string]int{"blocked": 0, "todo": 1, "in-progress": 2, "done": 3}
    return rank[a.Status] - rank[b.Status]
})
```

**Choosing between 3.4 and 3.6**: Both are valid. The method-on-Schema approach (3.4) keeps registration closer to `Field()` and `Preset()`. The free-function approach (3.6) matches `FilterItems`, `PaginateSlice`, `CountItems` -- the existing helper patterns. **Recommendation**: Provide both -- `SortField` as the method (consistent with `Field`, `Preset`, `Operation`) and `SortableField`/`SortableFieldFunc` as top-level generics.

### 3.7 Should Sort Accessors Reuse Field Accessors?

**No.** Field accessors are `func(T) any` for JSON serialization. Sort comparators need typed values for meaningful comparison. They serve different purposes:

- Display accessor: `func(T) any` -- returns the display representation (could be formatted, truncated)
- Sort comparator: `func(a, b T) int` -- compares raw values in a type-safe way

For the common case where they're identical, a convenience function registers both:

```go
// FieldSortable registers a field as both displayable and sortable.
func FieldSortable[T any, V cmp.Ordered](s *Schema[T], name string, accessor func(T) V) {
    s.Field(name, func(t T) any { return accessor(t) })
    SortableField(s, name, accessor)
}
```

### 3.8 Direction (asc/desc) Handling

With a comparator that returns `int`, reversing sort direction is trivial: **negate the result**.

```go
func applySortDirection[T any](cmp SortComparator[T], desc bool) SortComparator[T] {
    if desc {
        return func(a, b T) int { return -cmp(a, b) }
    }
    return cmp
}
```

The comparator always defines the "natural" ascending order. The framework wraps it with negation when `desc` is requested. The consumer never thinks about direction during registration.

### 3.9 Parsing Sort Args (for Option D syntax)

```go
// ParseSortSpecs extracts sort specifications from args.
// Sort args are identified by the "sort_" prefix on the key.
// Value must be "asc" or "desc" (case-insensitive). Default is "asc".
//
// Example args: sort_priority=desc, sort_name=asc
// Returns: [{Field: "priority", Direction: Desc}, {Field: "name", Direction: Asc}]
func ParseSortSpecs(args []Arg) ([]SortSpec, error) {
    var specs []SortSpec
    for _, arg := range args {
        if !strings.HasPrefix(arg.Key, "sort_") {
            continue
        }
        field := arg.Key[5:] // strip "sort_" prefix
        if field == "" {
            return nil, &Error{
                Code:    ErrValidation,
                Message: "sort_ prefix requires a field name",
                Details: map[string]any{"arg": arg.Key},
            }
        }

        dir := Asc
        switch strings.ToLower(arg.Value) {
        case "asc", "":
            dir = Asc
        case "desc":
            dir = Desc
        default:
            return nil, &Error{
                Code:    ErrValidation,
                Message: fmt.Sprintf("sort direction must be 'asc' or 'desc', got %q", arg.Value),
                Details: map[string]any{"field": field, "value": arg.Value},
            }
        }

        specs = append(specs, SortSpec{Field: field, Direction: dir})
    }
    return specs, nil
}
```

### 3.10 Building the Sort Function

```go
// BuildSortFunc constructs a comparison function from sort specs and registered comparators.
// Returns a func(a, b T) int suitable for slices.SortStableFunc.
// If specs is empty, returns nil (no sorting needed).
func BuildSortFunc[T any](specs []SortSpec, sortFields map[string]SortComparator[T]) (func(T, T) int, error) {
    if len(specs) == 0 {
        return nil, nil
    }

    // Validate all fields are registered as sortable
    type step struct {
        compare func(T, T) int
        desc    bool
    }
    steps := make([]step, len(specs))

    for i, spec := range specs {
        cmp, ok := sortFields[spec.Field]
        if !ok {
            return nil, &Error{
                Code:    ErrValidation,
                Message: fmt.Sprintf("field %q is not sortable", spec.Field),
                Details: map[string]any{"field": spec.Field},
            }
        }
        steps[i] = step{compare: cmp, desc: spec.Direction == Desc}
    }

    // Multi-field sort: chain comparators using "first non-zero wins" (cmp.Or pattern)
    return func(a, b T) int {
        for _, s := range steps {
            result := s.compare(a, b)
            if s.desc {
                result = -result
            }
            if result != 0 {
                return result
            }
        }
        return 0
    }, nil
}
```

### 3.11 The `SortSlice` Helper

Following the `PaginateSlice` pattern -- a generic helper that operation handlers opt into:

```go
// SortSlice sorts items in-place using sort specifications extracted from args.
// Validates that all sort fields are registered.
// If no sort_* args are present, items are returned unchanged (no-op).
//
// Uses slices.SortStableFunc for deterministic results when items
// are equal on the sort keys.
//
// Usage in operation handlers:
//
//    items, err := ctx.Items()
//    // ... filter items ...
//    err = agentquery.SortSlice(items, ctx.Statement.Args, schema.SortFields())
//    // ... paginate items ...
func SortSlice[T any](items []T, args []Arg, sortFields map[string]SortComparator[T]) error {
    specs, err := ParseSortSpecs(args)
    if err != nil {
        return err
    }
    if len(specs) == 0 {
        return nil // no sorting requested
    }

    cmpFunc, err := BuildSortFunc(specs, sortFields)
    if err != nil {
        return err
    }

    slices.SortStableFunc(items, cmpFunc)
    return nil
}
```

**Note**: Uses `slices.SortStableFunc` (not `slices.SortFunc`) to preserve relative order of equal elements -- important for deterministic results when sorting by a subset of fields.

### 3.12 Optional: `SortSliceWithDefault`

```go
// SortSliceWithDefault sorts items by sort_* args, falling back to defaultSort
// if no sort args are present. Useful for operations with a natural default order.
func SortSliceWithDefault[T any](
    items []T,
    args []Arg,
    sortFields map[string]SortComparator[T],
    defaultSort func(T, T) int,
) error {
    specs, err := ParseSortSpecs(args)
    if err != nil {
        return err
    }

    if len(specs) == 0 {
        if defaultSort != nil {
            slices.SortStableFunc(items, defaultSort)
        }
        return nil
    }

    cmpFunc, err := BuildSortFunc(specs, sortFields)
    if err != nil {
        return err
    }

    slices.SortStableFunc(items, cmpFunc)
    return nil
}
```

### 3.13 Schema Introspection

Sorting should appear in `schema()` output alongside fields, presets, and operations:

```json
{
  "operations": ["count", "get", "list", "schema", "summary"],
  "fields": ["id", "name", "status", "assignee", "description"],
  "presets": { "overview": ["id", "name", "status", "assignee"] },
  "defaultFields": ["default"],
  "sortableFields": ["name", "priority", "status", "created"],
  "operationMetadata": {
    "list": {
      "description": "List tasks with optional filters, sorting, and pagination",
      "parameters": [
        { "name": "status", "type": "string", "optional": true, "description": "Filter by status" },
        { "name": "sort_<field>", "type": "string", "optional": true, "default": "asc",
          "description": "Sort by field (asc|desc). Sortable: name, priority, status" },
        { "name": "skip", "type": "int", "optional": true, "default": 0 },
        { "name": "take", "type": "int", "optional": true }
      ]
    }
  }
}
```

The `sortableFields` list enables agents to discover available sort fields without external documentation.

### 3.14 Default Sort Behavior

When no `sort_*` args are present, items retain their natural order (as returned by the loader). This matches the current behavior and the principle of least surprise.

Consumers can set a default sort at the operation level using `SortSliceWithDefault`:

```go
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    // ... filter ...
    err = agentquery.SortSliceWithDefault(items, ctx.Statement.Args, schema.SortFields(),
        func(a, b Task) int { return cmp.Compare(a.Priority, b.Priority) })
    // ... paginate ...
}
```

### 3.15 Comparison: Accessor-Based vs. Comparator-Based

#### Accessor-based: `func(T) string` / `func(T) int` (library compares)

**Problems:**
- Library needs separate registration per type (`SortFieldString`, `SortFieldInt`, `SortFieldTime`, ...) or uses `func(T) any` with runtime type-switching.
- Runtime type-switching: what about `[]string`? Custom structs? Error? Panic?
- Custom orderings (enum ranks, reversed dates, null-last) cannot be expressed.
- The existing `FieldAccessor[T]` returns `any` -- fine for JSON but terrible for comparison.

#### Comparator-based: `func(a, b T) int` (consumer compares)

**Advantages:**
- **One type** (`SortComparator[T]`) for all fields -- no type explosion.
- **Consumer owns the logic** -- enum ordering, null handling, locale-aware comparison, derived values.
- **Zero reflection** -- the library never inspects return values.
- **Composable** -- direction = negate, multi-field = chain, stable sort = `slices.SortStableFunc`.
- **Go-idiomatic** -- matches `slices.SortFunc`, `cmp.Compare`, `cmp.Or`.
- **Testable** -- each comparator is a standalone function.

**One trade-off:** Slightly more verbose registration for trivial fields. Fully mitigated by `SortFieldOf`.

**Verdict**: Comparator-based is the clear winner.

---

## Part 4: Recommendation

### Best DSL Syntax: Option D (`sort_<field>=asc|desc`)

```
list(sort_priority=desc, sort_name=asc) { overview }
list(status=done, sort_priority=desc, skip=0, take=5) { overview }
list(sort_name=asc) { overview }
```

**Why Option D over Option A (Django prefix):**

1. **Zero parser changes**: `sort_priority=desc` is already valid grammar. `sort=-priority` requires quoting or parser modification.

2. **Native multi-sort**: Each sort field is a separate arg. No string packing, no secondary parsing.

3. **Clear arg namespace**: `sort_*` args are trivially distinguished from filter args (`status=done`) and pagination args (`skip`/`take`). No ambiguity.

4. **Better error messages**: "field 'foo' is not sortable" -- direct and clean.

5. **Explicit direction**: `sort_priority=desc` is completely unambiguous. For LLM agents (the primary audience), explicitness > brevity.

6. **Consistent with filter pattern**: Filters use `status=done, priority__gt=3`, sort uses `sort_priority=desc`, pagination uses `skip=0, take=5`. Three visually distinct namespaces.

**The tradeoff**: 4 chars more verbose than Django's `-priority` per sort field. Negligible for agent-facing DSL.

### Best Registration API

```go
// Method-based (matches Field, Preset, Operation pattern)
schema.SortField("name", agentquery.SortFieldOf(func(t Task) string { return t.Name }))
schema.SortField("priority", agentquery.SortFieldOf(func(t Task) int { return t.Priority }))
schema.SortField("status", func(a, b Task) int {
    rank := map[string]int{"blocked": 0, "todo": 1, "in-progress": 2, "done": 3}
    return rank[a.Status] - rank[b.Status]
})

// Free-function alternatives (match FilterItems, PaginateSlice pattern)
agentquery.SortableField(schema, "name", func(t Task) string { return t.Name })
agentquery.SortableFieldFunc(schema, "status", func(a, b Task) int { ... })

// Convenience: register as both displayable and sortable
agentquery.FieldSortable(schema, "name", func(t Task) string { return t.Name })
```

### Pipeline Integration

```go
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    if err != nil {
        return nil, err
    }

    // 1. Filter
    filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))
    // (future: filtered := agentquery.FilterItems(items, ctx.Predicate))

    // 2. Sort (NEW)
    if err := agentquery.SortSlice(filtered, ctx.Statement.Args, schema.SortFields()); err != nil {
        return nil, err
    }

    // 3. Paginate
    page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
    if err != nil {
        return nil, err
    }

    // 4. Project
    results := make([]map[string]any, 0, len(page))
    for _, task := range page {
        results = append(results, ctx.Selector.Apply(task))
    }
    return results, nil
}
```

### Arg Namespace Summary

```
list(status=done, sort_priority=desc, sort_name=asc, skip=0, take=5) { overview }
      ^^^^^^^^^^^  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^  ^^^^^^^^^^^       ^^^^^^^^
      Filter args  Sort args (sort_ prefix)          Pagination args   Projection
```

Each category identified by:
- **Filter args**: Key matches a registered filterable field (or `field__operator` pattern)
- **Sort args**: Key has `sort_` prefix
- **Pagination args**: Key is `skip` or `take`
- **Projection**: `{ ... }` block

No ambiguity. No overlap. Each helper extracts the args it cares about and ignores the rest.

### Implementation Complexity Estimate

| Component | Estimated LOC | Notes |
|-----------|---------------|-------|
| `types.go` additions | ~25 | `SortComparator[T]`, `SortSpec`, `SortDirection` |
| `sort.go` (new file) | ~120 | `SortField`, `SortFieldOf`, `SortableField`, `SortableFieldFunc`, `FieldSortable`, `ParseSortSpecs`, `BuildSortFunc`, `SortSlice`, `SortSliceWithDefault` |
| `sort_test.go` (new file) | ~200 | Unit tests: parsing, building, sorting, edge cases, error paths |
| `schema.go` changes | ~15 | `sortFields` map, `SortFields()` getter, introspection update |
| `example/main.go` changes | ~15 | Register sortable fields, use `SortSlice` in `opList` |
| **Total** | **~375** | Comparable to paginate.go (~87 LOC) + tests |

### Dependencies

- Go 1.21+ for `cmp.Ordered`, `cmp.Compare`, `slices.SortStableFunc`
- No external dependencies

### Future Extensions (Not in v1)

1. **Null handling**: `sort_priority=desc_nullslast` -- extend direction parsing or use separate `nulls_*` modifier args
2. **Default sort on schema**: `schema.DefaultSort("priority", Desc)` -- applied when no sort args present
3. **Sort on computed/derived values**: Already covered by custom `SortComparator`
4. **Random sort**: `sort=random` -- special-case arg, not field-based

---

## Sources

- [Hasura Sort Query Results](https://hasura.io/docs/2.0/queries/postgres/sorting/)
- [Hasura DDN Sort Query Results](https://hasura.io/docs/3.0/graphql-api/queries/sorting/)
- [Prisma Filtering and Sorting](https://www.prisma.io/docs/orm/prisma-client/queries/filtering-and-sorting)
- [Prisma orderBy Null Handling (Issue #14377)](https://github.com/prisma/prisma/issues/14377)
- [OData $orderby Syntax - Azure AI Search](https://learn.microsoft.com/en-us/azure/search/search-query-odata-orderby)
- [OData $orderby Specification](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-odata/793b1e83-95ee-4446-8434-f5b634f20d1e)
- [OData Order Rows - Power Apps](https://learn.microsoft.com/en-us/power-apps/developer/data-platform/webapi/query/order-rows)
- [Django QuerySet Order By](https://www.w3schools.com/django/django_queryset_orderby.php)
- [Django ORM Cookbook: Ascending/Descending](https://books.agiliq.com/projects/django-orm-cookbook/en/latest/asc_or_desc.html)
- [PostgREST Tables and Views - Ordering](https://docs.postgrest.org/en/stable/references/api/tables_views.html)
- [Strapi REST API Sort & Pagination](https://docs.strapi.io/cms/api/rest/sort-pagination)
- [Directus Query Parameters](https://directus.io/docs/guides/connect/query-parameters)
- [Go cmp.Or for Multi-Field Sorting (brandur.org)](https://brandur.org/fragments/cmp-or-multi-field)
- [Go slices.SortFunc with cmp.Compare](https://www.dario.dev.br/til/go-sorting/)
- [GraphQL Filtering, Pagination & Sorting Tutorial](https://www.howtographql.com/graphql-js/8-filtering-pagination-and-sorting/)
