# tRPC Mutation Patterns: Research for CLI DSL Library

Research date: 2026-02-18
Focus: patterns transferable to a non-HTTP, CLI-DSL context (Go `agentquery` library)

---

## 1. Procedure-Based Mutations

### How It Works in tRPC

tRPC treats every endpoint as a **procedure** -- a typed function with input, output, and a resolver. There are two kinds: `query` (read) and `mutation` (write). The distinction is purely semantic on the server -- the resolver code is identical in structure. The difference manifests at the transport layer (GET vs POST) and on the client (cache invalidation behavior).

```typescript
const t = initTRPC.create();

const appRouter = t.router({
  // Query (read)
  userById: t.procedure
    .input(z.string())
    .query(async ({ input }) => {
      return db.user.findById(input);
    }),

  // Mutation (write) -- same structure, different method
  userCreate: t.procedure
    .input(z.object({ name: z.string() }))
    .mutation(async ({ input }) => {
      return db.user.create(input);
    }),
});
```

Key observations:
- **Procedures are named functions** in a flat or nested namespace (router)
- **Query vs mutation is a type tag**, not a structural difference
- **Nested routers** create dot-separated paths: `user.create`, `post.delete`

### Transferable Pattern for CLI DSL

The core idea: **every operation is a named procedure with typed input and typed output**. In the current `agentquery` library, operations are already named (`get`, `list`, `count`, `schema`). Mutations would be additional operations distinguished by a type tag:

```
# Current (queries only):
list(status=done) { overview }

# With mutations -- same grammar, different operation type:
create(name="Fix bug", status=todo)
update(task-1, status=done)
delete(task-1)
```

The grammar doesn't need to change. The operation handler registration needs a type tag (`query` vs `mutation`) so that:
1. Schema introspection can report which operations are read vs write
2. Middleware can apply different policies (e.g., confirmation prompts for mutations)
3. Agents can discover what's safe to call vs what modifies state

---

## 2. Server-Side Definition (.mutation() API)

### How It Works in tRPC

The builder pattern chains: `procedure.input().mutation()`:

```typescript
const appRouter = t.router({
  // Minimal mutation (no input validation)
  goodbye: t.procedure
    .mutation(async ({ ctx }) => {
      await ctx.signGuestBook();
      return { message: 'goodbye!' };
    }),

  // With input validation
  userCreate: t.procedure
    .input(z.object({ name: z.string() }))
    .mutation(async ({ input }) => {
      const user = await db.user.create(input);
      return user;
    }),

  // Built on a reusable base procedure (with middleware)
  addMember: organizationProcedure
    .input(z.object({ email: z.string().email() }))
    .mutation(({ ctx, input }) => {
      // ctx already has org context from organizationProcedure
      return addMemberToOrg(ctx.org, input.email);
    }),
});
```

Key observations:
- **Handler registration is declarative** -- you declare what a mutation accepts and what it does
- **Base procedures** are reusable building blocks (like protectedProcedure, adminProcedure)
- **The resolver receives a single opts object** with `{ input, ctx }` -- not positional arguments

### Transferable Pattern for CLI DSL

Current `agentquery` already uses handler registration:

```go
schema.Operation("get", func(ctx agentquery.OperationContext[Task]) (any, error) {
    // ...
})
```

For mutations, the registration API could differentiate:

```go
// Option A: separate method
schema.Mutation("create", func(ctx agentquery.MutationContext[Task]) (any, error) { ... })

// Option B: operation with type metadata (tRPC-style -- same method, tagged)
schema.OperationWithMetadata("create", agentquery.OperationMetadata{
    Type: agentquery.MutationType,  // vs agentquery.QueryType
    // ...
}, handler)
```

Option B is closer to tRPC's philosophy where query/mutation is just a tag.

---

## 3. Input Validation (Zod Integration)

### How It Works in tRPC

tRPC delegates validation to external libraries (Zod is the canonical choice). The `.input()` method accepts any validator conforming to the Standard Schema interface:

```typescript
// Zod schema as validator
t.procedure
  .input(z.object({
    name: z.string().min(1),
    email: z.string().email(),
    role: z.enum(['admin', 'member']).default('member'),
  }))
  .mutation(({ input }) => {
    // input is fully typed: { name: string, email: string, role: 'admin' | 'member' }
  })
```

**Input stacking** -- multiple `.input()` calls merge:

```typescript
const baseProcedure = t.procedure
  .input(z.object({ orgId: z.string() }))
  .use((opts) => {
    // middleware can access opts.input.orgId
    return opts.next();
  });

// Final input is { orgId: string } & { name: string }
baseProcedure
  .input(z.object({ name: z.string() }))
  .mutation(({ input }) => {
    // input.orgId + input.name both available
  })
```

**Function-based validators** (no library needed):

```typescript
t.procedure
  .input((value): string => {
    if (typeof value === 'string') return value;
    throw new Error('Input is not a string');
  })
```

**Output validation**:

```typescript
t.procedure
  .output(z.object({ id: z.string(), name: z.string() }))
  .mutation(({ input }) => {
    // Return value is validated against output schema
    return { id: '1', name: 'test' };
  })
```

Key observations:
- **Schema-driven validation** -- the schema IS the documentation
- **Validation happens before the resolver** -- invalid input never reaches business logic
- **Input stacking** allows base procedures to add mandatory fields
- **Output validation** is optional but ensures contract compliance

### Transferable Pattern for CLI DSL

Current `agentquery` validates at parse time (operation name, field names) but mutation inputs need richer validation. Possible approaches:

```go
// Validator as a function (like tRPC's function-based validator)
schema.Mutation("create", agentquery.MutationDef[Task]{
    Validate: func(args map[string]string) error {
        if args["name"] == "" {
            return fmt.Errorf("name is required")
        }
        return nil
    },
    Handler: func(ctx MutationContext[Task]) (any, error) { ... },
})

// Or: declare expected params in metadata (schema introspection can report them)
schema.MutationWithMetadata("create", agentquery.OperationMetadata{
    Parameters: []agentquery.ParameterDef{
        {Name: "name", Type: "string", Required: true, Description: "Task name"},
        {Name: "status", Type: "string", Required: false, Default: "todo"},
    },
}, handler)
```

The ParameterDef approach already exists in `agentquery` for schema introspection. Extending it with `Required` and `Default` fields gives validation metadata that can be enforced automatically.

---

## 4. Error Handling

### How It Works in tRPC

tRPC uses a dedicated `TRPCError` class with typed error codes:

```typescript
import { TRPCError } from '@trpc/server';

// Throwing in a procedure
t.procedure.mutation(({ input }) => {
  const task = db.task.findById(input.id);
  if (!task) {
    throw new TRPCError({
      code: 'NOT_FOUND',
      message: `Task ${input.id} not found`,
    });
  }
  if (!canEdit(task)) {
    throw new TRPCError({
      code: 'FORBIDDEN',
      message: 'You do not have permission to edit this task',
    });
  }
  return db.task.update(input);
});
```

**Error codes** (subset most relevant to CLI):

| Code | HTTP | CLI Relevance |
|------|------|---------------|
| BAD_REQUEST | 400 | Invalid arguments |
| UNAUTHORIZED | 401 | Auth required |
| FORBIDDEN | 403 | Permission denied |
| NOT_FOUND | 404 | Item doesn't exist |
| CONFLICT | 409 | Concurrent modification |
| UNPROCESSABLE_CONTENT | 422 | Valid syntax, invalid semantics |
| INTERNAL_SERVER_ERROR | 500 | Bug / unexpected error |

**Error response structure** (JSON-RPC 2.0 style):

```json
{
  "error": {
    "message": "Task task-99 not found",
    "code": -32600,
    "data": {
      "code": "NOT_FOUND",
      "httpStatus": 404,
      "path": "task.update",
      "stack": "..."
    }
  }
}
```

**Error formatting** -- customizable via `errorFormatter`:

```typescript
const t = initTRPC.create({
  errorFormatter({ shape, error }) {
    return {
      ...shape,
      data: {
        ...shape.data,
        // Include Zod validation details when input fails
        zodError:
          error.code === 'BAD_REQUEST' && error.cause instanceof ZodError
            ? error.cause.flatten()
            : null,
      },
    };
  },
});
```

Key observations:
- **Typed error codes** -- not arbitrary strings, a fixed enum
- **Structured errors** -- always have code + message + optional metadata
- **Validation errors carry details** -- which field failed and why
- **Error formatter is a hook** -- customizable per-server

### Transferable Pattern for CLI DSL

Current `agentquery` has `Error` with code/message/details. For mutations, extend this:

```go
// Current
type Error struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
}

// Extended for mutations
const (
    ErrBadRequest          = "BAD_REQUEST"
    ErrNotFound            = "NOT_FOUND"
    ErrConflict            = "CONFLICT"
    ErrForbidden           = "FORBIDDEN"
    ErrValidationFailed    = "VALIDATION_FAILED"
    ErrInternalServerError = "INTERNAL_SERVER_ERROR"
)

// Validation error detail
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
    Value   any    `json:"value,omitempty"`
}
```

The important tRPC pattern: **validation errors are structured, not just strings**. When a mutation fails validation, the agent gets `{"field": "name", "message": "required"}` -- not a blob of text.

---

## 5. Batching

### How It Works in tRPC

tRPC batches multiple procedure calls into a single HTTP request using `httpBatchLink`:

**Client side** -- automatic via Promise.all:

```typescript
// These three calls are batched into ONE HTTP request
const [post1, post2, post3] = await Promise.all([
  trpc.post.byId.query(1),
  trpc.post.byId.query(2),
  trpc.post.byId.query(3),
]);
```

**Wire format**:
- URL: `/api/trpc/postById,postById,postById?batch=1&input={"0":1,"1":2,"2":3}`
- Procedure names are comma-separated in the path
- Inputs are numbered in a JSON object
- Response: array of individual results, status 207 (Multi-Status) when mixed

**Batch response**:

```json
[
  { "result": { "data": { "id": 1, "title": "First" } } },
  { "result": { "data": { "id": 2, "title": "Second" } } },
  { "error": { "message": "Not found", "code": -32600 } }
]
```

**Configuration**:

```typescript
httpBatchLink({
  url: 'http://localhost:3000',
  maxURLLength: 2083,  // split into multiple requests if URL too long
  maxItems: 10,        // max operations per batch
})
```

**Server-side batch limiting** (via context):

```typescript
export async function createContext(opts) {
  if (opts.info.calls.length > 10) {
    throw new TRPCError({
      code: 'TOO_MANY_REQUESTS',
      message: 'Batch size limit of 10 exceeded',
    });
  }
}
```

Key observations:
- **Batching is transparent** -- client code looks like individual calls
- **Individual error isolation** -- one failure doesn't kill the batch (same as current `agentquery`)
- **Response is always an ordered array** matching input order
- **Mixed queries and mutations can batch** (same HTTP method required though)

### Transferable Pattern for CLI DSL

Current `agentquery` already supports batching with `;`:

```
list(status=done) { overview }; count(); get(task-1) { minimal }
```

This maps directly to tRPC's model. The same patterns apply for mutations:

```
create(name="Fix bug"); update(task-2, status=done); delete(task-3)
```

The error isolation pattern is already implemented -- batch errors are inlined per-statement.

One new consideration: **mixed read/write batches**. Should `list(); create(name="X"); count()` be allowed? tRPC separates by HTTP method (GET batches and POST batches). For CLI, a possible rule: **mutations execute sequentially within the batch, queries can be parallel** (or just execute everything sequentially, which is simpler and what CLI users expect).

---

## 6. Discovery/Introspection

### How It Works in tRPC

tRPC intentionally has **no built-in runtime introspection**. Discovery is handled three ways:

1. **Type-level inference** (compile time only):
```typescript
type AppRouter = typeof appRouter;
type Inputs = inferRouterInputs<AppRouter>;
type Outputs = inferRouterOutputs<AppRouter>;
// Inputs['userCreate'] = { name: string }
```

2. **trpc-openapi** (third-party, generates OpenAPI spec):
```typescript
import { generateOpenApiDocument } from 'trpc-openapi';

const openApiDocument = generateOpenApiDocument(appRouter, {
  title: 'My API',
  version: '1.0.0',
  baseUrl: 'http://localhost:3000/api',
});
```

3. **Internal `_def` property** (unstable, undocumented):
```typescript
// Walk the router tree to discover procedures
appRouter._def.procedures // map of procedure definitions
appRouter._def.record     // nested router structure
```

4. **trpc-cli** (turns router into CLI with auto-generated help):
- Walks router definition at startup
- Converts Zod schemas to CLI argument definitions
- Generates `--help` output from `.describe()` annotations
- Maps nested routers to subcommands

**Procedure metadata** via `.meta()`:

```typescript
const t = initTRPC.meta<{
  summary?: string;
  description?: string;
  tags?: string[];
}>().create();

t.procedure
  .meta({ summary: 'Create a user', tags: ['users'] })
  .input(z.object({ name: z.string() }))
  .mutation(({ input }) => { ... });
```

Key observations:
- **tRPC favors static type safety over runtime introspection**
- **Community tools fill the gap** (trpc-openapi, trpc-cli)
- **Metadata is opt-in** and schema-typed
- **Zod schemas are introspectable** -- `zod-to-json-schema` converts them

### Transferable Pattern for CLI DSL

Current `agentquery` already has a built-in `schema()` operation that reports operations, fields, presets, and metadata. This is **better than tRPC's approach** for CLI/agent use. The extension for mutations:

```json
{
  "operations": {
    "list": { "type": "query", "parameters": [...] },
    "get": { "type": "query", "parameters": [...] },
    "create": {
      "type": "mutation",
      "parameters": [
        { "name": "name", "type": "string", "required": true },
        { "name": "status", "type": "string", "required": false, "default": "todo" }
      ],
      "examples": ["create(name=\"Fix bug\")", "create(name=\"New feature\", status=in-progress)"]
    },
    "delete": {
      "type": "mutation",
      "parameters": [
        { "name": "id", "type": "string", "required": true }
      ],
      "confirmationRequired": true
    }
  }
}
```

The `schema()` output becomes the agent's "contract" -- it tells the agent what mutations are available, what they accept, and which ones are destructive.

---

## 7. Middleware

### How It Works in tRPC

Middleware wraps procedure execution using `.use()`. The middleware calls `opts.next()` to continue the chain:

**Auth middleware**:

```typescript
const isAuthed = t.middleware(async ({ ctx, next }) => {
  if (!ctx.user) {
    throw new TRPCError({ code: 'UNAUTHORIZED' });
  }
  return next({
    ctx: {
      user: ctx.user,  // narrows type from User | null to User
    },
  });
});

const protectedProcedure = t.procedure.use(isAuthed);
```

**Logging middleware**:

```typescript
const loggedProcedure = t.procedure.use(async (opts) => {
  const start = Date.now();
  const result = await opts.next();
  const durationMs = Date.now() - start;
  const meta = { path: opts.path, type: opts.type, durationMs };
  result.ok
    ? console.log('OK request timing:', meta)
    : console.error('Non-OK request timing', meta);
  return result;
});
```

**Input-aware middleware** (experimental standalone):

```typescript
const projectAccessMiddleware = experimental_standaloneMiddleware<{
  ctx: { allowedProjects: string[] };
  input: { projectId: string };
}>().create((opts) => {
  if (!opts.ctx.allowedProjects.includes(opts.input.projectId)) {
    throw new TRPCError({ code: 'FORBIDDEN' });
  }
  return opts.next();
});
```

**Middleware piping** (composable chain):

```typescript
const fooMiddleware = t.middleware((opts) =>
  opts.next({ ctx: { foo: 'foo' } })
);

const barMiddleware = fooMiddleware.unstable_pipe((opts) =>
  opts.next({ ctx: { bar: 'bar' } })
);

// barProcedure gets both { foo, bar } in ctx
const barProcedure = t.procedure.use(barMiddleware);
```

Key observations:
- **Middleware is "around" advice** -- wraps the handler, can run before AND after
- **Context refinement** -- middleware narrows types (nullable -> non-null)
- **Composable** -- multiple middleware stack/pipe
- **Has access to path, type, input** -- can make decisions based on what's being called
- **Next returns a result** -- middleware can inspect success/failure after handler runs

### Transferable Pattern for CLI DSL

For CLI mutations, middleware maps to hooks/interceptors:

```go
// Before hook -- validation, confirmation, auth
type BeforeHook[T any] func(ctx *MutationContext[T]) error

// After hook -- logging, notification, cache invalidation
type AfterHook[T any] func(ctx *MutationContext[T], result any, err error)

// Registration
schema.MutationMiddleware("create", BeforeHook[Task](func(ctx *MutationContext[Task]) error {
    // Confirmation prompt for destructive operations
    if ctx.Operation == "delete" {
        return confirmWithUser(ctx)
    }
    return nil
}))
```

Or a more tRPC-like wrapping approach:

```go
// Middleware wraps the handler -- has both before and after
type Middleware[T any] func(ctx *OperationContext[T], next func() (any, error)) (any, error)

// Logging middleware
func LoggingMiddleware[T any](ctx *OperationContext[T], next func() (any, error)) (any, error) {
    start := time.Now()
    result, err := next()
    log.Printf("%s %s took %v", ctx.Type, ctx.Operation, time.Since(start))
    return result, err
}
```

The tRPC "around" pattern (before + next + after) is more powerful than separate before/after hooks because it naturally handles error cases and timing.

---

## 8. Context

### How It Works in tRPC

Context is created per-request and flows through all middleware and the final resolver:

**Context creation**:

```typescript
export const createContext = async (opts: CreateHTTPContextOptions) => {
  const session = await getSession(opts.req);
  return {
    session,
    db: prisma,
  };
};

type Context = Awaited<ReturnType<typeof createContext>>;
const t = initTRPC.context<Context>().create();
```

**Context in procedures**:

```typescript
t.procedure.mutation(({ ctx, input }) => {
  // ctx.session -- from createContext
  // ctx.db -- from createContext
  return ctx.db.user.create({ data: input });
});
```

**Inner/outer context split** (request-independent vs request-dependent):

```typescript
// Inner: always available (db, config)
export async function createContextInner(opts) {
  return { prisma, session: opts.session };
}

// Outer: request-specific (headers, cookies)
export async function createContext(opts) {
  const session = getSessionFromCookie(opts.req);
  return { ...await createContextInner({ session }), req: opts.req };
}
```

**Batch-aware context** (v11 feature):

```typescript
export async function createContext(opts) {
  // opts.info.calls tells you how many procedures are in this batch
  if (opts.info.calls.length > 10) {
    throw new TRPCError({ code: 'TOO_MANY_REQUESTS' });
  }
  return {};
}
```

Key observations:
- **Context is the dependency injection mechanism** -- replaces constructor injection
- **Created once per request/batch** -- not per procedure
- **Middleware can extend context** with new fields (and narrow types)
- **Two-layer pattern** -- inner (global services) + outer (request-specific)

### Transferable Pattern for CLI DSL

For CLI, context would be created once per CLI invocation and passed to all operations in a batch:

```go
// Context created by the CLI tool, passed to schema
type CLIContext struct {
    // Inner (always available)
    DataDir  string
    Config   *Config

    // Outer (per-invocation)
    DryRun   bool
    Verbose  bool
    User     string
}

// Schema receives context factory
schema := agentquery.NewSchema[Task](agentquery.SchemaOptions[Task]{
    ContextFactory: func() (*CLIContext, error) {
        return &CLIContext{
            DataDir: os.Getenv("DATA_DIR"),
            DryRun:  flags.DryRun,
        }, nil
    },
})
```

In the current `agentquery` design, `OperationContext` already carries the parsed statement and items loader. Adding a user-provided context would follow tRPC's pattern -- a generic `Ctx` field on the context:

```go
type OperationContext[T any] struct {
    Statement Statement
    Selector  *FieldSelector[T]
    Items     func() ([]T, error)
    Predicate func(T) bool
    Ctx       any   // user-provided context, like tRPC's ctx
}
```

---

## Summary: Key Patterns Transferable to CLI DSL

| tRPC Pattern | Current agentquery | Mutation Extension |
|---|---|---|
| Procedure = named typed function | Operation handlers | Add mutation type tag |
| .input() validation | Parse-time field/op validation | Add parameter validation with Required/Default |
| TRPCError with codes | Error with code/message/details | Add standard error codes enum |
| Batch in single request | `;` batch syntax | Same -- works for mutations too |
| No runtime introspection (by design) | Built-in schema() | Extend schema() with mutation metadata |
| Middleware wraps handler | None | Add around-advice middleware |
| Context per request | OperationContext | Add user-provided Ctx field |
| Query/mutation type tag | All operations are queries | Add Type field to OperationMetadata |

### Critical Design Decision for CLI DSL

tRPC's biggest insight for our use case: **mutations and queries are structurally identical procedures, differentiated only by a semantic type tag**. This means:

1. **No grammar changes needed** -- `create(name="X")` uses the same parser as `list(status=done)`
2. **Same batching mechanism** -- `list(); create(name="X"); count()` just works
3. **Schema introspection reports both** -- agents discover mutations alongside queries
4. **Middleware applies uniformly** -- same hook system for reads and writes
5. **The type tag enables policy** -- "confirm before mutations", "dry-run for mutations", "log all mutations"

---

## Bonus: trpc-cli (Direct Prior Art)

The `trpc-cli` library by @mmkal is the closest existing project to what `agentquery` is doing. Key implementation details:

- **Router -> CLI mapping**: nested routers become subcommands, procedures become commands
- **Zod -> CLI args**: z.object fields become `--named-flags`, z.string/z.number become positional args
- **z.describe()** -> `--help` text
- **z.default()** -> default argument values
- **z.enum()** -> restricted choices
- **Validation runs before handler** -- same as tRPC server
- **Output**: JSON to stdout by default, non-zero exit for errors
- **Single dependency**: commander.js for arg parsing

This validates that the tRPC procedure model maps cleanly to CLI tools.

---

## Sources

- [Define Procedures | tRPC](https://trpc.io/docs/server/procedures)
- [Input & Output Validators | tRPC](https://trpc.io/docs/server/validators)
- [Error Handling | tRPC](https://trpc.io/docs/server/error-handling)
- [Error Formatting | tRPC](https://trpc.io/docs/server/error-formatting)
- [HTTP Batch Link | tRPC](https://trpc.io/docs/client/links/httpBatchLink)
- [HTTP RPC Specification | tRPC](https://trpc.io/docs/rpc)
- [Middlewares | tRPC](https://trpc.io/docs/server/middlewares)
- [Context | tRPC](https://trpc.io/docs/server/context)
- [Quickstart | tRPC](https://trpc.io/docs/quickstart)
- [trpc-cli | GitHub](https://github.com/mmkal/trpc-cli)
- [openapi-trpc | GitHub](https://github.com/dtinth/openapi-trpc)
- [How To Generate an OpenAPI Spec With tRPC | Speakeasy](https://www.speakeasy.com/openapi/frameworks/trpc)
