# GraphQL Mutations: Deep Dive for agentquery Design

**Date**: 2026-02-18
**Task**: TASK-260218-2qd94m
**Purpose**: Research GraphQL mutation patterns to inform agentquery mutation/write support design

---

## 1. Mutation Syntax

### Basic Structure

GraphQL mutations use the same selection-set syntax as queries but under a `mutation` root type:

```graphql
mutation {
  createReview(episode: EMPIRE, review: { stars: 5, commentary: "Great!" }) {
    stars
    commentary
  }
}
```

With variables (the production pattern):

```graphql
mutation CreateReview($ep: Episode!, $review: ReviewInput!) {
  createReview(episode: $ep, review: $review) {
    stars
    commentary
  }
}
# Variables: { "ep": "EMPIRE", "review": { "stars": 5, "commentary": "Great!" } }
```

### Key Syntax Elements

1. **Operation keyword**: `mutation` (vs `query` for reads)
2. **Operation name**: Optional but recommended (e.g., `CreateReview`)
3. **Arguments**: Inline scalars, enums, or input objects
4. **Selection set**: Fields to return from the result (same as queries)
5. **Variables**: `$name: Type!` declarations with separate JSON values

### Schema Definition

```graphql
type Mutation {
  createReview(episode: Episode, review: ReviewInput!): Review
  updateHumanName(id: ID!, name: String!): Human
  deleteStarship(id: ID!): ID!
}
```

Mutations are root-level fields on the `Mutation` type, parallel to `Query`.

---

## 2. Input Types

### Input Object Types

GraphQL has a dedicated `input` keyword for mutation parameters — these are distinct from regular object types:

```graphql
input ReviewInput {
  stars: Int!
  commentary: String
}

input CreateUserInput {
  name: String!
  email: String!
  role: Role = VIEWER    # default value
}
```

### Input vs Object Type Rules

| Aspect | `type` (output) | `input` (input) |
|--------|-----------------|-----------------|
| Used in | Return types | Arguments |
| Can have fields | Yes | Yes |
| Fields can be other types/inputs | Other `type` only | Other `input` only |
| Can have default values | No | Yes |
| Can implement interfaces | Yes | No |
| Can be in unions | Yes | No |

### The Single-Input-Argument Convention

**Best practice** (Apollo, Relay, Shopify): every mutation takes a single `input` argument:

```graphql
# Good: single input object
mutation {
  createUser(input: { name: "Alice", email: "alice@ex.com" }) {
    user { id name }
    userErrors { message field }
  }
}

# Avoid: multiple top-level arguments
mutation {
  createUser(name: "Alice", email: "alice@ex.com") { ... }
}
```

**Why single input**:
- Easier to extend (add fields to input type, no signature change)
- Client codegen is simpler
- Maps cleanly to a single struct in typed languages
- Relay requires it (`clientMutationId` goes in the input)

### Naming Convention for Input Types

```
<MutationName>Input → CreateUserInput, UpdatePostInput, DeleteCommentInput
```

### Scalars in Inputs

- Built-in: `String`, `Int`, `Float`, `Boolean`, `ID`
- Custom scalars: `DateTime`, `URL`, `HTML`, `Email`, `NonEmptyString`
- Enums work in inputs: `role: Role!`
- Non-null (`!`) enforces required at schema level

### Nested Inputs

Inputs can nest other input types for structured data:

```graphql
input AddressInput {
  street: String!
  city: String!
  country: String!
}

input CreateUserInput {
  name: String!
  address: AddressInput
  tags: [String!]
}
```

---

## 3. Return Types (Payloads)

### Three Common Patterns

#### Pattern A: Return the Mutated Object Directly

```graphql
type Mutation {
  createUser(input: CreateUserInput!): User
  deleteStarship(id: ID!): ID!
}
```

Simple but limited — no room for metadata, errors, or additional data.

#### Pattern B: Payload Wrapper (Recommended — Shopify/Relay Standard)

```graphql
type CreateUserPayload {
  user: User
  userErrors: [UserError!]!
}

type UserError {
  field: [String!]    # path to the problematic input field
  message: String!
  code: UserErrorCode  # optional enum for machine-readable errors
}

type Mutation {
  createUser(input: CreateUserInput!): CreateUserPayload
}
```

Each mutation gets its own `<MutationName>Payload` type. This is the **industry standard** (Shopify, GitHub, Stripe use this pattern).

**Response on success**:
```json
{
  "data": {
    "createUser": {
      "user": { "id": "123", "name": "Alice" },
      "userErrors": []
    }
  }
}
```

**Response on validation failure**:
```json
{
  "data": {
    "createUser": {
      "user": null,
      "userErrors": [
        {
          "field": ["input", "email"],
          "message": "Email is already taken",
          "code": "TAKEN"
        }
      ]
    }
  }
}
```

#### Pattern C: Union Result Types (Type-Safe Errors)

```graphql
type User { id: ID!, name: String! }

type UserNotFoundError { message: String! }
type EmailTakenError { message: String!, suggestedEmail: String }

union CreateUserResult = User | EmailTakenError
union UpdateUserResult = User | UserNotFoundError

type Mutation {
  createUser(input: CreateUserInput!): CreateUserResult!
  updateUser(input: UpdateUserInput!): UpdateUserResult!
}
```

Client queries use inline fragments:
```graphql
mutation {
  createUser(input: { ... }) {
    ... on User { id name }
    ... on EmailTakenError { message suggestedEmail }
  }
}
```

More type-safe but verbose. Can combine with an `Error` interface:

```graphql
interface Error {
  message: String!
}

type EmailTakenError implements Error {
  message: String!
  suggestedEmail: String
}

type InvalidInputError implements Error {
  message: String!
  field: String!
}
```

### Comparison

| Aspect | Direct Return | Payload Wrapper | Union Result |
|--------|--------------|-----------------|--------------|
| Simplicity | High | Medium | Low |
| Error richness | None | Good | Best |
| Extensibility | Poor | Excellent | Good |
| Agent-friendliness | Highest | High | Medium (needs `__typename`) |
| Industry adoption | Rare | Dominant | Growing |

---

## 4. Error Handling

GraphQL has **two layers** of errors:

### Layer 1: Top-Level Errors (Transport/System)

The `errors` array in the response. These represent:
- Syntax errors in the query
- Authorization failures
- Server errors
- Type mismatches

```json
{
  "errors": [
    {
      "message": "Cannot query field 'foo' on type 'User'",
      "locations": [{ "line": 3, "column": 5 }],
      "extensions": {
        "code": "GRAPHQL_VALIDATION_FAILED"
      }
    }
  ],
  "data": null
}
```

Clients should treat top-level errors as **infrastructure failures** — the request itself was malformed or unauthorized.

### Layer 2: Business/Domain Errors (In Payload)

These are **expected error cases** — validation failures, business rule violations, not-found conditions. They belong in the mutation payload, NOT in the top-level errors array.

**Shopify's rule**: Top-level `errors` = client/server errors. `userErrors` in payload = domain errors.

```json
{
  "data": {
    "createUser": {
      "user": null,
      "userErrors": [
        { "field": ["input", "email"], "message": "Email already taken" },
        { "field": ["input", "name"], "message": "Name cannot be blank" }
      ]
    }
  }
}
```

### Error Path Convention

The `field` array traces the path through the input object:
- `["input", "email"]` — the `email` field of the `input` argument
- `["input", "address", "city"]` — nested input field

This is crucial for agents and UIs that need to know **which input** caused the error.

### Partial Success in Batches

When multiple mutations run in one request, each mutation has its own payload. One can fail while others succeed — there's **no automatic rollback**:

```json
{
  "data": {
    "createUser": { "user": { "id": "1" }, "userErrors": [] },
    "deleteUser": { "deletedId": null, "userErrors": [{ "message": "Not found" }] }
  }
}
```

---

## 5. Batching

### Spec-Level Guarantee

The GraphQL spec states: **"Mutation fields execute serially, in the order listed."** This is the critical difference from queries (which MAY execute in parallel).

```graphql
mutation {
  first: createUser(input: { name: "Alice" }) { user { id } }
  second: createUser(input: { name: "Bob" }) { user { id } }
  third: deleteUser(input: { id: "old-user" }) { deletedId }
}
```

Execution order: `first` → `second` → `third`, guaranteed.

### Aliases Required for Same-Type Batching

If you batch multiple invocations of the same mutation, you MUST use aliases:

```graphql
mutation {
  alice: createUser(input: { name: "Alice" }) { user { id } }
  bob: createUser(input: { name: "Bob" }) { user { id } }
}
```

Without aliases, the second `createUser` would overwrite the first in the response object.

### Batch Error Semantics

- Each mutation in the batch executes independently
- A failure in one does NOT abort the rest
- No automatic transaction/rollback semantics
- Each mutation's result (or error) appears under its alias in the response
- Some implementations (Hasura, Data API Builder) optionally wrap batches in transactions

### Comparison with agentquery Batching

agentquery already has `;`-separated batch queries with the same semantics:
- Sequential execution
- Per-statement error isolation
- No rollback

```
list(status=done) { overview }; count()
```

This is **directly analogous** to GraphQL mutation batching. The pattern transfers naturally.

---

## 6. Schema Introspection

### How Mutations Appear in Introspection

GraphQL's `__schema` exposes mutations alongside queries:

```graphql
{
  __schema {
    mutationType {
      name
      fields {
        name
        description
        args {
          name
          type {
            name
            kind
            ofType { name kind }
          }
          defaultValue
        }
        type {
          name
          kind
          fields { name type { name } }
        }
      }
    }
  }
}
```

### What Introspection Reveals

For each mutation field:
- **Name**: `createUser`
- **Description**: Human-readable purpose
- **Args**: Each argument with its type (including nested input types)
- **Return type**: The payload type with its fields

For input types:
- All fields, their types, whether required (`NON_NULL` wrapper), default values

### Agent Discovery Flow

1. Query `__schema.mutationType.fields` → discover available mutations
2. For each mutation, inspect `args` → learn what input is needed
3. Inspect arg type recursively → understand the full input shape
4. Inspect return type → know what to request back

This is **self-documenting** — an agent can build a valid mutation call purely from introspection. No external docs needed.

### Relevance to agentquery

agentquery already has `schema()` introspection that lists operations, parameters, and examples. Extending this to include mutation operations is straightforward — they'd just be additional operations with `"kind": "mutation"` or similar tagging.

---

## 7. Naming Conventions

### Mutation Names

**Pattern**: `verbNoun` in camelCase

| Operation | Convention | Examples |
|-----------|-----------|----------|
| Create | `create<Type>` | `createUser`, `createPost` |
| Update | `update<Type>` | `updateUser`, `updatePost` |
| Delete | `delete<Type>` | `deleteUser`, `deletePost` |
| Toggle | `toggle<Property>` | `toggleTodoCompleted` |
| Specific action | `verb<Noun>` | `likePost`, `archiveProject`, `assignTask` |
| Bulk | `bulk<Verb><Type>` | `bulkDeleteUsers`, `bulkUpdatePosts` |

**Shopify's rule**: Make mutations as **specific** as possible. `updateUserEmail` is better than `updateUser` when the action is focused.

### Input Type Names

```
<MutationName>Input → CreateUserInput, ToggleTodoCompletedInput
```

### Payload Type Names

```
<MutationName>Payload → CreateUserPayload, DeletePostPayload
```

### For agentquery

The DSL uses `operation_name(args)` not `verbNoun`, so the convention would be CLI-style:

```
create(type=user, name="Alice", email="a@b.com")
update(task-1, status=done)
delete(task-1)
assign(task-1, to=alice)
```

Or operation-per-entity:

```
create-user(name="Alice")
update-task(task-1, status=done)
```

The second pattern is more discoverable via `schema()`.

---

## 8. Validation

### Three Layers of Validation in GraphQL

#### Layer 1: Schema-Level Type Checking (Automatic)

The GraphQL runtime validates that:
- Required fields (`!`) are present
- Types match (`String` is a string, `Int` is an integer)
- Enum values are valid
- Input object structure matches the schema

This is **free** — no resolver code needed. Invalid inputs are rejected before the resolver runs.

```graphql
input CreateUserInput {
  name: String!       # required string
  age: Int            # optional int
  role: Role = VIEWER # enum with default
}
```

#### Layer 2: Custom Scalars (Type-Enhanced)

Custom scalars add semantic validation:

```graphql
scalar Email       # validates email format
scalar URL         # validates URL format
scalar NonEmptyString  # rejects empty strings, trims whitespace
scalar DateTime    # validates ISO 8601
```

The validation runs during input coercion — before the resolver sees the data.

#### Layer 3: Business Logic Validation (In Resolver)

Complex rules that cross fields or require DB lookups:

```javascript
const resolvers = {
  Mutation: {
    createUser: async (_, { input }, context) => {
      const errors = [];

      if (await context.db.emailExists(input.email)) {
        errors.push({ field: ["input", "email"], message: "Email taken" });
      }

      if (input.age && input.age < 13) {
        errors.push({ field: ["input", "age"], message: "Must be 13+" });
      }

      if (errors.length > 0) {
        return { user: null, userErrors: errors };
      }

      const user = await context.db.createUser(input);
      return { user, userErrors: [] };
    }
  }
};
```

### Relevance to agentquery

agentquery currently has minimal input validation (parse-time operation/field checking). For mutations, we'd need:

1. **Schema-level**: Arg types (string, int, bool) checked during parsing — we already have `ParameterDef.Type`
2. **Required/optional**: `ParameterDef.Optional` already exists
3. **Business logic**: Delegated to the operation handler (same as now)

The key question: how much validation do we bake into the framework vs leave to handlers?

---

## 9. Subscriptions Triggered by Mutations

### The PubSub Pattern

GraphQL subscriptions let clients receive real-time updates when data changes. The mutation → subscription flow:

```graphql
# Schema
type Subscription {
  userCreated: User
  taskUpdated(id: ID!): Task
}

# Subscription (client listens)
subscription {
  taskUpdated(id: "task-1") {
    id
    status
    updatedAt
  }
}
```

### How Mutations Trigger Subscriptions

In the mutation resolver, you publish to a PubSub bus:

```javascript
const resolvers = {
  Mutation: {
    updateTask: async (_, { input }, { db, pubsub }) => {
      const task = await db.updateTask(input);
      pubsub.publish('TASK_UPDATED', { taskUpdated: task });
      return { task, userErrors: [] };
    }
  },
  Subscription: {
    taskUpdated: {
      subscribe: (_, { id }, { pubsub }) =>
        pubsub.asyncIterator(['TASK_UPDATED'])
    }
  }
};
```

### Relevance to agentquery

For a CLI tool, subscriptions don't apply directly. However, the **side effect** concept does:

- After a mutation, the CLI could print confirmation
- The tool could trigger file watchers or hooks
- Batch mutations could produce a summary of all changes

This maps more to **post-mutation hooks** than real-time subscriptions:

```
# Hypothetical agentquery mutation with confirmation output
> create(type=task, name="Fix bug", status=todo)
Created: task-42 (Fix bug, status=todo)
```

---

## 10. Agent-Friendliness Analysis

### What Makes GraphQL Mutations Agent-Friendly

1. **Self-documenting via introspection**: An agent can discover all available mutations, their exact input shapes, required fields, types, and return data — purely from `__schema`. No external docs needed.

2. **Structured input**: Input types provide a clear contract. An agent knows exactly what key-value pairs to send. No guessing.

3. **Typed error responses**: `userErrors` with `field` paths tell the agent exactly what went wrong and where. The agent can auto-correct and retry.

4. **Predictable return data**: Selection sets let the agent request only what it needs from the mutation result, minimizing token overhead.

5. **Batching for efficiency**: Multiple mutations in one request = fewer round trips. Serial ordering guarantees make the result predictable.

### What Makes GraphQL Mutations Agent-Unfriendly

1. **Verbose syntax**: The full GraphQL query string is heavy. Input types, selection sets, variable declarations — all add tokens.

2. **Nested input objects**: Deep nesting (`input: { address: { geo: { lat: ..., lng: ... } } }`) is hard for agents to construct correctly.

3. **Union types for errors**: Agents need to handle `__typename` discrimination, inline fragments. More cognitive/token overhead.

4. **Schema verbosity**: `__schema` introspection response can be enormous. For a CLI tool, you don't need type-level recursion.

5. **Overkill for CLI**: GraphQL was designed for HTTP APIs with diverse clients (web, mobile, other services). A CLI tool has ONE client (the agent). Much of GraphQL's flexibility is wasted.

### Agent + CLI Mutation Discovery Pattern

The ideal pattern for agent-facing CLI tools (from Apollo MCP Server research):

1. **Discover**: `schema()` → lists operations including mutations, with parameter descriptions
2. **Validate**: Schema-level type checking catches obvious errors before execution
3. **Execute**: Flat key-value args, not nested objects: `create(name="Alice", email="a@b.com")`
4. **Confirm**: Structured response with the created/modified object and any errors
5. **Retry**: Error includes which arg was wrong, agent can fix and retry

---

## Key Takeaways for agentquery Design

### What to Adopt from GraphQL

1. **Separate operation keyword/tag for mutations**: GraphQL uses `mutation` vs `query` root. agentquery could tag operations as `read` vs `write` in schema introspection, so agents know which operations have side effects.

2. **Single-input-object pattern**: Translates naturally to agentquery's `operation(key=value, key=value)` syntax — the args ARE the input object, just flat.

3. **Payload with errors**: Return both the result and errors:
   ```json
   { "result": { "id": "task-1", "status": "done" }, "errors": [] }
   ```
   vs
   ```json
   { "result": null, "errors": [{ "field": "status", "message": "invalid value: 'bogus'" }] }
   ```

4. **Schema introspection for mutations**: `schema()` should show mutation operations with their parameters, types, and required/optional status. Already supported via `OperationMetadata`.

5. **Serial batch execution**: Already how agentquery batching works. Perfect fit.

6. **Specific mutation names**: `update-status(task-1, status=done)` is better than `update(task-1, field=status, value=done)`.

### What NOT to Adopt

1. **Nested input types**: Keep args flat (key=value). The DSL's strength is simplicity. Nesting adds parser complexity and token overhead for minimal gain in a CLI context.

2. **Selection sets on mutation results**: Mutations should return a fixed, complete confirmation. The agent doesn't need to project fields on a write response — it's always small.

3. **Union/interface error types**: Overkill for CLI. Simple `{ field, message, code }` error objects are sufficient.

4. **Subscription system**: Not relevant for CLI. Post-mutation hooks or confirmation output is sufficient.

5. **Separate type system for inputs vs outputs**: In agentquery, there's no type graph. Args are strings/ints/bools. Keep it flat.

6. **Variable declarations**: The DSL should accept values inline. No variable system needed.

### Proposed agentquery Mutation Syntax

Based on GraphQL patterns, simplified for CLI:

```
# Create
create-task(name="Fix login bug", status=todo, priority=high)

# Update
update-task(task-1, status=done)

# Delete
delete-task(task-1)

# Specific action
assign-task(task-1, assignee=alice)

# Batch (sequential, isolated errors)
update-task(task-1, status=done); assign-task(task-2, assignee=bob)
```

### Proposed Mutation Response

```json
{
  "ok": true,
  "result": {
    "id": "task-1",
    "status": "done"
  }
}
```

On error:
```json
{
  "ok": false,
  "errors": [
    { "field": "status", "message": "invalid value: 'bogus'", "code": "INVALID_VALUE" }
  ]
}
```

### Schema Introspection Extension

```json
{
  "operations": ["list", "get", "count", "schema", "distinct"],
  "mutations": ["create-task", "update-task", "delete-task", "assign-task"],
  "operationMetadata": {
    "create-task": {
      "kind": "mutation",
      "description": "Create a new task",
      "parameters": [
        { "name": "name", "type": "string", "optional": false },
        { "name": "status", "type": "string", "optional": true, "default": "todo" },
        { "name": "priority", "type": "string", "optional": true, "default": "medium" }
      ],
      "examples": ["create-task(name=\"Fix bug\", status=todo)"]
    }
  }
}
```

### Open Questions for Design Phase

1. **Syntax reuse vs new keyword**: Should mutations use the same `operation(args) { fields }` syntax, or should there be a separate `mutate` command? (CLI equivalent of `q` for queries vs `m` for mutations?)
2. **Confirmation output**: Should mutations always return the modified object? Or just an ack?
3. **Dry-run / preview**: Should there be a `--dry-run` flag that validates without executing?
4. **Idempotency**: Should the framework support idempotency keys?
5. **Handler signature**: Should mutation handlers have a different type signature than read handlers (e.g., `MutationHandler[T]` vs `OperationHandler[T]`)?

---

## Sources

- [GraphQL Mutations — Official Docs](https://graphql.org/learn/mutations/)
- [Mutations and Input Types — graphql-js](https://www.graphql-js.org/docs/mutations-and-input-types/)
- [Designing GraphQL Mutations — Apollo Blog](https://www.apollographql.com/blog/designing-graphql-mutations)
- [Shopify GraphQL Design Tutorial](https://github.com/Shopify/graphql-design-tutorial/blob/master/TUTORIAL.md)
- [Shopify UserError API Reference](https://shopify.dev/docs/api/admin-graphql/latest/objects/UserError)
- [Apollo Schema Naming Conventions](https://www.apollographql.com/docs/graphos/schema-design/guides/naming-conventions)
- [Guide to GraphQL Errors — Production Ready GraphQL](https://productionreadygraphql.com/2020-08-01-guide-to-graphql-errors/)
- [Handling GraphQL Errors with Unions and Interfaces — LogRocket](https://blog.logrocket.com/handling-graphql-errors-like-a-champ-with-unions-and-interfaces/)
- [GraphQL Introspection — Official Docs](https://graphql.org/learn/introspection/)
- [Multiple Mutations — Hasura Docs](https://hasura.io/docs/2.0/mutations/postgres/multiple-mutations/)
- [GraphQL Subscriptions — Official Docs](https://graphql.org/learn/subscriptions/)
- [Building Efficient AI Agents with GraphQL — Apollo Blog](https://www.apollographql.com/blog/building-efficient-ai-agents-with-graphql-and-apollo-mcp-server)
- [MCP and GraphQL: A Match Made for AI Models](https://levelup.gitconnected.com/mcp-and-graphql-a-match-made-for-ai-models-7c10704f8f77)
- [GraphQL Mutations Best Practices — Tailcall](https://tailcall.run/graphql/graphql-mutations/)
- [Sophia Willows — GraphQL Naming Conventions](https://sophiabits.com/blog/graphql-naming-conventions)
- [GraphQL Input Validation & Sanitization — Escape.tech](https://escape.tech/blog/graphql-input-validation-and-sanitization/)
