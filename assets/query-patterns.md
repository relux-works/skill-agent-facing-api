# Query Patterns Catalog

Reference of DSL query patterns with example inputs and expected JSON outputs.

---

## 1. Single Element Lookup

**Minimal fields (cheapest):**
```
get(ITEM-42) { minimal }
```
```json
{"id": "ITEM-42", "status": "active"}
```

**Default fields (when you need the name too):**
```
get(ITEM-42)
```
```json
{"id": "ITEM-42", "name": "fix-auth-bug", "status": "active"}
```

**Specific fields:**
```
get(ITEM-42) { id status assignee }
```
```json
{"id": "ITEM-42", "status": "active", "assignee": "agent-auth"}
```

**Full details:**
```
get(ITEM-42) { full }
```
```json
{
  "id": "ITEM-42",
  "name": "fix-auth-bug",
  "status": "active",
  "assignee": "agent-auth",
  "description": "JWT tokens expire silently...",
  "parent": "GROUP-05",
  "created": "2026-02-10T14:30:00Z",
  "updated": "2026-02-11T09:15:00Z",
  "tags": ["auth", "urgent"],
  "blockedBy": [],
  "blocks": ["ITEM-43"]
}
```

---

## 2. Filtered Lists

**By type:**
```
list(type=task) { overview }
```
```json
{
  "elements": [
    {"id": "ITEM-42", "name": "fix-auth-bug", "status": "active", "assignee": "agent-auth", "parent": "GROUP-05"},
    {"id": "ITEM-43", "name": "add-refresh-token", "status": "pending", "assignee": "", "parent": "GROUP-05"}
  ],
  "count": 2
}
```

**By status:**
```
list(type=task, status=active) { id name }
```
```json
{
  "elements": [
    {"id": "ITEM-42", "name": "fix-auth-bug"}
  ],
  "count": 1
}
```

**By parent:**
```
list(parent=GROUP-05, type=task) { minimal }
```
```json
{
  "elements": [
    {"id": "ITEM-42", "status": "active"},
    {"id": "ITEM-43", "status": "pending"}
  ],
  "count": 2
}
```

**Blocked items:**
```
list(blocked=true) { id name status }
```

---

## 3. Pagination

Pagination uses `skip` and `take` keyword params on list operations. Applied after filtering, before field projection.

**First page:**
```
list(take=5) { overview }
```
```json
[
  {"id": "ITEM-01", "name": "first-task", "status": "active", "assignee": "alice"},
  {"id": "ITEM-02", "name": "second-task", "status": "pending", "assignee": "bob"},
  {"id": "ITEM-03", "name": "third-task", "status": "done", "assignee": "alice"},
  {"id": "ITEM-04", "name": "fourth-task", "status": "active", "assignee": "carol"},
  {"id": "ITEM-05", "name": "fifth-task", "status": "pending", "assignee": "bob"}
]
```

**Next page (skip first 5, take next 5):**
```
list(skip=5, take=5) { overview }
```

**Pagination with filters:**
```
list(status=active, skip=0, take=3) { id name assignee }
```
```json
[
  {"id": "ITEM-01", "name": "first-task", "assignee": "alice"},
  {"id": "ITEM-04", "name": "fourth-task", "assignee": "carol"},
  {"id": "ITEM-09", "name": "ninth-task", "assignee": "dave"}
]
```

**Skip past all items (returns empty):**
```
list(skip=1000) { minimal }
```
```json
[]
```

---

## 4. Count

Count returns `{"count": N}` — the cheapest way to know how many items match. No field projection needed.

**Count all:**
```
count()
```
```json
{"count": 48}
```

**Count with filter:**
```
count(status=done)
```
```json
{"count": 31}
```

**Count with multiple filters:**
```
count(status=active, assignee=alice)
```
```json
{"count": 3}
```

**Batch: count + paginated list (know total before paging):**
```
count(status=active); list(status=active, take=5) { overview }
```
```json
[
  {"count": 12},
  [
    {"id": "ITEM-01", "name": "first-task", "status": "active", "assignee": "alice"},
    {"id": "ITEM-04", "name": "fourth-task", "status": "active", "assignee": "carol"}
  ]
]
```

---

## 5. Summary / Overview

```
summary()
```
```json
{
  "summary": {
    "byType": {
      "group": {"total": 5, "active": 2, "done": 3, "todo": 0, "closed": 0},
      "task":  {"total": 48, "active": 10, "done": 31, "todo": 5, "closed": 2}
    },
    "active": [
      {"id": "ITEM-42", "name": "fix-auth-bug", "status": "active"},
      {"id": "ITEM-50", "name": "api-rate-limiting", "status": "active"}
    ],
    "blocked": [
      {"id": "ITEM-43", "name": "add-refresh-token", "blockedBy": ["ITEM-42"]}
    ]
  }
}
```

---

## 6. Batch Queries

**Multiple lookups in one call:**
```
get(ITEM-42) { status }; get(ITEM-43) { status }; get(ITEM-50) { status }
```
```json
[
  {"id": "ITEM-42", "status": "active"},
  {"id": "ITEM-43", "status": "pending"},
  {"id": "ITEM-50", "status": "active"}
]
```

**Mixed operations:**
```
summary(); list(type=task, status=active) { id name assignee }
```
```json
[
  {"summary": {"byType": {...}, "active": [...], "blocked": [...]}},
  {"elements": [...], "count": 3}
]
```

**Error handling in batch (per-statement errors, rest continues):**
```
get(ITEM-42) { status }; get(NONEXISTENT) { status }; get(ITEM-50) { status }
```
```json
[
  {"id": "ITEM-42", "status": "active"},
  {"error": {"message": "element NONEXISTENT not found"}},
  {"id": "ITEM-50", "status": "active"}
]
```

---

## 7. Grep Patterns

**Simple text search:**
```bash
mytool grep "authentication"
```
```
config/auth.md:5:## Authentication Flow
tasks/ITEM-42/README.md:3:Fix authentication token expiry
```

**Scoped to file type:**
```bash
mytool grep "blocked" --file progress.md
```
```
tasks/ITEM-43/progress.md:8:blocked
tasks/ITEM-43/progress.md:14:- ITEM-42
```

**Case-insensitive:**
```bash
mytool grep "todo" -i --file README.md
```

**With context (2 lines before/after):**
```bash
mytool grep "error" -C 2 --file progress.md
```

---

## 8. Schema Introspection

The built-in `schema()` operation returns the full API contract. When operations are registered with metadata, `operationMetadata` is included with parameter definitions and examples:

```
schema()
```
```json
{
  "operations": ["count", "get", "list", "schema", "summary"],
  "fields": ["id", "name", "status", "assignee", "description"],
  "presets": {
    "minimal": ["id", "status"],
    "default": ["id", "name", "status"],
    "overview": ["id", "name", "status", "assignee"],
    "full": ["id", "name", "status", "assignee", "description"]
  },
  "defaultFields": ["default"],
  "operationMetadata": {
    "get": {
      "description": "Find a single item by ID",
      "parameters": [
        {"name": "id", "type": "string", "optional": false, "description": "Item ID (positional)"}
      ],
      "examples": ["get(ITEM-42) { overview }"]
    },
    "list": {
      "description": "List items with optional filters and pagination",
      "parameters": [
        {"name": "status", "type": "string", "optional": true, "description": "Filter by status"},
        {"name": "skip", "type": "int", "optional": true, "default": 0, "description": "Skip first N items"},
        {"name": "take", "type": "int", "optional": true, "description": "Return at most N items"}
      ],
      "examples": ["list() { overview }", "list(status=done, skip=0, take=5) { minimal }"]
    },
    "count": {
      "description": "Count items matching optional filters",
      "parameters": [
        {"name": "status", "type": "string", "optional": true, "description": "Filter by status"}
      ],
      "examples": ["count()", "count(status=done)"]
    },
    "summary": {
      "description": "Return counts grouped by status",
      "examples": ["summary()"]
    }
  }
}
```

Agents call `schema()` once at the start of a session to discover available operations, fields, and usage patterns — no external documentation needed.

---

## 9. Mutations (Write Operations)

Mutations use the same DSL grammar as queries. Registered via `Mutation()`/`MutationWithMetadata()` and accessed through the `m` subcommand with safety flags.

### Create

**Basic create:**
```
create(title="Fix login bug", status=todo)
```
```json
{"ok": true, "result": {"id": "ITEM-09", "title": "Fix login bug", "status": "todo", "priority": "medium"}}
```

**Create with all fields:**
```
create(title="New feature", status=in-progress, assignee=alice, priority=high)
```
```json
{"ok": true, "result": {"id": "ITEM-10", "title": "New feature", "status": "in-progress", "assignee": "alice", "priority": "high"}}
```

**Create with dry run (preview):**
```
create(title="Test task", dry_run=true)
```
```json
{"ok": true, "result": {"dry_run": true, "would_create": {"title": "Test task", "status": "todo", "priority": "medium"}}}
```

### Update

**Update a single field:**
```
update(ITEM-42, status=done)
```
```json
{"ok": true, "result": {"id": "ITEM-42", "title": "fix-auth-bug", "status": "done", "assignee": "alice", "priority": "high"}}
```

**Update multiple fields:**
```
update(ITEM-42, title="New title", assignee=bob)
```
```json
{"ok": true, "result": {"id": "ITEM-42", "title": "New title", "status": "active", "assignee": "bob", "priority": "high"}}
```

**Update with dry run:**
```
update(ITEM-42, status=done, dry_run=true)
```
```json
{"ok": true, "result": {"dry_run": true, "id": "ITEM-42", "would_update": {"status": "done"}}}
```

### Delete

**Delete (destructive — requires `--confirm` on CLI):**
```
delete(ITEM-42)
```
```json
{"ok": true, "result": {"deleted": true, "id": "ITEM-42", "title": "fix-auth-bug"}}
```

**Delete with dry run:**
```
delete(ITEM-42, dry_run=true)
```
```json
{"ok": true, "result": {"dry_run": true, "would_delete": {"id": "ITEM-42", "title": "fix-auth-bug", "status": "active"}}}
```

### Validation Errors

**Missing required parameter:**
```
create()
```
```json
{"ok": false, "errors": [{"field": "title", "message": "required parameter \"title\" is missing", "code": "REQUIRED"}]}
```

**Invalid enum value:**
```
create(title="Test", status=invalid)
```
```json
{"ok": false, "errors": [{"field": "status", "message": "invalid value \"invalid\" for status, must be one of: todo, in-progress, done", "code": "INVALID_VALUE"}]}
```

**Not found:**
```
update(NONEXISTENT, status=done)
```
```json
{"ok": false, "errors": [{"message": "task \"NONEXISTENT\" not found"}]}
```

### Schema Introspection with Mutations

When mutations are registered, `schema()` includes separate `mutations` and `mutationMetadata` sections:

```
schema()
```
```json
{
  "operations": ["count", "get", "list", "schema", "summary"],
  "mutations": ["create", "delete", "update"],
  "mutationMetadata": {
    "create": {
      "description": "Create a new task",
      "parameters": [
        {"name": "title", "type": "string", "required": true, "description": "Task title"},
        {"name": "status", "type": "string", "enum": ["todo", "in-progress", "done"], "default": "todo"}
      ],
      "destructive": false,
      "idempotent": false,
      "examples": ["create(title=\"Fix login bug\")"]
    },
    "delete": {
      "description": "Delete a task by ID",
      "parameters": [
        {"name": "id", "type": "string", "required": true, "description": "Task ID (positional)"}
      ],
      "destructive": true,
      "idempotent": true,
      "examples": ["delete(ITEM-42)"]
    }
  }
}
```

---

## 10. Anti-Patterns

### Bad: Multiple calls for data available in one query

```bash
# 3 tool calls, 3x framing overhead (~240 tokens wasted)
mytool show ITEM-42
mytool show ITEM-43
mytool show ITEM-50
```

### Good: Single batch call

```bash
# 1 tool call
mytool q 'get(ITEM-42) { status }; get(ITEM-43) { status }; get(ITEM-50) { status }'
```

### Bad: Fetching all fields when you need one

```bash
# Returns ~300 tokens per element
mytool q 'get(ITEM-42) { full }'
```

### Good: Field projection

```bash
# Returns ~20 tokens
mytool q 'get(ITEM-42) { minimal }'
```

### Bad: Grep for structured data

```bash
# Unreliable, may return thousands of lines
mytool grep "active"
```

### Good: DSL for structured queries

```bash
# Exact, compact result
mytool q 'list(status=active) { id name }'
```
