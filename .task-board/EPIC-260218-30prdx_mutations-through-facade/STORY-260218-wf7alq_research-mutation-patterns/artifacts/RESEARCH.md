# Mutation Patterns Research: Hasura, PostgREST, Supabase

Research date: 2026-02-18
Context: EPIC-260218 — mutations through facade for agentquery

---

## 1. Hasura: Auto-Generated GraphQL Mutations

### How Mutations Are Generated

Hasura introspects a Postgres database schema and auto-generates a full GraphQL API. For every **tracked table**, it generates:

| Operation | Generated Field | Return Type |
|-----------|----------------|-------------|
| Insert batch | `insert_<table>(objects, on_conflict)` | `<table>_mutation_response` |
| Insert single | `insert_<table>_one(object, on_conflict)` | `<table>` (nullable) |
| Update batch | `update_<table>(where, _set, _inc, ...)` | `<table>_mutation_response` |
| Update by PK | `update_<table>_by_pk(pk_columns, _set, _inc, ...)` | `<table>` (nullable) |
| Update many | `update_<table>_many(updates: [...])` | `[<table>_mutation_response]` |
| Delete batch | `delete_<table>(where)` | `<table>_mutation_response` |
| Delete by PK | `delete_<table>_by_pk(pk_columns)` | `<table>` (nullable) |

### Auto-Generated Types

For a table `article` with columns `{id, title, content, author_id, rating, is_published}`:

```graphql
# Input type for inserts
input article_insert_input {
  id: Int
  title: String
  content: String
  author_id: Int
  rating: Int
  is_published: Boolean
  # nested relation inserts
  author: author_obj_rel_insert_input
  comments: comment_arr_rel_insert_input
}

# Conflict resolution
input article_on_conflict {
  constraint: article_constraint!        # enum of unique constraints
  update_columns: [article_column!]!     # enum of column names
  where: article_bool_exp               # optional condition
}

# Mutation response (batch operations)
type article_mutation_response {
  affected_rows: Int!
  returning: [article!]!
}

# Bool expression for where clauses
input article_bool_exp {
  _and: [article_bool_exp!]
  _or: [article_bool_exp!]
  _not: article_bool_exp
  id: Int_comparison_exp
  title: String_comparison_exp
  content: String_comparison_exp
  # ... one per column
}

# Comparison operators (per scalar type)
input Int_comparison_exp {
  _eq: Int
  _ne: Int
  _gt: Int
  _lt: Int
  _gte: Int
  _lte: Int
  _in: [Int!]
  _nin: [Int!]
  _is_null: Boolean
}

input String_comparison_exp {
  _eq: String
  _ne: String
  _gt: String
  _lt: String
  _like: String
  _nlike: String
  _ilike: String
  _nilike: String
  _in: [String!]
  _nin: [String!]
  _is_null: Boolean
}
```

### Insert Examples

**Single insert:**
```graphql
mutation {
  insert_article_one(
    object: {
      title: "GraphQL Basics"
      content: "Learn GraphQL"
      author_id: 1
    }
  ) {
    id
    title
    content
  }
}
```

**Batch insert:**
```graphql
mutation {
  insert_article(
    objects: [
      { title: "Article 1", content: "Content 1" }
      { title: "Article 2", content: "Content 2" }
    ]
  ) {
    affected_rows
    returning {
      id
      title
    }
  }
}
```

**Nested insert (parent + children in one call):**
```graphql
mutation {
  insert_author_one(
    object: {
      name: "Jane Doe"
      articles: {
        data: [
          { title: "Article 1" }
          { title: "Article 2" }
        ]
      }
    }
  ) {
    id
    name
    articles { id title }
  }
}
```

### Upsert (on_conflict)

**Update specific columns on conflict:**
```graphql
mutation {
  insert_article(
    objects: { title: "Existing Title", content: "New Content" }
    on_conflict: {
      constraint: article_title_key          # which unique constraint
      update_columns: [content]              # which columns to update
    }
  ) {
    affected_rows
    returning { id title content }
  }
}
```

**Conditional upsert with where:**
```graphql
mutation {
  insert_article(
    objects: { title: "My Article", published_on: "2026-01-15" }
    on_conflict: {
      constraint: article_title_key
      update_columns: [published_on]
      where: { published_on: { _lt: "2026-01-15" } }   # only update if new date is later
    }
  ) {
    id
    published_on
  }
}
```

**Ignore on conflict (do nothing):**
```graphql
mutation {
  insert_author(
    objects: { name: "Existing Author" }
    on_conflict: {
      constraint: author_name_key
      update_columns: []                  # empty = do nothing
    }
  ) {
    id
    name
  }
}
```

### Update Examples

**Update with _set (direct value assignment):**
```graphql
mutation {
  update_article(
    where: { id: { _eq: 1 } }
    _set: { rating: 5, is_published: true }
  ) {
    affected_rows
    returning { id rating is_published }
  }
}
```

**Update with _inc (increment):**
```graphql
mutation {
  update_article(
    where: { id: { _eq: 1 } }
    _inc: { likes: 2 }
  ) {
    affected_rows
    returning { id likes }
  }
}
```

**Update by primary key:**
```graphql
mutation {
  update_article_by_pk(
    pk_columns: { id: 1 }
    _set: { is_published: true }
  ) {
    id
    title
    is_published
  }
}
```

**Update many (v2.10+) -- multiple different updates in one transaction:**
```graphql
mutation {
  update_article_many(
    updates: [
      {
        where: { rating: { _lte: 1 } }
        _set: { is_published: false }
      }
      {
        where: { rating: { _gte: 4 } }
        _inc: { likes: 1 }
      }
    ]
  ) {
    affected_rows
    returning { id rating is_published likes }
  }
}
```
Updates execute sequentially. If any fails, entire transaction rolls back.

**JSONB operators:**
```graphql
mutation {
  update_article(
    where: { id: { _eq: 1 } }
    _append: { extra_info: { key: "value" } }       # append to JSONB
    _delete_key: { extra_info: "old_key" }           # remove key from JSONB
    _delete_at_path: { extra_info: ["nested", "path"] }
  ) {
    affected_rows
  }
}
```

### Delete Examples

**Delete by primary key:**
```graphql
mutation {
  delete_article_by_pk(id: 1) {
    id
    title
  }
}
```

**Delete with where clause:**
```graphql
mutation {
  delete_article(where: { rating: { _lte: 1 } }) {
    affected_rows
    returning { id title }
  }
}
```

**Delete all (empty where):**
```graphql
mutation {
  delete_article(where: {}) {
    affected_rows
  }
}
```

### Schema Introspection

Mutations are discoverable via standard GraphQL introspection:
```graphql
{
  __schema {
    mutationType {
      fields {
        name
        args { name type { name kind } }
        type { name kind }
      }
    }
  }
}
```

### Returning Clause

- **Batch operations** (`insert_<table>`, `update_<table>`, `delete_<table>`): return `{ affected_rows: Int!, returning: [<table>!]! }`. The `returning` field supports full field selection (same fields as query).
- **By-PK operations** (`insert_<table>_one`, `update_<table>_by_pk`, `delete_<table>_by_pk`): return the object directly (nullable -- returns null if row doesn't exist).

### Transaction Behavior

Multiple mutations in one GraphQL request execute **sequentially in a single transaction**. If any fails, all roll back.

---

## 2. PostgREST: HTTP Verbs as Mutations

### Verb-to-Operation Mapping

| HTTP Verb | DB Operation | Target | Notes |
|-----------|-------------|--------|-------|
| POST | INSERT | `/table` | Body = row(s) to insert |
| PATCH | UPDATE | `/table?filters` | Body = columns to set |
| PUT | UPSERT (single) | `/table?pk=eq.value` | Full row replacement |
| DELETE | DELETE | `/table?filters` | No body needed |

### Insert (POST)

**Single row:**
```http
POST /articles
Content-Type: application/json

{ "title": "New Article", "content": "...", "author_id": 1 }
```
Response: `201 Created` (no body by default).

**Bulk insert (JSON array):**
```http
POST /articles
Content-Type: application/json

[
  { "title": "Article 1", "content": "Content 1" },
  { "title": "Article 2", "content": "Content 2" }
]
```

**Bulk insert (CSV):**
```http
POST /articles
Content-Type: text/csv

title,content,author_id
Article 1,Content 1,1
Article 2,Content 2,2
```

**Column-restricted insert (ignore extra fields):**
```http
POST /articles?columns=title,content
Content-Type: application/json

{ "title": "Only These", "content": "Columns Matter", "ignored_field": "dropped" }
```

### Upsert

**Merge duplicates (on primary key):**
```http
POST /articles
Content-Type: application/json
Prefer: resolution=merge-duplicates

[
  { "id": 1, "title": "Updated Title", "content": "Updated" },
  { "id": 999, "title": "New Article", "content": "Inserted" }
]
```

**Merge on a UNIQUE column (not PK):**
```http
POST /products?on_conflict=sku
Content-Type: application/json
Prefer: resolution=merge-duplicates

[
  { "sku": "CL2031", "name": "Updated T-shirt", "price": 35 },
  { "sku": "NEW001", "name": "New Cap", "price": 30 }
]
```

**Ignore duplicates:**
```http
POST /articles
Content-Type: application/json
Prefer: resolution=ignore-duplicates

[
  { "id": 1, "title": "Already Exists" },
  { "id": 999, "title": "New Row" }
]
```

**Single-row PUT upsert (full replacement):**
```http
PUT /employees?id=eq.4
Content-Type: application/json

{ "id": 4, "name": "Sara B.", "salary": 60000 }
```
All columns must be present. Creates if not exists, replaces if exists.

### Update (PATCH)

**Basic update with filter:**
```http
PATCH /articles?author_id=eq.5
Content-Type: application/json

{ "is_published": true }
```

**Update with complex filter:**
```http
PATCH /articles?rating=gte.4&is_published=eq.false
Content-Type: application/json

{ "is_published": true }
```

**Limited update (with ordering):**
```http
PATCH /users?last_login=lt.2020-01-01&limit=10&order=id
Content-Type: application/json

{ "status": "inactive" }
```

### Delete

**Delete with filter:**
```http
DELETE /articles?is_published=eq.false
```

**Limited delete:**
```http
DELETE /users?status=eq.inactive&limit=10&order=id
```

### Returning Data

By default, POST/PATCH/DELETE return no body. Add header to get data back:

```http
Prefer: return=representation
```

Returns the affected rows as JSON (same shape as a GET). Can combine with field selection:

```http
DELETE /articles?id=eq.1
Prefer: return=representation

# Response:
[{ "id": 1, "title": "Deleted Article", "content": "..." }]
```

Other return preferences:
- `Prefer: return=minimal` -- no body (default)
- `Prefer: return=headers-only` -- no body, but `Content-Range` header
- `Prefer: return=representation` -- full affected rows

### Filter Operators in Mutations

Same operators available in GET queries work for PATCH/DELETE:
- `eq`, `neq`, `gt`, `gte`, `lt`, `lte`
- `like`, `ilike`
- `in.(value1,value2)`
- `is.null`, `is.true`, `is.false`
- `not.eq.5`, `not.in.(1,2,3)`
- `or=(age.gt.18,status.eq.active)`

---

## 3. Supabase: Client Library Wrapping PostgREST

Supabase's JS client is a thin typed wrapper around PostgREST. Every `.insert()`, `.update()`, `.delete()` call compiles to the corresponding PostgREST HTTP request.

### Insert

**Single insert:**
```javascript
const { error } = await supabase
  .from('articles')
  .insert({ title: 'New Article', content: '...', author_id: 1 })
```

**Bulk insert:**
```javascript
const { error } = await supabase
  .from('articles')
  .insert([
    { title: 'Article 1', content: 'Content 1' },
    { title: 'Article 2', content: 'Content 2' },
  ])
```

**Insert with returning data:**
```javascript
const { data, error } = await supabase
  .from('articles')
  .insert({ title: 'New Article', content: '...' })
  .select()                    // chains Prefer: return=representation
```

**Insert with specific return columns:**
```javascript
const { data, error } = await supabase
  .from('articles')
  .insert({ title: 'New', content: '...' })
  .select('id, title')        // only return these fields
```

### Upsert

**Basic upsert (conflict on PK):**
```javascript
const { data, error } = await supabase
  .from('articles')
  .upsert({ id: 1, title: 'Updated Title', content: 'New content' })
  .select()
```

**Bulk upsert:**
```javascript
const { data, error } = await supabase
  .from('articles')
  .upsert([
    { id: 1, title: 'Updated' },
    { id: 2, title: 'Also Updated' },
  ])
  .select()
```

**Upsert on a UNIQUE column (not PK):**
```javascript
const { data, error } = await supabase
  .from('users')
  .upsert(
    { id: 42, handle: 'saoirse', display_name: 'Saoirse' },
    { onConflict: 'handle' }         // resolve on handle, not id
  )
  .select()
```

**Ignore duplicates:**
```javascript
const { data, error } = await supabase
  .from('articles')
  .upsert(
    [{ id: 1, title: 'Exists' }, { id: 999, title: 'New' }],
    { ignoreDuplicates: true }       // ON CONFLICT DO NOTHING
  )
  .select()
```

**Primary keys must be in the payload.** Supabase needs them to generate the ON CONFLICT clause.

### Update

**Basic update with filter:**
```javascript
const { error } = await supabase
  .from('articles')
  .update({ is_published: true })
  .eq('author_id', 5)
```

**Update with returning:**
```javascript
const { data, error } = await supabase
  .from('articles')
  .update({ rating: 5 })
  .eq('id', 1)
  .select()
```

**Update JSON column:**
```javascript
const { data, error } = await supabase
  .from('users')
  .update({
    address: {
      street: 'Melrose Place',
      postcode: 90210
    }
  })
  .eq('address->postcode', 90210)
  .select()
```

**Update must always have filters.** Calling `.update()` without `.eq()`, `.in()`, etc. affects zero rows (safety).

### Delete

**Delete with filter:**
```javascript
const { error } = await supabase
  .from('articles')
  .delete()
  .eq('id', 1)
```

**Batch delete:**
```javascript
const { error } = await supabase
  .from('articles')
  .delete()
  .in('id', [1, 2, 3])
```

**Delete with returning:**
```javascript
const { data, error } = await supabase
  .from('articles')
  .delete()
  .eq('id', 1)
  .select()            // returns the deleted row(s)
```

**Delete must always have filters.** Same safety as update.

### Available Filter Methods

All usable with `.update()` and `.delete()`:
- `.eq(column, value)` -- equals
- `.neq(column, value)` -- not equals
- `.gt(column, value)` -- greater than
- `.gte(column, value)` -- >=
- `.lt(column, value)` -- less than
- `.lte(column, value)` -- <=
- `.like(column, pattern)` -- LIKE
- `.ilike(column, pattern)` -- case-insensitive LIKE
- `.in(column, [values])` -- IN array
- `.is(column, value)` -- IS (for null, true, false)
- `.match({ col1: val1, col2: val2 })` -- multiple eq filters
- `.or(filters)` -- OR conditions
- `.not(column, operator, value)` -- negation

### Supabase to PostgREST Mapping

| Supabase JS | PostgREST HTTP | SQL |
|-------------|----------------|-----|
| `.insert(obj)` | `POST /table` + JSON body | `INSERT INTO table ...` |
| `.insert([...])` | `POST /table` + JSON array | `INSERT INTO table ... (batch)` |
| `.upsert(obj)` | `POST /table` + `Prefer: resolution=merge-duplicates` | `INSERT ... ON CONFLICT DO UPDATE` |
| `.upsert(obj, {ignoreDuplicates: true})` | `POST /table` + `Prefer: resolution=ignore-duplicates` | `INSERT ... ON CONFLICT DO NOTHING` |
| `.upsert(obj, {onConflict: 'col'})` | `POST /table?on_conflict=col` + merge header | `INSERT ... ON CONFLICT (col) DO UPDATE` |
| `.update(obj).eq('id', 1)` | `PATCH /table?id=eq.1` + JSON body | `UPDATE table SET ... WHERE id = 1` |
| `.delete().eq('id', 1)` | `DELETE /table?id=eq.1` | `DELETE FROM table WHERE id = 1` |
| `.select()` (chained) | `Prefer: return=representation` | `... RETURNING *` |
| `.select('id,name')` (chained) | `Prefer: return=representation` + `select=id,name` | `... RETURNING id, name` |

### Validation

Supabase relies on PostgreSQL constraints for validation:
- NOT NULL constraints
- CHECK constraints
- UNIQUE constraints (trigger conflict handling)
- Foreign key constraints
- Row Level Security (RLS) policies

No client-side validation. Errors propagate as PostgreSQL error codes through PostgREST.

---

## Cross-System Pattern Summary

### Naming Conventions

| Concept | Hasura | PostgREST | Supabase |
|---------|--------|-----------|----------|
| Insert single | `insert_<table>_one()` | `POST /table` (single obj) | `.from(table).insert(obj)` |
| Insert batch | `insert_<table>()` | `POST /table` (array) | `.from(table).insert([...])` |
| Update filtered | `update_<table>(where:)` | `PATCH /table?filters` | `.from(table).update(obj).eq(...)` |
| Update by ID | `update_<table>_by_pk(pk_columns:)` | `PATCH /table?id=eq.X` | `.from(table).update(obj).eq('id', X)` |
| Delete filtered | `delete_<table>(where:)` | `DELETE /table?filters` | `.from(table).delete().eq(...)` |
| Delete by ID | `delete_<table>_by_pk(id:)` | `DELETE /table?id=eq.X` | `.from(table).delete().eq('id', X)` |
| Upsert | `on_conflict: {...}` arg on insert | `Prefer: resolution=merge-duplicates` | `.upsert(obj, {onConflict: ...})` |

### Input Shapes

| System | Insert Input | Update Input | Delete Input |
|--------|-------------|-------------|-------------|
| Hasura | `object:` / `objects:` (typed input) | `_set:`, `_inc:`, etc. + `where:` | `where:` (bool_exp) |
| PostgREST | JSON body (object or array) | JSON body (partial object) + URL filters | URL filters only |
| Supabase | `.insert(object \| array)` | `.update(partial).filters()` | `.delete().filters()` |

### Conflict / Upsert Handling

| System | Mechanism | Specify Conflict Target | Update Columns | Do Nothing |
|--------|-----------|------------------------|----------------|------------|
| Hasura | `on_conflict` arg | `constraint: enum` | `update_columns: [cols]` | `update_columns: []` |
| PostgREST | `Prefer` header | `?on_conflict=col` | All non-PK cols (merge) | `resolution=ignore-duplicates` |
| Supabase | `.upsert()` method | `{ onConflict: 'col' }` | All non-PK cols (merge) | `{ ignoreDuplicates: true }` |

### Returning Data After Mutation

| System | Mechanism | Field Selection |
|--------|-----------|----------------|
| Hasura | `returning { field1 field2 }` in mutation body | Full GraphQL field selection |
| PostgREST | `Prefer: return=representation` header | `?select=field1,field2` query param |
| Supabase | `.select()` or `.select('field1,field2')` chain | Same as PostgREST under the hood |

### Where/Filter in Write Operations

| System | Update Filter | Delete Filter | Operators |
|--------|--------------|---------------|-----------|
| Hasura | `where: { col: { _eq: val } }` (bool_exp) | Same `where:` | `_eq, _ne, _gt, _lt, _gte, _lte, _in, _nin, _like, _ilike, _is_null`, `_and, _or, _not` |
| PostgREST | `?col=eq.val` URL params | Same URL params | `eq, neq, gt, gte, lt, lte, like, ilike, in, is, not` |
| Supabase | `.eq('col', val)` chain | Same filter chain | `.eq, .neq, .gt, .gte, .lt, .lte, .like, .ilike, .in, .is, .match, .or, .not` |

---

## Key Design Insights for agentquery

1. **Separate mutation verbs from queries**: All three systems have distinct syntax for reads vs writes. Hasura: `query {}` vs `mutation {}`. PostgREST: GET vs POST/PATCH/DELETE. Supabase: `.select()` vs `.insert()/.update()/.delete()`.

2. **Naming convention matters**: Hasura's `insert_<table>`, `update_<table>`, `delete_<table>` prefix convention is extremely discoverable. PostgREST reuses the same endpoint with different verbs. For a CLI DSL, Hasura's approach (distinct operation names) maps better.

3. **_one vs batch**: Hasura distinguishes single-item (`_one`, `_by_pk`) from batch operations. Single returns the object directly; batch returns `{ affected_rows, returning }`. This distinction is valuable for agent ergonomics.

4. **Update operators beyond _set**: Hasura's `_inc`, `_append`, `_prepend`, `_delete_key` are powerful for partial updates without read-modify-write. PostgREST/Supabase only support full-column replacement.

5. **Conflict handling is always opt-in**: None of these systems default to upsert. Insert is insert; upsert requires explicit opt-in (`on_conflict`, `Prefer` header, `.upsert()` method).

6. **Returning is opt-in**: All three systems default to NOT returning data after mutations. You must explicitly request it. This is a performance/bandwidth optimization.

7. **Filters in writes use the same language as reads**: Hasura reuses `bool_exp` from queries in mutation `where` clauses. PostgREST reuses query parameter filters. Supabase reuses the same `.eq()/.gt()` chain. The filter language is shared between reads and writes.

8. **Schema introspection includes mutations**: Hasura exposes all mutation types via `__schema { mutationType }`. This is critical for agent discoverability -- agents can discover what writes are available without documentation.

9. **Transaction semantics**: Hasura wraps multiple mutations in a single request into one transaction. PostgREST wraps each request in a transaction. Supabase inherits PostgREST's behavior (one request = one transaction).

10. **Validation is server-side**: All three rely on database constraints (NOT NULL, CHECK, FK, UNIQUE) for validation. No client-side validation DSL. Errors are database errors propagated through the API layer.
