# taskdemo — agentquery example

A minimal task tracker CLI that demonstrates the [agentquery](../agentquery/) library. It shows how to define a domain type, register fields and presets, implement operations, and wire everything into a Cobra CLI with structured queries and full-text search.

## Build

```bash
cd example/
go build -o taskdemo .
```

## Usage

The binary exposes two subcommands added by `cobraext.AddCommands`:

- **`q <query>`** — execute a structured DSL query
- **`grep <pattern>`** — full-text regex search across data files

### Query examples

**Get a summary of all tasks:**

```bash
./taskdemo q 'summary()'
```

```json
{"counts":{"done":3,"in-progress":3,"todo":2},"total":8}
```

**Get a single task with the `overview` preset (id, name, status, assignee):**

```bash
./taskdemo q 'get(task-1) { overview }'
```

```json
{"assignee":"alice","id":"task-1","name":"Auth service refactor","status":"in-progress"}
```

**List tasks filtered by status, with the `minimal` preset (id, status):**

```bash
./taskdemo q 'list(status=done) { minimal }'
```

```json
[{"id":"task-3","status":"done"},{"id":"task-4","status":"done"},{"id":"task-7","status":"done"}]
```

**Batch query — multiple statements separated by `;`:**

```bash
./taskdemo q 'get(task-1) { status }; get(task-2) { status }'
```

```json
[{"status":"in-progress"},{"status":"todo"}]
```

**List by assignee with full details:**

```bash
./taskdemo q 'list(assignee=alice) { full }'
```

```json
[
  {"assignee":"alice","description":"Refactor auth to use JWT tokens","id":"task-1","name":"Auth service refactor","status":"in-progress"},
  {"assignee":"alice","description":"Users get stuck on /callback after OAuth","id":"task-3","name":"Fix login redirect bug","status":"done"}
]
```

**List all tasks (default projection = id, name, status):**

```bash
./taskdemo q 'list()'
```

```json
[
  {"id":"task-1","name":"Auth service refactor","status":"in-progress"},
  {"id":"task-2","name":"Dashboard performance","status":"todo"},
  {"id":"task-3","name":"Fix login redirect bug","status":"done"},
  {"id":"task-4","name":"Add dark mode","status":"done"},
  {"id":"task-5","name":"Pagination API","status":"in-progress"},
  {"id":"task-6","name":"CI pipeline speedup","status":"todo"},
  {"id":"task-7","name":"Write onboarding docs","status":"done"},
  {"id":"task-8","name":"Rate limiter middleware","status":"in-progress"}
]
```

### Search examples

**Find all TODO comments in data files:**

```bash
./taskdemo grep "TODO"
```

```json
[
  {"source":{"path":"task-1.md","line":14},"content":"TODO: Benchmark token validation latency under load.","isMatch":true},
  {"source":{"path":"task-1.md","line":15},"content":"TODO: Decide on token expiry duration (currently thinking 15min access / 7d refresh).","isMatch":true},
  {"source":{"path":"task-2.md","line":14},"content":"TODO: Profile the API response payload sizes — some endpoints return way too much data.","isMatch":true},
  {"source":{"path":"task-5.md","line":13},"content":"TODO: Migrate existing offset-based consumers to cursor-based.","isMatch":true}
]
```

**Case-insensitive search:**

```bash
./taskdemo grep "pagination" -i
```

```json
[
  {"source":{"path":"task-2.md","line":15},"content":"Blocked by: backend pagination API (task-5).","isMatch":true},
  {"source":{"path":"task-5.md","line":1},"content":"# Pagination API","isMatch":true},
  {"source":{"path":"task-5.md","line":3},"content":"Add cursor-based pagination to all list endpoints.","isMatch":true},
  {"source":{"path":"task-5.md","line":7},"content":"- Implement cursor-based pagination (not offset-based)","isMatch":true}
]
```

## How it works

### 1. Define a domain type

```go
type Task struct {
    ID          string
    Name        string
    Status      string
    Assignee    string
    Description string
}
```

### 2. Create a schema and register fields

```go
schema := agentquery.NewSchema[Task](
    agentquery.WithDataDir("data"),
    agentquery.WithExtensions(".md"),
)

schema.Field("id", func(t Task) any { return t.ID })
schema.Field("name", func(t Task) any { return t.Name })
schema.Field("status", func(t Task) any { return t.Status })
schema.Field("assignee", func(t Task) any { return t.Assignee })
schema.Field("description", func(t Task) any { return t.Description })
```

Each `Field` call registers a named accessor that extracts a value from the domain type. Fields are used in query projections — only requested fields are evaluated.

### 3. Register presets

```go
schema.Preset("minimal", "id", "status")
schema.Preset("default", "id", "name", "status")
schema.Preset("overview", "id", "name", "status", "assignee")
schema.Preset("full", "id", "name", "status", "assignee", "description")

schema.DefaultFields("default")
```

Presets are named bundles of fields. When a query says `{ overview }`, it expands to `{ id name status assignee }`. `DefaultFields` sets what's returned when no projection is specified.

### 4. Set a data loader

```go
schema.SetLoader(func() ([]Task, error) {
    return sampleTasks(), nil
})
```

The loader is called lazily — only when an operation accesses `ctx.Items()`. In a real application this would query a database, read files, or call an API.

### 5. Register operations

```go
schema.Operation("get", func(ctx agentquery.OperationContext[Task]) (any, error) {
    targetID := ctx.Statement.Args[0].Value
    items, _ := ctx.Items()
    for _, task := range items {
        if task.ID == targetID {
            return ctx.Selector.Apply(task), nil
        }
    }
    return nil, &agentquery.Error{Code: agentquery.ErrNotFound, Message: "not found"}
})
```

Operations receive an `OperationContext` with:
- `Statement` — parsed operation name, arguments, and field list
- `Selector` — field selector with `Apply(item)` to project fields
- `Items` — lazy loader function `func() ([]T, error)`

### 6. Wire Cobra commands

```go
root := &cobra.Command{Use: "taskdemo"}
cobraext.AddCommands(root, schema)
root.Execute()
```

`AddCommands` adds two subcommands: `q` (query) and `grep` (search). That's all the CLI wiring needed.

## Further reading

See the library's full specification and agent integration patterns in [SKILL.md](../SKILL.md).
