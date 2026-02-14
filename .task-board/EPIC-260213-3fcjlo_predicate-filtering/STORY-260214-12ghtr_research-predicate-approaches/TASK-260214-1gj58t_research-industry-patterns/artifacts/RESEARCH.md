# Research: Industry Predicate/Filtering Patterns

## Context

agentquery's current DSL grammar:
```
operation(params) { field_projection }
```

Filtering is currently manual: each operation handler parses `Arg` key-value pairs, builds a predicate `func(T) bool`, and calls `FilterItems[T]`. Example from `example/main.go`:

```go
func taskFilterFromArgs(args []agentquery.Arg) func(Task) bool {
    var filterStatus, filterAssignee string
    for _, arg := range args {
        switch arg.Key {
        case "status":  filterStatus = arg.Value
        case "assignee": filterAssignee = arg.Value
        }
    }
    return func(t Task) bool {
        if filterStatus != "" && !strings.EqualFold(t.Status, filterStatus) { return false }
        if filterAssignee != "" && !strings.EqualFold(t.Assignee, filterAssignee) { return false }
        return true
    }
}
```

This boilerplate repeats across every handler that needs filtering. The goal: move predicate construction into the library so handlers don't write filter logic by hand.

---

## 1. GraphQL

### Syntax

GraphQL itself has **no built-in filter syntax** -- filtering is an application-level concern. However, the dominant pattern (Hasura, Apollo, Gatsby, Contember, Dgraph) converges on a `where` argument taking an Input Type:

```graphql
query {
  tasks(where: { status: { _eq: "done" }, assignee: { _eq: "alice" } }) {
    id
    name
  }
}
```

### Field Registration

Fields available for filtering are defined in the **GraphQL schema** via Input Types:

```graphql
input TaskWhereInput {
  status: StringComparison
  assignee: StringComparison
  priority: IntComparison
  AND: [TaskWhereInput!]
  OR: [TaskWhereInput!]
  NOT: TaskWhereInput
}

input StringComparison {
  _eq: String
  _neq: String
  _in: [String!]
  _contains: String
  _ilike: String
}
```

Each filterable field must be explicitly declared in the Input Type. The schema **is** the registration.

### Operators (Hasura as representative)

| Operator | Meaning |
|----------|---------|
| `_eq` | equals |
| `_neq` | not equal |
| `_gt`, `_gte` | greater than (or equal) |
| `_lt`, `_lte` | less than (or equal) |
| `_in`, `_nin` | in / not in list |
| `_like`, `_ilike` | pattern match (case-sensitive / insensitive) |
| `_similar` | SQL SIMILAR TO |
| `_regex`, `_iregex` | regex match |
| `_is_null` | null check |

### Compound Filters

Logical operators are top-level keys in the where object:

```graphql
where: {
  _and: [
    { status: { _eq: "done" } },
    { _or: [
        { assignee: { _eq: "alice" } },
        { assignee: { _eq: "bob" } }
    ]}
  ]
}
```

Multiple conditions at the same level are implicitly AND-ed.

### Type Safety

Fully type-safe -- the schema defines what operators apply to what types. Invalid filters are caught at query validation time, not runtime.

### Composability with Pagination/Projection

Orthogonal: `where` filters rows, `select` (field list) projects columns, `limit`/`offset` (or `first`/`after` for cursor) paginate.

```graphql
tasks(where: { status: { _eq: "done" } }, limit: 10, offset: 20) {
  id name
}
```

### Key Takeaway for agentquery

The **Input Type pattern** (encapsulate all filters in one structured argument) is the gold standard for API evolution. Apollo's recommendation: start with simple key-value filters, wrap them in an Input Type so you can add operators later without changing the query shape.

---

## 2. OData

### Syntax

OData uses a `$filter` query parameter with an **infix expression language**:

```
GET /tasks?$filter=status eq 'done' and assignee eq 'alice'
GET /tasks?$filter=priority gt 3 and (status eq 'todo' or status eq 'in-progress')
GET /tasks?$filter=contains(name, 'auth')
GET /tasks?$filter=name in ('task-1', 'task-2', 'task-3')
```

### Field Registration

Fields are declared in the **Entity Data Model (EDM)** as properties of entity types. The OData metadata document (`$metadata`) exposes which properties exist and their types. Any property of the entity is filterable unless explicitly annotated as non-filterable.

### Operators

**Comparison:** `eq`, `ne`, `gt`, `ge`, `lt`, `le`

**Logical:** `and`, `or`, `not`

**Functions (string):** `contains(field, 'value')`, `startswith(field, 'value')`, `endswith(field, 'value')`, `toupper()`, `tolower()`, `trim()`, `concat()`, `length()`, `indexof()`, `substring()`

**Functions (date):** `year()`, `month()`, `day()`, `hour()`, `minute()`, `second()`

**Functions (math):** `round()`, `floor()`, `ceiling()`

**Collection:** `any()`, `all()` with lambda expressions

**Membership:** `in` (value in list)

### Compound Filters

Standard boolean algebra with precedence: `not` > `and` > `or`. Parentheses for explicit grouping:

```
$filter=not (status eq 'done') and (priority gt 3 or assignee eq 'alice')
```

### Type Safety

Types come from the EDM. The server validates that operators are applied to compatible types (e.g., `gt` not valid on strings in strict implementations).

### Composability

`$filter`, `$select`, `$orderby`, `$top`, `$skip`, `$count` are all independent query options that compose orthogonally:

```
GET /tasks?$filter=status eq 'done'&$select=id,name,status&$top=10&$skip=20&$count=true
```

### Key Takeaway for agentquery

OData's `$filter` is a **full expression language** -- powerful but heavy. The function-call syntax (`contains(name, 'auth')`) is interesting for our DSL. The operator set (eq/ne/gt/lt/contains/in) is the industry minimum. The separation of `$filter` from `$select` and `$top`/`$skip` validates agentquery's existing separation of filters (args) from projection (braces) and pagination (skip/take args).

---

## 3. Django ORM QuerySet

### Syntax

Django uses **keyword arguments with double-underscore lookups** as the filter syntax:

```python
Task.objects.filter(status="done")                     # implicit __exact
Task.objects.filter(status__in=["done", "in-progress"])
Task.objects.filter(priority__gt=3, assignee="alice")   # AND
Task.objects.filter(name__icontains="auth")
Task.objects.filter(created__range=[start, end])
```

### Field Registration

Fields are **model attributes** -- any field defined on the Django model is automatically filterable. No separate registration needed. The model definition IS the field registry.

```python
class Task(models.Model):
    status = models.CharField(max_length=32)
    priority = models.IntegerField()
    name = models.CharField(max_length=200)
    assignee = models.ForeignKey(User, ...)
```

### Operators (Lookups)

| Lookup | Meaning |
|--------|---------|
| `exact` / `iexact` | exact match (case-sensitive / insensitive) |
| `contains` / `icontains` | substring match |
| `startswith` / `istartswith` | prefix match |
| `endswith` / `iendswith` | suffix match |
| `gt`, `gte`, `lt`, `lte` | comparison |
| `in` | membership |
| `range` | between two values |
| `isnull` | null check |
| `regex` / `iregex` | regex match |

**Default:** when no lookup is specified, `exact` is assumed. `filter(status="done")` is `filter(status__exact="done")`.

### Compound Filters

- **AND:** multiple kwargs in one `.filter()` call, or chained `.filter()` calls
- **OR / NOT / XOR:** `Q` objects with `|`, `~`, `^` operators

```python
from django.db.models import Q

# OR
Task.objects.filter(Q(status="done") | Q(status="in-progress"))

# NOT
Task.objects.filter(~Q(status="done"))

# Complex
Task.objects.filter(
    Q(status="done") | Q(priority__gt=3),
    assignee="alice"  # AND with the Q expression above
)
```

### Type Safety

Runtime type coercion -- Django will try to cast filter values to the field's type but raises `ValueError` / `FieldError` for invalid lookups or types.

### Composability

`.filter()` / `.exclude()` chain with `.values()` / `.values_list()` (projection), `.order_by()`, slicing `[start:stop]` (pagination), `.count()`:

```python
Task.objects.filter(status="done").values("id", "name")[:10]
```

### Key Takeaway for agentquery

The **double-underscore convention** (`field__operator=value`) is remarkably expressive for a flat key-value syntax. It packs field + operator into the key, keeping the value clean. This maps directly to agentquery's existing `key=value` args. Imagine:

```
list(status=done, priority__gt=3) { overview }
```

The `__` separator is already compatible with the tokenizer's identifier rules (underscores are valid in identifiers). The "default to exact" behavior reduces verbosity for the common case.

---

## 4. Prisma

### Syntax

Prisma uses a **typed object model** for filters:

```typescript
const tasks = await prisma.task.findMany({
  where: {
    status: "done",                               // implicit equals
    priority: { gt: 3 },                          // comparison
    name: { contains: "auth", mode: "insensitive" }, // string match
    OR: [
      { assignee: "alice" },
      { assignee: "bob" }
    ]
  },
  select: { id: true, name: true, status: true },
  skip: 20,
  take: 10
})
```

### Field Registration

Prisma generates filter types from the **schema.prisma** data model:

```prisma
model Task {
  id       String @id
  status   String
  priority Int
  name     String
  assignee String?
}
```

The Prisma Client generator automatically creates `TaskWhereInput`, `StringFilter`, `IntFilter`, etc. from this schema. Every model field gets a corresponding filter type based on its data type. No manual registration -- it's derived from the data model.

### Operators

**Scalar (all types):** `equals`, `not`, `in`, `notIn`

**Numeric/Date:** `lt`, `lte`, `gt`, `gte`

**String:** `contains`, `startsWith`, `endsWith` (each with optional `mode: "insensitive"`)

**Null:** `{ not: null }` or implicit `null`

**Relation:** `some`, `every`, `none`, `is`, `isNot`

**List/Array:** `has`, `hasEvery`, `hasSome`, `isEmpty`

### Compound Filters

`AND`, `OR`, `NOT` as top-level keys in the where object:

```typescript
where: {
  AND: [
    { status: "done" },
    { OR: [{ assignee: "alice" }, { priority: { gt: 5 } }] }
  ]
}
```

Multiple conditions at the same level are implicitly AND-ed.

### Type Safety

**Full compile-time type safety.** The generated client ensures:
- Only valid field names are accepted in `where`
- Operators match the field's data type (no `contains` on Int, no `gt` on String without explicit casting)
- Nested relation filters match the related model's filter type

This is the strongest type safety story among all systems reviewed.

### Composability

`where`, `select`, `include`, `orderBy`, `skip`, `take` are independent top-level options on every query method.

### Key Takeaway for agentquery

Prisma's "equals is the default" and "operator as nested object key" pattern keeps simple filters simple while allowing progressive complexity. The **auto-generated filter types from the data model** is the ideal -- in agentquery terms, registered fields should automatically become filterable, with operators determined by a declared field type.

---

## 5. GORM

### Syntax

GORM uses **method chaining** with three condition styles:

```go
// String conditions
db.Where("status = ?", "done").Find(&tasks)
db.Where("priority > ? AND assignee = ?", 3, "alice").Find(&tasks)

// Struct conditions (AND only, zero values are skipped)
db.Where(&Task{Status: "done", Assignee: "alice"}).Find(&tasks)

// Map conditions (AND only, zero values included)
db.Where(map[string]interface{}{"status": "done", "assignee": "alice"}).Find(&tasks)
```

### Field Registration

Fields are **struct fields** annotated with GORM tags. Any exported struct field is queryable. Column names come from naming convention or explicit tags:

```go
type Task struct {
    ID       string `gorm:"primaryKey"`
    Status   string
    Priority int
    Name     string
    Assignee string
}
```

### Operators

GORM doesn't have named operators -- operators are expressed in SQL strings:

- `=`, `<>`, `>`, `>=`, `<`, `<=` (in string conditions)
- `IN`, `LIKE`, `BETWEEN` (in string conditions)
- Struct/Map conditions only support `=` (equality)

For anything beyond equality, you must use string conditions.

### Compound Filters

**AND:** chain `.Where()` calls or use multiple conditions

**OR:** `.Or()` method

**Group conditions (nested AND/OR):**
```go
db.Where(
    db.Where("pizza = ?", "pepperoni").
        Where(db.Where("size = ?", "small").Or("size = ?", "medium")),
).Or(
    db.Where("pizza = ?", "hawaiian").Where("size = ?", "xlarge"),
).Find(&results)
// SQL: (pizza='pepperoni' AND (size='small' OR size='medium'))
//       OR (pizza='hawaiian' AND size='xlarge')
```

### Scopes (Reusable Filters)

Scopes are `func(*gorm.DB) *gorm.DB` -- composable filter functions:

```go
func StatusFilter(status string) func(db *gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        if status != "" {
            return db.Where("status = ?", status)
        }
        return db
    }
}

func PriorityAbove(min int) func(db *gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        return db.Where("priority > ?", min)
    }
}

// Usage: compose scopes
db.Scopes(StatusFilter("done"), PriorityAbove(3)).Find(&tasks)
```

### Type Safety

Minimal -- string conditions have no compile-time type checking. Struct/Map conditions check field existence but not operator validity.

### Composability

Scopes chain naturally with `.Limit()`, `.Offset()`, `.Select()`, `.Order()`.

### Key Takeaway for agentquery

GORM's **Scope pattern** is directly analogous to what agentquery already has with `FilterItems[T]` + custom predicates. The interesting insight is that scopes are **higher-order functions returning modified query builders** -- they compose by chaining, not by building expression trees. For agentquery, the "registered field + declared operator set" approach (like Prisma/GraphQL) is more appropriate than GORM's raw string approach, since we're building a library for non-SQL consumers.

---

## 6. PostgREST

### Syntax

PostgREST uses **URL query parameters** with `column=operator.value` format:

```
GET /tasks?status=eq.done
GET /tasks?priority=gt.3
GET /tasks?status=in.(done,in-progress)
GET /tasks?name=ilike.*auth*
GET /tasks?status=eq.done&assignee=eq.alice     (implicit AND)
GET /tasks?or=(status.eq.done,priority.gt.3)    (explicit OR)
GET /tasks?not.status=eq.done                   (NOT)
```

### Field Registration

Every column in the PostgreSQL table/view is automatically filterable. The metadata comes from the database schema. PostgREST exposes an OpenAPI spec with available columns.

### Operators

| Abbreviation | Meaning |
|-------------|---------|
| `eq` | equals |
| `neq` | not equal |
| `gt`, `gte` | greater than (or equal) |
| `lt`, `lte` | less than (or equal) |
| `like`, `ilike` | pattern match |
| `match`, `imatch` | regex match |
| `in` | in a list |
| `is` | IS (null, true, false) |
| `isdistinct` | IS DISTINCT FROM |
| `cs`, `cd` | contains / contained in |
| `ov` | overlap |
| `fts`, `plfts`, `phfts`, `wfts` | full-text search variants |

**Negation modifier:** `not.` prefix negates any operator: `?status=not.eq.done`

**Quantifier modifiers:** `any` and `all` for list-based operators: `?status=like(any).{done*,in-*}`

### Compound Filters

- **AND:** implicit (multiple query parameters)
- **OR:** `?or=(cond1,cond2)`
- **NOT:** `?not.and=(cond1,cond2)` or prefix on individual conditions
- **Nesting:** `?or=(status.eq.done,not.and(priority.gt.3,assignee.eq.alice))`

### Composability

`select` (vertical filtering) and horizontal filtering (row filtering) are completely independent:

```
GET /tasks?status=eq.done&select=id,name,status&limit=10&offset=20
```

### Key Takeaway for agentquery

PostgREST's `column=operator.value` syntax is **the closest existing pattern to agentquery's `key=value` args**. The dot-separated `operator.value` format packs both the operator and the value into the value position. This could map to agentquery as:

```
list(status.eq=done, priority.gt=3) { overview }
```

Or with a different separator:

```
list(status__eq=done, priority__gt=3) { overview }
```

The negation modifier (`not.eq`) and quantifier modifiers (`like(any)`) are elegant for a compact syntax.

---

## 7. Strapi / Directus

### Strapi Syntax

Strapi uses **nested bracket notation** in URL query strings:

```
GET /api/tasks?filters[status][$eq]=done
GET /api/tasks?filters[priority][$gt]=3
GET /api/tasks?filters[name][$contains]=auth
GET /api/tasks?filters[$and][0][status][$eq]=done&filters[$and][0][priority][$gt]=3
GET /api/tasks?filters[$or][0][status][$eq]=done&filters[$or][1][status][$eq]=in-progress
```

Deep filtering through relations:
```
GET /api/tasks?filters[project][owner][name][$eq]=alice
```

### Strapi Operators

| Operator | Meaning |
|----------|---------|
| `$eq` / `$eqi` | equal (case-sensitive / insensitive) |
| `$ne` / `$nei` | not equal |
| `$gt`, `$gte`, `$lt`, `$lte` | comparison |
| `$in`, `$notIn` | membership |
| `$contains` / `$containsi` | substring (case-sensitive / insensitive) |
| `$notContains` / `$notContainsi` | negated substring |
| `$startsWith` / `$startsWithi` | prefix match |
| `$endsWith` / `$endsWithi` | suffix match |
| `$null`, `$notNull` | null check |
| `$between` | range |
| `$and`, `$or`, `$not` | logical operators |

### Directus Syntax

Directus uses a **JSON filter rules object**:

```json
{
  "status": { "_eq": "done" },
  "priority": { "_gt": 3 },
  "_or": [
    { "assignee": { "_eq": "alice" } },
    { "assignee": { "_eq": "bob" } }
  ]
}
```

URL encoding:
```
GET /items/tasks?filter[status][_eq]=done&filter[priority][_gt]=3
GET /items/tasks?filter={"status":{"_eq":"done"},"priority":{"_gt":3}}
```

### Directus Operators

| Operator | Meaning |
|----------|---------|
| `_eq`, `_neq` | equality |
| `_lt`, `_lte`, `_gt`, `_gte` | comparison |
| `_in`, `_nin` | membership |
| `_null`, `_nnull` | null state |
| `_contains` / `_ncontains` | substring (case-sensitive) |
| `_icontains` / `_nicontains` | substring (case-insensitive) |
| `_starts_with`, `_nstarts_with` | prefix |
| `_ends_with`, `_nends_with` | suffix |
| `_between`, `_nbetween` | range |
| `_empty`, `_nempty` | empty state |
| `_regex` | regex match |
| `_and`, `_or` | logical operators |

Dynamic variables: `$CURRENT_USER`, `$CURRENT_ROLE`, `$NOW`

### Field Registration (Both)

Both systems auto-derive filterable fields from the content model / collection schema. Every field in the schema is filterable by default. Strapi allows marking fields as private or restricting via permissions. Directus uses role-based field permissions.

### Composability

Both compose filtering orthogonally with field selection, pagination, and sorting:
- Strapi: `?filters[...]&fields[0]=id&fields[1]=name&pagination[page]=1&pagination[pageSize]=10`
- Directus: `?filter[...]&fields=id,name&limit=10&offset=20`

### Key Takeaway for agentquery

The `_` prefix convention for operators (Directus) and `$` prefix (Strapi) distinguish operators from field names cleanly. Both systems demonstrate that the **core operator set** (`eq`, `ne`, `gt`, `lt`, `gte`, `lte`, `in`, `contains`, `null`) covers 95% of use cases. The case-insensitive variants (`$containsi`, `_icontains`) and negated variants (`$notContains`, `_ncontains`) show a pattern for operator modifiers.

---

## Cross-Cutting Analysis

### Universal Operator Set

Every system supports these operators (the minimum viable set):

| Operator | Systems | Notes |
|----------|---------|-------|
| **eq** (equals) | ALL 7 | Default in Django, Prisma. Implicit in GORM struct/map |
| **ne / neq** (not equal) | ALL 7 | |
| **gt, gte, lt, lte** | ALL 7 | |
| **in** | ALL 7 | Value is a list |
| **contains** | 6/7 | Not native in GORM (uses LIKE) |

Extended operators present in most but not all:

| Operator | Systems | Notes |
|----------|---------|-------|
| **startsWith** | 5/7 | GraphQL, Django, Prisma, Strapi, Directus |
| **endsWith** | 5/7 | Same as startsWith |
| **icontains** (case-insensitive) | 5/7 | Django, Prisma (mode), Strapi, Directus, PostgREST |
| **null / notNull** | 6/7 | All except GORM (uses IS NULL string) |
| **regex** | 4/7 | Django, PostgREST, Directus, Hasura |
| **between** | 3/7 | Django (range), Strapi, Directus |

### Compound Filter Patterns

Three approaches to AND/OR/NOT:

1. **Implicit AND, explicit OR/NOT** (most common): GraphQL, Prisma, Strapi, Directus, PostgREST
   - Multiple conditions at same level = AND
   - OR/NOT require explicit syntax

2. **All explicit** (OData): `and`, `or`, `not` as infix operators in expression language

3. **Method chaining** (Django, GORM): AND by chaining calls, OR/NOT via Q objects or `.Or()` method

### Field Registration Approaches

| Approach | Systems | Applicability to agentquery |
|----------|---------|---------------------------|
| **Auto-derived from data model** | Prisma, GORM, Django, PostgREST, Strapi, Directus | agentquery already has `Schema.Field()` registration -- this is the analog |
| **Explicit Input Type declaration** | GraphQL | More verbose but most flexible |
| **No explicit registration** | OData (EDM property = filterable) | Similar to auto-derived |

### Default Operator Convention

| System | Default when no operator specified |
|--------|-----------------------------------|
| Django | `exact` (equality) |
| Prisma | `equals` |
| PostgREST | `eq` |
| OData | No default (operator required) |
| Strapi | `$eq` |
| Directus | `_eq` |

**Consensus: equality is the default operator.** When a user writes `status=done`, it means `status equals done`.

---

## Synthesis: Patterns Applicable to agentquery

### Constraint 1: Must Fit the Existing Grammar

Current grammar: `operation(key=value, key=value) { fields }`

The tokenizer supports identifiers (letters, digits, underscores, hyphens) and string values. We can NOT add new syntax characters without parser changes (no `>`, `<`, `!`, `$`, brackets, etc.).

### Constraint 2: Minimal DSL

This is not SQL. The target audience is LLM agents that need a simple, predictable filter syntax. Complex expression trees are overkill.

### Constraint 3: Backwards Compatibility

Existing queries like `list(status=done)` must keep working, meaning equality-by-default.

---

### Approach A: Django-style Double Underscore

Encode field + operator in the key:

```
list(status=done)                    # implicit eq (backwards compatible)
list(status__ne=done)                # not equal
list(priority__gt=3)                 # greater than
list(priority__in="1,3,5")          # in (comma-separated in quoted string)
list(name__contains=auth)            # substring
list(name__icontains=auth)           # case-insensitive substring
list(status=done, priority__gt=3)    # AND (multiple filters)
```

**Pros:**
- Backwards compatible (bare `key=value` = equality)
- No parser changes needed (`__` is valid in identifiers)
- Flat key-value pairs, no nesting
- LLM agents can learn it in one example
- Proven at scale (Django has used this for 20 years)
- Composable with existing skip/take args

**Cons:**
- No native OR/NOT (would need special keys like `_or`, which complicates things)
- Operators are stringly-typed at parse time (validated at execution time)
- `__` in the middle of a key could theoretically clash with field names containing `__`

### Approach B: PostgREST-style Dot in Key

Encode operator after a dot in the key:

```
list(status=done)                    # implicit eq
list(status.ne=done)                 # not equal
list(priority.gt=3)                  # greater than
list(name.contains=auth)             # substring
```

**Pros:**
- Clean, readable
- Backwards compatible

**Cons:**
- Dot (`.`) is NOT currently a valid identifier character -- requires parser change
- Could conflict with nested field access if we ever add that

### Approach C: Operator in Value

Encode operator as a prefix in the value:

```
list(status=done)                    # implicit eq
list(status="ne:done")               # not equal (quoted to include colon)
list(priority="gt:3")                # greater than
list(status="in:done,in-progress")   # in
```

**Pros:**
- No parser changes at all
- Backwards compatible

**Cons:**
- Ugly -- operators hidden inside values
- Harder for LLM agents to parse mentally
- Requires quoting for any operator-prefixed value
- Field type mismatch: value is always a string, even for numeric comparisons

### Approach D: Separate Filter Operation / Reserved Keyword

Add a dedicated filter syntax element:

```
list(where(status=done, priority__gt=3), skip=2, take=5) { overview }
```

Or use a reserved keyword in args:

```
list(filter="status=done,priority>3", skip=2, take=5) { overview }
```

**Pros:**
- Clearly separates filter args from operation args (skip/take/format)
- Could support more complex expressions in the filter string

**Cons:**
- Major parser change (nested parentheses or expression sub-language)
- More complex for agents to construct
- Over-engineered for the use case

### Approach E: Hybrid (Recommended)

Combine Django-style `__` for operators with the existing flat arg model. Library-level changes:

1. **Schema registers filterable fields with types:**
   ```go
   schema.FilterableField("status", FieldTypeString, func(t Task) any { return t.Status })
   schema.FilterableField("priority", FieldTypeInt, func(t Task) any { return t.Priority })
   // Or reuse existing Field() with type annotation:
   schema.Field("status", func(t Task) any { return t.Status }, agentquery.Filterable(agentquery.TypeString))
   ```

2. **Library parses `__operator` suffix at execution time:**
   ```go
   // Internal: split "priority__gt" into field="priority", op="gt"
   // Then validate: is "priority" filterable? does "gt" apply to its type?
   ```

3. **Library builds the predicate automatically:**
   ```go
   // Instead of handler calling FilterItems with hand-built predicate,
   // handler calls:
   filtered, err := ctx.ApplyFilters(items)
   // or the library provides:
   pred := agentquery.BuildPredicate(ctx.Statement.Args, schema.filterableFields)
   filtered := agentquery.FilterItems(items, pred)
   ```

4. **Default operator is equality** -- `status=done` means `status__eq=done`

5. **Reserved arg keys (skip, take) are not treated as filters** -- they're recognized and excluded

6. **AND is implicit** (multiple filter args). No OR/NOT in v1 -- keep it simple. If needed later, add `_or` as a special arg key.

**Minimal operator set for v1:**

| Operator | Suffix | Types | Example |
|----------|--------|-------|---------|
| equals | (none) or `__eq` | all | `status=done` |
| not equals | `__ne` | all | `status__ne=done` |
| greater than | `__gt` | int, string | `priority__gt=3` |
| greater or equal | `__gte` | int, string | `priority__gte=3` |
| less than | `__lt` | int, string | `priority__lt=5` |
| less or equal | `__lte` | int, string | `priority__lte=5` |
| contains | `__contains` | string | `name__contains=auth` |
| in | `__in` | all | `status__in="done,in-progress"` |

**Operator set for v2 (when needed):**

| Operator | Suffix | Types | Example |
|----------|--------|-------|---------|
| case-insensitive contains | `__icontains` | string | `name__icontains=auth` |
| starts with | `__startswith` | string | `name__startswith=auth` |
| ends with | `__endswith` | string | `name__endswith=fix` |
| is null | `__null` | all | `assignee__null=true` |
| regex | `__regex` | string | `name__regex="^auth.*"` |

---

## Summary Table: All Systems at a Glance

| System | Filter Location | Operator Encoding | Default Op | AND | OR | Registration | Type Safety |
|--------|----------------|-------------------|------------|-----|-----|-------------|-------------|
| **GraphQL** | `where` arg (Input Type) | nested object key | (varies) | implicit | `_or` / `OR` | Schema Input Types | compile-time |
| **OData** | `$filter` param | infix operators | none | `and` | `or` | EDM properties | EDM types |
| **Django** | `.filter()` kwargs | `__` suffix on key | `exact` | implicit | Q objects | Model fields | runtime |
| **Prisma** | `where` object | nested object key | `equals` | implicit | `OR` | Schema-generated | compile-time |
| **GORM** | `.Where()` chain | SQL strings | none | chaining | `.Or()` | Struct fields | minimal |
| **PostgREST** | URL params | dot prefix on value | `eq` | implicit | `or=()` | DB columns | DB types |
| **Strapi** | `filters` param | `$` prefix | `$eq` | implicit | `$or` | Content-type fields | runtime |
| **Directus** | `filter` param | `_` prefix | `_eq` | implicit | `_or` | Collection fields | runtime |

---

## Recommendation

**Approach E (Django-style `__` operators in flat args)** is the best fit because:

1. **Zero parser changes** -- `__` is already valid in identifiers
2. **100% backwards compatible** -- bare `key=value` stays as equality
3. **Proven pattern** -- Django has used it for 20 years at massive scale
4. **LLM-friendly** -- flat key-value pairs are trivial for agents to construct
5. **Progressive complexity** -- start with eq/ne/gt/lt/contains/in, add more operators later
6. **Fits the existing grammar** without introducing nesting, brackets, or new token types
7. **Type-safe at registration time** -- schema knows which fields are filterable and what type they are, so invalid operator+type combinations are caught at execution time with good error messages

The only feature this defers is OR/NOT compound logic. Based on the research, AND-only covers the vast majority of real-world filter usage. OR/NOT can be added later via reserved arg keys (`_or`, `_not`) or a dedicated combinator syntax without breaking the base pattern.

---

## Sources

- [GraphQL Queries - graphql.org](https://graphql.org/learn/queries/)
- [GraphQL Search and Filter - Apollo Blog](https://www.apollographql.com/blog/how-to-search-and-filter-results-with-graphql)
- [Hasura Postgres Filters](https://hasura.io/docs/2.0/queries/postgres/filters/index/)
- [OData $filter Queries Explained](https://five.co/blog/odata-filter-queries-explained/)
- [OData URI Conventions](https://www.odata.org/documentation/odata-version-2-0/uri-conventions/)
- [OData $filter Syntax - Azure](https://docs.azure.cn/en-us/search/search-query-odata-filter)
- [Django Making Queries](https://docs.djangoproject.com/en/5.2/topics/db/queries/)
- [Django QuerySet API Reference](https://docs.djangoproject.com/en/4.2/ref/models/querysets/)
- [Prisma Filtering and Sorting](https://www.prisma.io/docs/orm/prisma-client/queries/filtering-and-sorting)
- [GORM Scopes](https://gorm.io/docs/scopes.html)
- [GORM Advanced Query](https://gorm.io/docs/advanced_query.html)
- [PostgREST Tables and Views](https://docs.postgrest.org/en/stable/references/api/tables_views.html)
- [Strapi REST API Filters](https://docs.strapi.io/cms/api/rest/filters)
- [Directus Filter Rules](https://directus.io/docs/guides/connect/filter-rules)
- [Directus Query Parameters](https://directus.io/docs/guides/connect/query-parameters)
