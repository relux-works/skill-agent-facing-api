# Alternative Mutation Patterns Research

Date: 2026-02-18
Task: TASK-260218-20g20f
Context: agentquery library — adding mutation/write support to the existing read-only CLI DSL

**Detailed per-pattern research artifacts:**
- `../.task-board/.../artifacts/RESEARCH-cqrs-command-bus.md`
- `../.task-board/.../artifacts/RESEARCH.md` (Hasura/PostgREST/Supabase)
- `../.task-board/.../artifacts/RESEARCH-trpc-mutations.md`
- `../.task-board/.../artifacts/RESEARCH-firestore-mutations.md`
- `../.task-board/.../artifacts/RESEARCH-cli-read-write-patterns.md`
- `../.task-board/.../artifacts/RESEARCH-agent-facing-mutation-patterns.md`

---

## Table of Contents

1. [CQRS / Command Bus](#1-cqrs--command-bus)
2. [Auto-Generated APIs (Hasura, PostgREST, Supabase)](#2-auto-generated-apis)
3. [tRPC](#3-trpc)
4. [Firebase / Firestore](#4-firebase--firestore)
5. [CLI Tools with Write Support](#5-cli-tools-with-write-support)
6. [Agent-Specific Patterns](#6-agent-specific-patterns)
7. [Comparison Table](#7-comparison-table)
8. [Key Takeaways for agentquery](#8-key-takeaways-for-agentquery)

---

## 1. CQRS / Command Bus

### Core Concepts

Commands are immutable data containers describing **intent to change state**. Key rules:
- **Verb + noun naming** in domain language: `ScheduleTraining`, `CancelOrder` — not `CreateTraining`, `DeleteOrder`
- **One handler per command** (enforced by all Go implementations)
- **Commands carry only the data needed** — no ID generation, no behavior
- Commands traditionally return nothing or minimal metadata (ID, status)

### Handler Registration (Go)

```go
// Watermill: function signature inference
cqrs.NewCommandHandler("BookRoomHandler", handler.Handle)

// Go-MediatR: full generic parameterization
mediatr.RegisterHandler[*CreateProductCommand, *CreateProductResponse](handler)

// go-commandbus: minimal
bus.Register(&CreateUser{}, CreateHandler)
bus.Execute(ctx, &CreateUser{"name"})
```

### Validation: Three-Layer Model

| Layer | Who | What |
|-------|-----|------|
| Structural | Framework (pre-handler) | Required fields, types, format |
| Domain | Handler | Business rules, state transitions |
| Infrastructure | Handler | DB constraints, external services |

The framework can auto-validate Layer 1 from parameter metadata. Layers 2-3 belong in the handler.

### Middleware / Pipeline

Go-MediatR's `PipelineBehavior` and Watermill's `OnHandle` both implement **around-advice** — wraps the handler, runs before AND after:

```go
type PipelineBehavior interface {
    Handle(ctx context.Context, request interface{}, next RequestHandlerFunc) (interface{}, error)
}
```

### Batching Recommendation

Independent execution (same as agentquery's existing read batches). Per-statement errors don't abort the batch. Transactional/saga patterns are too heavy for a CLI DSL — handlers can implement internally if needed.

### Schema Introspection Gap

None of the Go CQRS libraries provide runtime introspection — they rely on Go's type system. This is the gap that agentquery's `schema()` fills.

**Recommendation:** Extend `schema()` with a separate `mutations` section (not mixed into `operations`) so agents clearly distinguish reads from writes.

---

## 2. Auto-Generated APIs

### Hasura: The Most Complete Mutation Model

For every tracked table, Hasura auto-generates **7 mutation types**:

| Operation | Naming | Returns |
|-----------|--------|---------|
| Insert batch | `insert_<table>(objects:, on_conflict:)` | `{ affected_rows, returning }` |
| Insert single | `insert_<table>_one(object:, on_conflict:)` | Object directly (nullable) |
| Update batch | `update_<table>(where:, _set:, _inc:)` | `{ affected_rows, returning }` |
| Update by PK | `update_<table>_by_pk(pk_columns:, _set:)` | Object directly (nullable) |
| Update many | `update_<table>_many(updates: [...])` | `[{ affected_rows, returning }]` |
| Delete batch | `delete_<table>(where:)` | `{ affected_rows, returning }` |
| Delete by PK | `delete_<table>_by_pk(id:)` | Object directly (nullable) |

**Update operators** beyond simple `_set`: `_inc` (increment), `_append`/`_prepend` (JSONB), `_delete_key`/`_delete_elem`/`_delete_at_path`.

**Upsert**: `on_conflict: { constraint: ..., update_columns: [...], where: ... }`. Always explicit opt-in.

**Key insight: Filters in writes reuse the same `bool_exp` type from queries.** Same predicate syntax for reads and writes.

### PostgREST: HTTP Verbs as Mutations

| Verb | SQL | Upsert |
|------|-----|--------|
| POST | INSERT | `Prefer: resolution=merge-duplicates` |
| PATCH | UPDATE (partial) | N/A |
| PUT | UPSERT (full) | On PK only |
| DELETE | DELETE | N/A |

**Returning** is opt-in via `Prefer: return=representation`. Default = no body returned.

### Supabase: Typed Wrapper

```javascript
// Insert with return
await supabase.from('tasks').insert({title: 'New'}).select()
// Update with filter + return
await supabase.from('tasks').update({status: 'done'}).eq('id', 1).select()
// Upsert with conflict column
await supabase.from('tasks').upsert({id: 1, title: 'Updated'}, {onConflict: 'id'}).select()
// Delete with filter + return deleted rows
await supabase.from('tasks').delete().in('id', [1,2,3]).select()
```

**Filters are required on update/delete** for safety — without filters, zero rows affected.

### Cross-System Patterns

1. **Distinct operation names** (Hasura's prefix convention) beat verb-overloading
2. **_one vs batch distinction** is valuable — single returns object, batch returns `{ affected_rows, returning }`
3. **Returning is always opt-in** — default is no data returned
4. **Upsert is always explicit** — never the default
5. **Validation is server-side** via DB constraints — no client-side validation DSL
6. **Schema introspection includes mutations** (critical for agent discoverability)

---

## 3. tRPC

### Core Model

Mutations and queries are **structurally identical procedures**, differentiated only by a **semantic type tag**:

```typescript
// Query
t.procedure.query(({ input }) => db.task.findMany(input));
// Mutation — same shape, different tag
t.procedure.mutation(({ input }) => db.task.create(input));
```

### Input Validation (Zod)

```typescript
t.procedure
  .input(z.object({
    title: z.string().min(1).max(200).describe("Task title"),
    status: z.enum(["todo", "in_progress", "done"]).default("todo"),
  }))
  .mutation(({ input }) => { ... });
```

Zod schemas provide validation, JSON Schema generation, default values, and descriptions — all from one definition.

### Error Handling

Typed error codes: `BAD_REQUEST`, `NOT_FOUND`, `FORBIDDEN`, `CONFLICT`, `UNPROCESSABLE_CONTENT`, `INTERNAL_SERVER_ERROR`. Structured errors with metadata. Validation errors include per-field details via `zodError.flatten()`.

### Batching

tRPC batches multiple procedure calls into a single request. Error isolation — one failure doesn't kill the batch. Response is always an ordered array matching input order. Same mechanism for queries and mutations.

### Discovery

tRPC has **no built-in runtime introspection** (by design). Discovery is via:
- Compile-time type inference
- `trpc-openapi` (third-party, generates OpenAPI spec)
- `trpc-cli` (converts tRPC router to CLI with auto-generated help)

agentquery's built-in `schema()` operation is **better** for agent consumption.

### Key Insight

**No grammar changes needed.** `create(name="X")` uses the same parser as `list(status=done)`. The type tag enables policy: "confirm before mutations", "dry-run for mutations", "log all mutations".

**trpc-cli is direct prior art**: validates that the tRPC procedure model maps cleanly to CLI tools.

---

## 4. Firebase / Firestore

### Mutation Vocabulary

| Method | Semantics | If exists | If not exists |
|--------|-----------|-----------|---------------|
| `Create` | Strict create | ALREADY_EXISTS error | Creates |
| `Set` | Upsert (full or merge) | Overwrites/merges | Creates |
| `Update` | Partial update | Updates fields | NOT_FOUND error |
| `Delete` | Remove | Deletes | No-op (idempotent) |
| `Add` | Create with auto-ID | N/A | Creates |

### Field-Level Operations (Server-Side Atomic)

```go
firestore.Update{
    {Path: "population", Value: firestore.Increment(50)},       // atomic increment
    {Path: "regions", Value: firestore.ArrayUnion("west_coast")}, // add to array
    {Path: "regions", Value: firestore.ArrayRemove("east_coast")}, // remove from array
    {Path: "lastModified", Value: firestore.ServerTimestamp},     // server timestamp
    {Path: "tempField", Value: firestore.Delete},                 // delete field
}
```

These eliminate race conditions — no read-modify-write needed. DSL mapping:
```
update(task-1, status="done", priority=increment(1), tags=append("urgent"))
```

### Batching: Two Tiers

| Mode | Atomicity | Max ops | Failure mode |
|------|-----------|---------|-------------|
| WriteBatch | All-or-nothing | 500 | Entire batch fails |
| BulkWriter | Independent | Unlimited | Per-operation results |

agentquery's `;` batching maps to BulkWriter (independent). Atomic batching would be new.

### Validation Rules (Declarative DSL)

Firestore Security Rules are a **separate validation DSL**:
- `hasAll(required)` — required fields
- `hasOnly(allowed)` — field whitelist
- `diff().affectedKeys()` — track what changed
- Type checking: `is string`, `is int`, etc.
- Separate rules per operation (`create` vs `update` vs `delete`)

**Gold for mutation design.** Maps to:
```go
schema.Mutation("update", handler).
    RequiredFields("id").
    AllowedFields("name", "status", "priority").
    ImmutableFields("createdAt", "createdBy")
```

### Preconditions (Optimistic Concurrency)

Two types: `Exists` (boolean) and `LastUpdateTime` (timestamp). Simple, effective conditional writes without transactions.

### Post-Mutation Hooks

`onCreate`, `onUpdate` (with before/after snapshot), `onDelete`. The before/after pattern lets hooks see exactly what changed.

---

## 5. CLI Tools with Write Support

### How Tools Distinguish Reads from Writes

| Approach | Tools |
|----------|-------|
| Separate commands | kubectl, gh, etcdctl, redis, vault |
| Verb keyword in grammar | SQL (`SELECT` vs `INSERT`) |
| Assignment operator | jq (`.name` vs `.name = "val"`) |
| Type-level separation | GraphQL (`Query` vs `Mutation` type) |
| Separate phases | Terraform (`plan` vs `apply`) |

### Safety Mechanism Spectrum

| Level | Mechanism | Tools |
|-------|-----------|-------|
| 0 — None | Trust the caller | jq, redis-cli, etcdctl |
| 1 — Dry-run | Preview what would happen | kubectl (`--dry-run=client/server`) |
| 2 — Diff | Show exact changes | kubectl (`diff`), terraform (`plan`) |
| 3 — Confirmation | Interactive yes/no | terraform (`apply`), gh (`delete`) |
| 4 — Two-phase | Saved plan + separate apply | terraform (`plan -out` + `apply`) |
| 5 — Ownership | Track who owns what field | kubectl SSA |
| 6 — Transaction | Atomic rollbackable batch | SQL, etcdctl (`txn`), redis (`MULTI/EXEC`) |

### Most Transferable Patterns

**kubectl dry-run** (three levels):
- `--dry-run=client` — local validation, no server contact
- `--dry-run=server` — full validation including webhooks, no persistence
- `kubectl diff` — exact diff before apply

**Terraform plan/apply**: canonical two-phase mutation with change symbols (`+` create, `~` update, `-` delete).

**SQL RETURNING**: mutations return the affected data with field projection:
```sql
INSERT INTO tasks (title) VALUES ('Fix bug') RETURNING id, title, status
```

**etcdctl txn** (compare-and-swap): conditional mutations — "update only if current value matches expected".

**Vault**: versioned mutations with soft delete, hard destroy, rollback. Key=value inline input.

### Key Insight

The most CLI-friendly pattern: **verb-first grammar** (operation name determines read vs write) with **same parser** for both. SQL, GraphQL, and most CLIs follow this.

---

## 6. Agent-Specific Patterns

### MCP Tool Definitions

MCP provides the most mature read/write separation for agents:
- **Resources** = read-only, application-controlled (fetched via URIs)
- **Tools** = model-controlled actions (mutations live here)

**Tool annotations** — the most directly transferable pattern:
| Annotation | Type | Default | Purpose |
|-----------|------|---------|---------|
| `readOnlyHint` | bool | false | Tool doesn't modify environment |
| `destructiveHint` | bool | true | May perform irreversible changes |
| `idempotentHint` | bool | false | Repeated calls with same args are safe |
| `openWorldHint` | bool | true | Interacts with external entities |

Discovery: `tools/list` (paginated). Flat input schemas to reduce token count.

### OpenAI Function Calling

**Strict mode** (`strict: true`): constrained decoding guarantees output matches schema. Requires `additionalProperties: false` on every object. Recommended always for production.

No behavior annotations. Decision entirely from `name`, `description`, `parameters`. Max 10-20 tools per call recommended.

### Anthropic Tool Use

Unique addition: **`input_examples`** — concrete example inputs (schema-validated) alongside the schema. Adds 20-200 tokens per example but dramatically improves model understanding.

Results use typed **content blocks** (text, image, document). `is_error: true` for execution errors — model retries 2-3 times with corrections.

### Design Principles for Agent-Facing Writes

1. **Consolidate multi-step operations**: expose intent-level `complete_task(id)`, not raw CRUD
2. **Token-efficient descriptions**: flat schemas, aggressive enums, semantic field names
3. **Structured > natural language**: JSON Schema gives "fill in the blanks" template
4. **Dry-run / preview**: `dry_run=true` param returns diff of changes
5. **Poka-yoke (mistake-proofing)**: design parameters so invalid invocations are structurally impossible
6. **Corrective error messages**: "status must be one of [todo, in_progress, done], got 'complete'. Did you mean 'done'?"
7. **Return the affected entity**: not just "ok" — the model needs ground truth

---

## 7. Comparison Table

| Feature | CQRS | Hasura/PostgREST | tRPC | Firestore | CLI Tools | Agent APIs (MCP) |
|---------|------|-----------------|------|-----------|-----------|-----------------|
| **Introspection** | None (code-level) | GraphQL `__schema` | None (type-level) | None | `--help` / subcommands | `tools/list` |
| **Validation** | 3-layer pipeline | DB constraints | Zod schemas | Security Rules DSL | N/A | JSON Schema |
| **Batching** | Independent | Transaction (all-or-nothing) | Independent, error isolation | WriteBatch (atomic) or BulkWriter (independent) | Varies | Parallel calls |
| **Error handling** | Structured codes | GraphQL errors | TRPCError codes | gRPC status codes | Exit codes + stderr | `isError` flag |
| **Read/write separation** | Separate types | Separate root type (`mutation`) | Type tag (`query`/`mutation`) | Separate methods | Separate commands/verbs | Resource vs Tool |
| **Dry-run** | No | No | No | Preconditions | kubectl, terraform | No (client decides) |
| **Returning data** | Optional (pragmatic CQRS) | `returning` clause | Always (typed output) | WriteResult only | RETURNING (SQL) | Result content blocks |
| **Field-level ops** | No | `_inc`, `_append`, etc. | No | Increment, ArrayUnion, etc. | No | No |
| **Conflict handling** | Handler responsibility | `on_conflict` clause | Handler responsibility | Preconditions / Transactions | SSA (kubectl) | No |

---

## 8. Key Takeaways for agentquery

### What Every Pattern Agrees On

1. **Same grammar for reads and writes.** No parser changes needed. The operation name determines if it's a read or write. SQL, GraphQL, tRPC, and all CLIs follow this.

2. **Mutations are registered separately from queries** but discovered through the same introspection mechanism. Extend `schema()` with a `mutations` section.

3. **Independent batch execution** (not transactional). Consistent with existing agentquery batch semantics. Handlers can implement transactions internally.

4. **Structured error codes.** Extend existing `Error` type with mutation-specific codes: `CONFLICT`, `FORBIDDEN`, `PRECONDITION_FAILED`.

5. **Validation in two phases.** Framework validates structural (required params, types) from metadata. Handler validates domain rules.

### What's Unique and Valuable from Each Pattern

| Pattern | Unique contribution |
|---------|-------------------|
| CQRS | Command naming (domain verbs, not CRUD), one-handler-per-command, pipeline middleware |
| Hasura | `_one` vs batch distinction, `on_conflict` upsert, update operators (`_inc`, `_set`), filter reuse |
| tRPC | Proof that query/mutation are same structure with type tag, trpc-cli validates CLI mapping |
| Firestore | Field-level atomic ops (Increment, ArrayUnion), declarative validation rules, preconditions, before/after hooks |
| CLI Tools | Dry-run spectrum (none → client → server → diff → confirm → two-phase), RETURNING clause |
| MCP/Agent APIs | Behavior annotations (destructive, idempotent), input_examples, corrective error messages, flat schemas |

### Concrete Recommendations for agentquery

**Registration API:**
```go
s.Mutation("create", createHandler)
s.MutationWithMetadata("create", createHandler, MutationMetadata{
    Description: "Create a new task",
    Parameters: []ParameterDef{
        {Name: "title", Type: "string", Required: true},
        {Name: "status", Type: "string", Enum: []string{"todo","in_progress","done"}, Default: "todo"},
    },
    Destructive: false,
    Idempotent: false,
    Examples:    []string{`create(title="Fix bug")`, `create(title="New feature", status=in_progress)`},
})
```

**Handler type:**
```go
type MutationHandler[T any] func(ctx MutationContext) (any, error)

type MutationContext struct {
    Mutation string
    Args     []Arg
    ArgMap   map[string]string
}
```

**Schema output:**
```json
{
  "operations": ["list", "get", "count", "schema"],
  "mutations": ["create", "update", "delete"],
  "mutationMetadata": {
    "create": {
      "description": "Create a new task",
      "parameters": [
        {"name": "title", "type": "string", "required": true},
        {"name": "status", "type": "string", "enum": ["todo","in_progress","done"], "default": "todo"}
      ],
      "destructive": false,
      "idempotent": false,
      "examples": ["create(title=\"Fix bug\")"]
    }
  }
}
```

**New error codes:**
```go
const (
    ErrConflict       = "CONFLICT"
    ErrForbidden      = "FORBIDDEN"
    ErrPrecondition   = "PRECONDITION_FAILED"
)
```

### Open Design Questions

1. **Should mutations support field projection on the response?** (GraphQL: yes, CQRS: no, SQL RETURNING: yes)
2. **Should `dry_run=true` be a framework convention or handler-level?** (kubectl/terraform: framework, others: handler)
3. **Should mutations be invocable via `Query()` or a separate `Execute()`?** (tRPC: same path, CQRS: separate)
4. **Should the parser distinguish mutations from queries syntactically?** (All evidence says no — registry difference only)
5. **Mixed batches (queries + mutations in one batch)?** (tRPC allows it, GraphQL separates, SQL allows it)

---

## Sources

### CQRS / Command Bus
- [Watermill CQRS Component](https://watermill.io/docs/cqrs/)
- [Basic CQRS in Go — Three Dots Labs](https://threedots.tech/post/basic-cqrs-in-go/)
- [Go-MediatR](https://github.com/mehdihadeli/Go-MediatR)
- [go-mediator — Generic mediator in Go](https://pkritiotis.io/mediator-pattern-in-go/)
- [CQRS command validation — Daniel Whittaker](https://danielwhittaker.me/2016/04/20/how-to-validate-commands-in-a-cqrs-application/)
- [CQRS exception handling — Enterprise Craftsmanship](https://enterprisecraftsmanship.com/posts/cqrs-exception-handling/)

### Auto-Generated APIs
- [Hasura Insert Mutations](https://hasura.io/docs/2.0/mutations/postgres/insert/)
- [Hasura Update Mutations](https://hasura.io/docs/2.0/mutations/postgres/update/)
- [Hasura Upsert Mutations](https://hasura.io/docs/2.0/mutations/postgres/upsert/)
- [PostgREST Tables and Views](https://postgrest.org/en/v12/references/api/tables_views.html)
- [Supabase Insert](https://supabase.com/docs/reference/javascript/insert)
- [Supabase Upsert](https://supabase.com/docs/reference/javascript/upsert)

### tRPC
- [tRPC Define Procedures](https://trpc.io/docs/server/procedures)
- [tRPC Input & Output Validators](https://trpc.io/docs/server/validators)
- [tRPC Error Handling](https://trpc.io/docs/server/error-handling)
- [tRPC Middleware](https://trpc.io/docs/server/middlewares)
- [trpc-cli](https://github.com/mmkal/trpc-cli)

### Firebase / Firestore
- [Add Data to Firestore](https://firebase.google.com/docs/firestore/manage-data/add-data)
- [Transactions and Batched Writes](https://firebase.google.com/docs/firestore/manage-data/transactions)
- [Security Rules: Data Validation](https://firebase.google.com/docs/rules/data-validation)
- [Cloud Functions Firestore Triggers](https://firebase.google.com/docs/functions/firestore-events)
- [Go SDK Firestore Package](https://pkg.go.dev/cloud.google.com/go/firestore)

### CLI Tools
- [kubectl patch](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/update-api-object-kubectl-patch/)
- [kubectl dry-run and diff](https://kubernetes.io/blog/2019/01/14/apiserver-dry-run-and-kubectl-diff/)
- [Terraform plan](https://developer.hashicorp.com/terraform/cli/commands/plan)
- [Terraform apply](https://developer.hashicorp.com/terraform/cli/commands/apply)
- [etcdctl txn](https://etcd.io/docs/v3.5/tutorials/how-to-transactional-write/)
- [CLI Design Guidelines](https://clig.dev/)

### Agent-Specific Patterns
- [MCP Tools Specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)
- [OpenAI Function Calling Guide](https://platform.openai.com/docs/guides/function-calling)
- [Anthropic Tool Use](https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use)
- [Anthropic: Writing Tools for Agents](https://www.anthropic.com/engineering/writing-tools-for-agents)
- [Anthropic: Building Effective Agents](https://www.anthropic.com/research/building-effective-agents)
- [LangChain StructuredTool](https://api.python.langchain.com/en/latest/tools/langchain_core.tools.structured.StructuredTool.html)
- [Semantic Kernel Plugins](https://learn.microsoft.com/en-us/semantic-kernel/concepts/plugins/)
