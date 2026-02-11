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

**With limit:**
```
list(type=task, status=pending, limit=5) { default }
```

**Blocked items:**
```
list(blocked=true) { id name status }
```

---

## 3. Summary / Overview

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

## 4. Batch Queries

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

## 5. Grep Patterns

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

## 6. Anti-Patterns

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
