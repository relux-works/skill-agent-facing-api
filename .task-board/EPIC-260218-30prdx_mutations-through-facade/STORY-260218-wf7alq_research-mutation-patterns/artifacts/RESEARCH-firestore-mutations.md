# Firestore Mutation Patterns Research

Research for TASK-260218-20g20f (research-alternative-mutation-patterns).
Focus: patterns transferable to a CLI DSL library in Go.

---

## 1. Document Mutation API

Firestore provides four core mutation operations on `DocumentRef`. Each operates on a single document identified by a path (collection/document).

### 1.1 `Create` -- create a new document (fail if exists)

```go
func (d *DocumentRef) Create(ctx context.Context, data interface{}) (*WriteResult, error)
```

Semantics:
- Creates a new document at the given path.
- **Fails with `ALREADY_EXISTS`** if the document already exists.
- Data can be a struct, `map[string]interface{}`, or any serializable type.

```go
_, err := client.Collection("cities").Doc("LA").Create(ctx, map[string]interface{}{
    "name":       "Los Angeles",
    "state":      "CA",
    "country":    "USA",
    "population": 3900000,
})
// err != nil if "cities/LA" already exists
```

**DSL pattern**: Strict create -- no upsert. Useful when you need to guarantee uniqueness.

### 1.2 `Set` -- create or overwrite (upsert)

```go
func (d *DocumentRef) Set(ctx context.Context, data interface{}, opts ...SetOption) (*WriteResult, error)
```

Semantics:
- Without options: **replaces the entire document** (destructive overwrite).
- With `MergeAll`: only updates provided fields, leaves others intact.
- With `Merge(fieldPaths...)`: selectively merges specific fields.

```go
// Full overwrite (replaces all fields)
_, err := cityRef.Set(ctx, map[string]interface{}{
    "name":    "Los Angeles",
    "state":   "CA",
    "country": "USA",
})

// Merge -- only update specified fields, keep others
_, err := cityRef.Set(ctx, map[string]interface{}{
    "capital": true,
}, firestore.MergeAll)

// Selective merge -- only merge "capital" field
_, err := cityRef.Set(ctx, map[string]interface{}{
    "capital": true,
    "name":    "ignored because not in merge list",
}, firestore.Merge([]string{"capital"}))
```

**DSL pattern**: `set()` is the most flexible -- acts as both create and upsert depending on options. The `MergeAll` vs `Merge(fields)` distinction is worth studying: it's the difference between "update everything I'm sending" vs "update only these specific fields from what I'm sending".

### 1.3 `Update` -- update specific fields (document must exist)

```go
func (d *DocumentRef) Update(ctx context.Context, updates []Update, preconds ...Precondition) (*WriteResult, error)

type Update struct {
    Path      string      // dot-separated for nested: "address.city"
    FieldPath FieldPath   // alternative: []string{"address", "city"}
    Value     interface{} // new value, or a sentinel/transform
}
```

Semantics:
- **Fails with `NOT_FOUND`** if the document doesn't exist.
- Updates only the specified fields -- everything else stays untouched.
- Supports dot notation for nested fields: `"address.city"`.
- Supports preconditions (optimistic concurrency).

```go
_, err := cityRef.Update(ctx, []firestore.Update{
    {Path: "capital", Value: true},
    {Path: "population", Value: 3900000},
    {Path: "regions", Value: firestore.ArrayUnion("west_coast")},
})

// With precondition -- only update if document hasn't changed
snap, _ := cityRef.Get(ctx)
_, err := cityRef.Update(ctx,
    []firestore.Update{{Path: "capital", Value: true}},
    firestore.LastUpdateTime(snap.UpdateTime),
)
```

**DSL pattern**: The `Update` struct is a key design -- it's a list of `{path, value}` pairs, not a full document. This is the "partial update" pattern that avoids accidental field deletion.

### 1.4 `Delete` -- remove a document

```go
func (d *DocumentRef) Delete(ctx context.Context, preconds ...Precondition) (*WriteResult, error)
```

Semantics:
- Deletes the document. **No-op if the document doesn't exist** (no error).
- Supports preconditions to make it conditional.

```go
// Unconditional delete
_, err := client.Collection("cities").Doc("DC").Delete(ctx)

// Conditional delete -- only if it exists (fails with FAILED_PRECONDITION otherwise)
_, err := cityRef.Delete(ctx, firestore.Exists)

// Conditional delete -- only if not modified since last read
_, err := cityRef.Delete(ctx, firestore.LastUpdateTime(snap.UpdateTime))
```

**DSL pattern**: Delete is idempotent by default. Preconditions add strictness when needed.

### 1.5 `Add` -- create with auto-generated ID

```go
func (cr *CollectionRef) Add(ctx context.Context, data interface{}) (*DocumentRef, *WriteResult, error)
```

Semantics:
- Creates a new document with an auto-generated unique ID.
- Returns the `DocumentRef` so you know what ID was assigned.
- Equivalent to `Collection.Doc().Create(data)` -- generates ID client-side, then calls Create.

```go
ref, _, err := client.Collection("cities").Add(ctx, map[string]interface{}{
    "name":    "Tokyo",
    "country": "Japan",
})
fmt.Println("Added document with ID:", ref.ID)
```

**DSL pattern**: This is sugar for "create without specifying an ID". In a CLI DSL context, this maps to an `add` or `create` operation where the system generates the identifier.

### 1.6 Field-Level Operations (Transforms)

Firestore provides sentinel values and transforms that execute atomically on the server side:

```go
// Server timestamp -- set field to server's current time
firestore.Update{{Path: "lastModified", Value: firestore.ServerTimestamp}}

// Increment -- atomically add to a numeric field
firestore.Update{{Path: "population", Value: firestore.Increment(50)}}

// ArrayUnion -- add elements to array (skip duplicates)
firestore.Update{{Path: "regions", Value: firestore.ArrayUnion("west_coast", "pacific")}}

// ArrayRemove -- remove elements from array
firestore.Update{{Path: "regions", Value: firestore.ArrayRemove("east_coast")}}

// Delete a field entirely
firestore.Update{{Path: "tempField", Value: firestore.Delete}}

// Min/Max -- set to minimum/maximum of current and given value
firestore.Update{{Path: "lowestScore", Value: firestore.FieldTransformMinimum(newScore)}}
firestore.Update{{Path: "highestScore", Value: firestore.FieldTransformMaximum(newScore)}}
```

**DSL pattern**: These are **server-side atomic operations** -- the client doesn't need to read-then-write. Instead of `read population -> add 50 -> write`, you say "increment by 50". This eliminates race conditions. In a CLI DSL, these could be modifiers on field values:

```
update(task-1, status="done", priority=increment(1), tags=append("urgent"))
```

---

## 2. Batch Writes

### 2.1 WriteBatch API

```go
type WriteBatch struct{}

func (c *Client) Batch() *WriteBatch

func (b *WriteBatch) Create(doc *DocumentRef, data interface{}) *WriteBatch
func (b *WriteBatch) Set(doc *DocumentRef, data interface{}, opts ...SetOption) *WriteBatch
func (b *WriteBatch) Update(doc *DocumentRef, updates []Update, preconds ...Precondition) *WriteBatch
func (b *WriteBatch) Delete(doc *DocumentRef, preconds ...Precondition) *WriteBatch
func (b *WriteBatch) Commit(ctx context.Context) ([]*WriteResult, error)
```

Key properties:
- **Fluent/chaining API** -- methods return `*WriteBatch` for method chaining.
- **Atomic** -- all-or-nothing. Either all operations succeed or none are applied.
- **No reads** -- batches are write-only. If you need reads, use transactions.
- **Maximum 500 operations** per batch.
- **No partial results** -- if the batch fails, the entire batch is rolled back.

```go
batch := client.Batch()

// Mix different operations in one batch
nycRef := client.Collection("cities").Doc("NYC")
sfRef := client.Collection("cities").Doc("SF")
laRef := client.Collection("cities").Doc("LA")

batch.Set(nycRef, map[string]interface{}{"name": "New York"})
batch.Update(sfRef, []firestore.Update{{Path: "population", Value: 884000}})
batch.Delete(laRef)

// Commit atomically
results, err := batch.Commit(ctx)
if err != nil {
    // ALL operations failed -- batch is atomic
    log.Fatal(err)
}
// results[i] corresponds to operation i
```

### 2.2 BulkWriter (non-atomic, higher throughput)

```go
type BulkWriter struct{}

func (c *Client) BulkWriter(ctx context.Context) *BulkWriter

func (bw *BulkWriter) Create(doc *DocumentRef, data interface{}) (*BulkWriterJob, error)
func (bw *BulkWriter) Set(doc *DocumentRef, data interface{}, opts ...SetOption) (*BulkWriterJob, error)
func (bw *BulkWriter) Update(doc *DocumentRef, updates []Update, preconds ...Precondition) (*BulkWriterJob, error)
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*BulkWriterJob, error)
func (bw *BulkWriter) Flush()
func (bw *BulkWriter) End()
```

Key differences from WriteBatch:
- **Not atomic** -- each operation is independent.
- **No 500-operation limit** -- handles large volumes.
- **Async** -- returns `BulkWriterJob` that you can poll for results.
- **Automatic retry** with exponential backoff.
- **Automatic batching** -- groups operations internally for throughput.

```go
bw := client.BulkWriter(ctx)
jobs := make([]*firestore.BulkWriterJob, 0)

for _, city := range thousandsOfCities {
    job, err := bw.Set(client.Collection("cities").Doc(city.ID), city)
    if err != nil { /* handle */ }
    jobs = append(jobs, job)
}

bw.Flush() // wait for all pending operations
bw.End()   // shut down

for _, job := range jobs {
    result, err := job.Results()
    if err != nil {
        // This specific operation failed
    }
}
```

**DSL pattern**: Two modes of batching:
1. **Atomic batch** (WriteBatch) -- all-or-nothing, limited size. Maps to `BEGIN; ...; COMMIT;` semantics.
2. **Bulk write** (BulkWriter) -- independent operations, unlimited, partial failures allowed. Maps to "fire-and-forget" or "best-effort" batching.

For a CLI DSL, the atomic batch is more relevant. The existing `;` batching in agentquery is non-atomic (each statement independent). Atomic batching would be a new concept.

---

## 3. Transactions

### 3.1 Read-Then-Write Pattern

```go
func (c *Client) RunTransaction(ctx context.Context, f func(context.Context, *Transaction) error, opts ...TransactionOption) error

func MaxAttempts(n int) TransactionOption  // default: 5 attempts

// Inside the transaction function:
func (t *Transaction) Get(dr *DocumentRef) (*DocumentSnapshot, error)
func (t *Transaction) GetAll(drs []*DocumentRef) ([]*DocumentSnapshot, error)
func (t *Transaction) Create(dr *DocumentRef, data interface{}) error
func (t *Transaction) Set(dr *DocumentRef, data interface{}, opts ...SetOption) error
func (t *Transaction) Update(dr *DocumentRef, updates []Update, preconds ...Precondition) error
func (t *Transaction) Delete(dr *DocumentRef, preconds ...Precondition) error
```

**Constraint**: All reads must happen before any writes within the transaction function.

```go
err := client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
    // 1. Read phase
    doc, err := tx.Get(sfRef)
    if err != nil {
        return err
    }
    pop, err := doc.DataAt("population")
    if err != nil {
        return err
    }

    // 2. Write phase (must come after all reads)
    return tx.Update(sfRef, []firestore.Update{
        {Path: "population", Value: pop.(int64) + 1},
    })
})
```

### 3.2 Transactions vs. Batches

| Aspect | Transaction | WriteBatch |
|--------|-------------|------------|
| Reads | Yes (Get, GetAll) | No |
| Writes | Yes | Yes |
| Atomicity | Yes | Yes |
| Max operations | 500 | 500 |
| Retries | Auto-retry on contention | No retry needed |
| Use case | Read-then-write (conditional logic) | Write-only (no reads needed) |
| Concurrency | Optimistic (server SDKs) | N/A |
| Offline support | No (server-side only) | Yes (mobile/web) |

### 3.3 Retry Semantics

- **Server SDKs (Go, Node, Python, Java)**: Use **optimistic concurrency**. Transaction reads are tracked; if any read document is modified by another client before commit, the transaction is aborted and retried.
- **Mobile/Web SDKs**: Use **pessimistic concurrency**. Document locks are acquired on reads.
- Default max retries: **5 attempts** (configurable via `MaxAttempts`).
- If all retries fail, returns an error (typically `ABORTED`).
- The transaction function **must be idempotent** because it may be called multiple times.
- The transaction function **must not have side effects** (no network calls, no logging counters, etc.) because retries would multiply them.

**DSL pattern**: Transactions are inherently imperative (read, compute, write). They don't map cleanly to a declarative DSL. A CLI DSL could either:
- Expose transactions as a special command: `transaction { get(id); update(id, field=value) }`
- Or handle conditional logic server-side with preconditions (simpler, no transaction needed).

---

## 4. Validation Rules (Firestore Security Rules)

Firestore validates writes via **Security Rules** -- a declarative DSL that describes what writes are allowed.

### 4.1 Rule Structure

```
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    match /cities/{city} {
      allow create: if <condition>;
      allow update: if <condition>;
      allow delete: if <condition>;
      allow write: if <condition>;  // shorthand for create+update+delete
    }
  }
}
```

Operations in rules map to mutation types:
- `create` -- new document being created
- `update` -- existing document being modified
- `delete` -- document being removed
- `write` -- any of the above

### 4.2 Available Context in Rules

```
request.resource.data  -- the incoming (future) document state
resource.data          -- the existing document state (null on create)
request.auth           -- authenticated user info
request.time           -- server timestamp of the request
```

### 4.3 Field Type Validation

```
match /cities/{city} {
  allow write: if request.resource.data.name is string
               && request.resource.data.population is int
               && request.resource.data.founded is timestamp
               && request.resource.data.isCapital is bool;
}
```

Valid types: `bool`, `bytes`, `float`, `int`, `list`, `latlng`, `number`, `path`, `map`, `string`, `timestamp`.

### 4.4 Required Fields (hasAll)

```
match /cities/{city} {
  allow create: if request.resource.data.keys().hasAll(['name', 'state', 'country']);
}
```

### 4.5 Allowed Fields (hasOnly)

```
match /cities/{city} {
  allow create: if request.resource.data.keys().hasOnly(['name', 'state', 'country', 'population', 'capital']);
}
```

### 4.6 Combined: Required + Optional Fields

```
function verifyFields(required, optional) {
  let allAllowed = required.concat(optional);
  return request.resource.data.keys().hasAll(required)
      && request.resource.data.keys().hasOnly(allAllowed);
}

match /cities/{city} {
  allow create: if verifyFields(['name', 'state', 'country'], ['population', 'capital', 'regions']);
}
```

### 4.7 Detecting Changed Fields (diff + affectedKeys)

```
match /cities/{city} {
  // Only allow updating name and population, nothing else
  allow update: if request.resource.data.diff(resource.data)
                   .affectedKeys()
                   .hasOnly(['name', 'population']);
}
```

### 4.8 Immutable Fields

```
match /cities/{city} {
  // "createdBy" cannot be changed after creation
  allow update: if request.resource.data.createdBy == resource.data.createdBy;
}
```

### 4.9 Value Constraints

```
match /cities/{city} {
  allow write: if request.resource.data.name is string
               && request.resource.data.name.size() > 0
               && request.resource.data.name.size() < 100
               && request.resource.data.population is int
               && request.resource.data.population > 0;
}
```

### 4.10 Server Timestamp Enforcement

```
match /cities/{city} {
  allow create: if request.resource.data.createdAt == request.time;
  allow update: if request.resource.data.updatedAt == request.time;
}
```

### 4.11 Custom Validation Functions

```
function isValidCity(data) {
  return data.name is string
      && data.name.size() > 0
      && data.population is int
      && data.population >= 0
      && data.country is string;
}

match /cities/{city} {
  allow create: if isValidCity(request.resource.data);
  allow update: if isValidCity(request.resource.data);
}
```

**DSL pattern -- this is gold for mutation design.** Firestore Security Rules are a **declarative validation DSL** that separates concerns:
1. **Schema validation** -- field types, required fields, allowed fields.
2. **Value constraints** -- size limits, range checks.
3. **Change control** -- which fields can be modified, immutable fields.
4. **Operation-specific rules** -- different rules for create vs update vs delete.

For agentquery mutations, a similar pattern could be:
```go
schema.Mutation("update", handler).
    RequiredFields("id").
    AllowedFields("name", "status", "priority").
    ImmutableFields("createdAt", "createdBy").
    Validate(func(old, new T) error { ... })
```

---

## 5. Real-Time Triggers (Cloud Functions)

Firestore mutations can trigger server-side functions. This is the **post-mutation hook** pattern.

### 5.1 Trigger Types (2nd gen)

```typescript
import {
  onDocumentCreated,
  onDocumentUpdated,
  onDocumentDeleted,
  onDocumentWritten,
} from 'firebase-functions/v2/firestore';
```

| Trigger | Fires when | Event data |
|---------|-----------|------------|
| `onDocumentCreated` | New document created | `event.data` = new document snapshot |
| `onDocumentUpdated` | Existing document modified | `event.data.before` + `event.data.after` |
| `onDocumentDeleted` | Document deleted | `event.data` = deleted document snapshot |
| `onDocumentWritten` | Any of the above | `event.data.before` + `event.data.after` (before is null on create, after is null on delete) |

### 5.2 onCreate -- triggered on new document

```typescript
export const onCityCreate = onDocumentCreated('cities/{cityId}', (event) => {
    const newCity = event.data?.data();     // the new document's data
    const cityId = event.params.cityId;     // wildcard parameter

    console.log(`New city created: ${newCity.name} (ID: ${cityId})`);

    // Example: add metadata
    return event.data?.ref.update({
        createdAt: new Date(),
        searchIndex: newCity.name.toLowerCase(),
    });
});
```

### 5.3 onUpdate -- triggered on document modification

```typescript
export const onCityUpdate = onDocumentUpdated('cities/{cityId}', (event) => {
    const before = event.data.before.data();  // previous state
    const after = event.data.after.data();    // new state
    const cityId = event.params.cityId;

    // Only react to population changes
    if (before.population === after.population) {
        return null;  // no-op
    }

    console.log(`Population changed: ${before.population} -> ${after.population}`);

    // Example: update an aggregate
    return event.data.after.ref.update({
        lastPopulationChange: new Date(),
    });
});
```

### 5.4 onDelete -- triggered on document removal

```typescript
export const onCityDelete = onDocumentDeleted('cities/{cityId}', (event) => {
    const deletedData = event.data?.data();   // data before deletion
    const cityId = event.params.cityId;

    console.log(`City deleted: ${deletedData.name}`);

    // Example: cleanup related data
    return admin.firestore().collection('city_stats').doc(cityId).delete();
});
```

### 5.5 Important Semantics

- **At-least-once delivery**: A single event may trigger the function multiple times. Functions must be idempotent.
- **No ordering guarantee**: Rapid changes can trigger functions out of order.
- **No-op writes don't trigger**: An update that doesn't change any data won't fire `onUpdate`.
- **Wildcard parameters**: `{cityId}` in the path becomes `event.params.cityId`.
- **Don't combine `onWrite` with specific triggers**: Using both `onWrite` and `onUpdate` on the same path will cause duplicate handling.

**DSL pattern -- post-mutation hooks.** The trigger model maps cleanly to a hook system:
```go
schema.OnAfterCreate(func(ctx context.Context, item T) error { ... })
schema.OnAfterUpdate(func(ctx context.Context, old, new T) error { ... })
schema.OnAfterDelete(func(ctx context.Context, item T) error { ... })
```

The `before/after` snapshot pattern is particularly useful -- it lets hooks see what changed.

---

## 6. Error Handling

### 6.1 gRPC Error Codes (used by Go SDK)

Firestore's Go SDK uses gRPC status codes. The mutation-relevant codes:

| Code | Name | Mutation context |
|------|------|-----------------|
| 3 | `INVALID_ARGUMENT` | Malformed data, invalid field paths |
| 5 | `NOT_FOUND` | `Update()` on a non-existent document |
| 6 | `ALREADY_EXISTS` | `Create()` when document already exists |
| 7 | `PERMISSION_DENIED` | Security Rules rejected the write |
| 8 | `RESOURCE_EXHAUSTED` | Quota exceeded (writes/sec) |
| 9 | `FAILED_PRECONDITION` | Precondition check failed (`Exists`, `LastUpdateTime`) |
| 10 | `ABORTED` | Transaction conflict -- should retry at higher level |
| 14 | `UNAVAILABLE` | Transient -- retry the same call |

### 6.2 Error Handling in Go

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

_, err := docRef.Create(ctx, data)
if err != nil {
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.AlreadyExists:
            // Document already exists
        case codes.NotFound:
            // Document not found (for Update)
        case codes.FailedPrecondition:
            // Precondition failed (LastUpdateTime mismatch)
        case codes.PermissionDenied:
            // Security rules rejected the write
        case codes.Aborted:
            // Transaction contention -- retry
        default:
            // Other error
        }
    }
}
```

### 6.3 Preconditions (Optimistic Concurrency)

Firestore REST API Precondition type:
```json
{
  // Union field -- only one can be set
  "exists": boolean,      // document must exist (true) or not exist (false)
  "updateTime": string    // document must have been last updated at this timestamp
}
```

Go SDK preconditions:
```go
// Exists -- document must exist (cannot be used with Update, only Delete)
firestore.Exists

// LastUpdateTime -- document must have been updated at exactly this time
firestore.LastUpdateTime(snap.UpdateTime)
```

Usage pattern for optimistic concurrency:
```go
// 1. Read the document
snap, err := docRef.Get(ctx)

// 2. Modify locally
// ...

// 3. Write with precondition -- fails if someone else modified it
_, err = docRef.Update(ctx,
    []firestore.Update{{Path: "status", Value: "done"}},
    firestore.LastUpdateTime(snap.UpdateTime),
)
if err != nil {
    // FAILED_PRECONDITION = someone else changed it
    // Retry: re-read, re-compute, re-write
}
```

### 6.4 Batch Error Handling

- **WriteBatch**: All-or-nothing. `Commit()` returns a single error. If any operation fails, all are rolled back.
- **BulkWriter**: Per-operation results. Each `BulkWriterJob.Results()` returns its own error.
- **Transaction**: `RunTransaction` returns a single error. On contention, it retries automatically (up to `MaxAttempts`, default 5).

**DSL pattern**: The error model for a CLI DSL should distinguish:
1. **Parse errors** -- malformed mutation syntax (already handled by agentquery's parser).
2. **Validation errors** -- mutation violates schema rules (field types, required fields, value constraints).
3. **Precondition errors** -- optimistic concurrency check failed.
4. **Not found / Already exists** -- operation on wrong document state.
5. **Permission errors** -- caller not authorized.

---

## 7. Summary: Patterns Transferable to CLI DSL

### 7.1 Mutation Vocabulary

Firestore uses four verbs: `create`, `set`, `update`, `delete`. The distinction between `create` (fail if exists) and `set` (upsert) is important. `update` is partial (field list), `set` without merge is total (full replacement).

### 7.2 Field-Level Operations

Transforms (`Increment`, `ArrayUnion`, `ArrayRemove`, `ServerTimestamp`, `Min`, `Max`, `Delete`) are powerful because they're **atomic and race-free**. A CLI DSL could support these as value modifiers.

### 7.3 Batching Model

Two tiers:
- **Atomic batch**: All-or-nothing, limited size, write-only.
- **Bulk write**: Independent operations, unlimited, partial failures.

The existing agentquery `;` batching is closest to the bulk write model (independent operations). Atomic batching would be a new feature.

### 7.4 Validation as Declarative Rules

Firestore Security Rules are a **separate DSL** for validation. The key patterns:
- `hasAll(required)` -- required fields.
- `hasOnly(allowed)` -- field whitelist.
- `diff().affectedKeys()` -- track what changed.
- Type checking with `is` operator.
- Separate rules per operation (`create` vs `update` vs `delete`).

### 7.5 Preconditions (Optimistic Concurrency)

Two precondition types: `exists` (boolean) and `updateTime` (timestamp). These are union -- only one at a time. This is a simple, effective model for conditional writes.

### 7.6 Post-Mutation Hooks

Cloud Functions triggers map to `onCreate`, `onUpdate` (with before/after), `onDelete`. These are asynchronous, at-least-once, unordered. For a synchronous CLI DSL, hooks would be pre/post middleware.

### 7.7 Error Categories

Clear error taxonomy: `ALREADY_EXISTS`, `NOT_FOUND`, `FAILED_PRECONDITION`, `PERMISSION_DENIED`, `INVALID_ARGUMENT`, `ABORTED`. Each maps to a specific mutation failure mode.

---

## Sources

- [Go SDK Package Docs](https://pkg.go.dev/cloud.google.com/go/firestore)
- [Go SDK Cloud Reference](https://cloud.google.com/go/docs/reference/cloud.google.com/go/firestore/latest)
- [Add Data to Firestore](https://firebase.google.com/docs/firestore/manage-data/add-data)
- [Delete Data from Firestore](https://firebase.google.com/docs/firestore/manage-data/delete-data)
- [Transactions and Batched Writes](https://firebase.google.com/docs/firestore/manage-data/transactions)
- [Transaction Serializability](https://firebase.google.com/docs/firestore/transaction-data-contention)
- [Security Rules: Data Validation](https://firebase.google.com/docs/rules/data-validation)
- [Security Rules: Conditions](https://firebase.google.com/docs/firestore/security/rules-conditions)
- [Security Rules: Field Access Control](https://firebase.google.com/docs/firestore/security/rules-fields)
- [Security Rules Examples](https://www.sentinelstand.com/article/firestore-security-rules-examples)
- [Cloud Functions Firestore Triggers](https://firebase.google.com/docs/functions/firestore-events)
- [2nd Gen Cloud Functions](https://firebase.google.com/docs/firestore/extend-with-functions-2nd-gen)
- [Firestore REST Precondition](https://cloud.google.com/firestore/docs/reference/rest/v1/Precondition)
- [gRPC Status Codes](https://pkg.go.dev/google.golang.org/grpc/codes)
