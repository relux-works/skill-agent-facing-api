# Consumer Filtering Pain Point Analysis

## Overview

This document analyzes how filtering is implemented across the two known consumers of the `agentquery` library, quantifies the boilerplate, catalogs filter patterns, and identifies pain points.

Consumers analyzed:
1. **Example CLI** (`example/main.go`) -- simple demo with 2 filter fields
2. **board-cli** (`skill-project-management/tools/board-cli/`) -- real-world consumer with 5 filter fields, plus an MCP server that re-implements the same filtering independently

---

## 1. Line Count of Filter Boilerplate Per Operation

### Consumer 1: Example CLI (`example/main.go`)

| Component | Lines | Description |
|-----------|-------|-------------|
| `taskFilterFromArgs()` | 20 (L189-209) | Shared predicate builder: parses args, builds closure |
| `opList` usage | 1 | `filtered := agentquery.FilterItems(items, taskFilterFromArgs(ctx.Statement.Args))` |
| `opCount` usage | 1 | `n := agentquery.CountItems(items, taskFilterFromArgs(ctx.Statement.Args))` |
| Metadata declarations (list) | 2 | ParameterDef for status, assignee in OperationMetadata |
| Metadata declarations (count) | 2 | Same ParameterDef duplicated for count |
| **Total filter-related boilerplate** | **~26 lines** | Across 2 operations + 1 shared function |

Pattern: `taskFilterFromArgs` iterates `[]Arg`, matches keys via `switch`, extracts values, returns `func(Task) bool` closure. Used by both `list` and `count`.

### Consumer 2: board-cli (`internal/boardquery/operations.go`)

| Component | Lines | Description |
|-----------|-------|-------------|
| `applyListFilters()` | 57 (L17-74) | Shared filter chain: iterates args, switch on 5 keys + skip/take passthrough + unknown-key error |
| `handleList` usage | 4 | Call applyListFilters + error check |
| `handleCount` usage | 4 | Call applyListFilters + error check |
| Metadata declarations (list) | 7 | ParameterDef for type/status/parent/assignee/blocked/skip/take |
| Metadata declarations (count) | 5 | ParameterDef for type/status/parent/assignee/blocked (subset) |
| Standalone filter helpers in `fields.go` | 43 (L144-197) | 5 redundant `filterByX()` functions (unused by operations, exist from pre-agentquery era) |
| **Total filter-related boilerplate** | **~120 lines** | Across 2 operations + 1 shared function + 5 dead helpers |

### Consumer 2b: board-cli MCP Server (`cmd/mcp-server/tools.go`)

The MCP server does NOT use the `agentquery` query engine for filtering. It re-implements all filtering manually on `[]*board.Element`:

| Component | Lines | Description |
|-----------|-------|-------------|
| `makeListElements` filter block | 30 (L534-577) | Manual filter chain: type, status, parent, assignee, blocked -- each a separate if-block |
| `makePlan` activeOnly filter | 7 (L696-704) | Manual status != done/closed filter |
| `makeAgents` freshness filter | 8 (L778-788) | Manual assignee + done+stale filter |
| Input struct declarations | 6 | Type/Status/Parent/Assignee/Blocked fields on `ListElementsInput` |
| **Total MCP filter boilerplate** | **~51 lines** | 3rd copy of the same logic, different types |

### Consumer 2c: board-cli Legacy CLI (`cmd/list.go`)

| Component | Lines | Description |
|-----------|-------|-------------|
| Filter flag vars | 3 | `listStatus`, `listEpic`, `listStory` |
| Flag registration | 3 (L28-30) | `StringVar` calls |
| Filter application | 12 (L83-101) | ParseStatus + FilterByStatus + FilterByParent x2 |
| **Total** | **~18 lines** | 4th copy, subset of filters |

### Consumer 2d: `board` package (`internal/board/board.go`)

| Component | Lines | Description |
|-----------|-------|-------------|
| `FilterByStatus()` | 8 (L167-175) | Standalone filter on `[]*Element` |
| `FilterByParent()` | 10 (L179-188) | Standalone filter, case-insensitive |
| **Total** | **~18 lines** | Base-level helpers, used by cmd/list.go and MCP server |

### Grand Total Across All Consumers

| Location | Filter lines |
|----------|-------------|
| example/main.go | ~26 |
| boardquery/operations.go | ~66 |
| boardquery/fields.go (dead helpers) | ~43 |
| mcp-server/tools.go | ~51 |
| cmd/list.go | ~18 |
| board/board.go | ~18 |
| **Total** | **~222 lines of filter boilerplate** |

---

## 2. Common Filter Patterns

### Pattern A: Case-insensitive string equality
Used for: `status`, `assignee`, `parent`

```go
// Example: assignee filter
if filterAssignee != "" && !strings.EqualFold(t.Assignee, filterAssignee) {
    return false
}
```

Variants:
- `strings.EqualFold(a, b)` -- used for assignee in example and board-cli
- `strings.ToUpper(a) == strings.ToUpper(b)` -- used for parent ID in board-cli
- Both are equivalent; inconsistent usage

### Pattern B: Parsed enum equality
Used for: `type` (ElementType), `status` (Status)

```go
// Must parse string arg into domain type first, then exact-match
elemType, err := board.ParseElementType(arg.Value)
if err != nil {
    return nil, err
}
items = agentquery.FilterItems(items, func(item BoardItem) bool {
    return item.Element.Type == elemType
})
```

This pattern has 2 steps: (1) parse/validate, (2) equality check.
The parse step can produce user-facing errors (e.g. "unknown status: xyz").

### Pattern C: Boolean flag filter
Used for: `blocked`

```go
if strings.ToLower(arg.Value) == "true" {
    items = agentquery.FilterItems(items, func(item BoardItem) bool {
        return item.Board.IsBlocked(item.Element)
    })
}
```

Needs to parse "true"/"false" string into boolean.

### Pattern D: Simple string equality (exact match)
Used for: `id` (in `get` operation)

```go
if task.ID == targetID { ... }
```

Not really "filtering" -- this is single-item lookup. But it shares the same arg-parsing pattern.

### Summary of patterns

| Pattern | Frequency | Examples |
|---------|-----------|---------|
| Case-insensitive string equality | 6 uses | assignee, parent, status (example) |
| Parsed enum equality | 4 uses | type, status (board-cli) |
| Boolean flag | 2 uses | blocked |
| Exact string equality | 2 uses | id |

---

## 3. What Percentage of Operations Need Filtering

### Example CLI

| Operation | Needs filtering? | Filter params |
|-----------|-----------------|---------------|
| `get` | No (single ID lookup) | - |
| `list` | **Yes** | status, assignee |
| `count` | **Yes** | status, assignee |
| `summary` | No | - |
| **Total** | **2/4 = 50%** | |

### board-cli (agentquery operations)

| Operation | Needs filtering? | Filter params |
|-----------|-----------------|---------------|
| `get` | No (single ID lookup) | - |
| `list` | **Yes** | type, status, parent, assignee, blocked |
| `count` | **Yes** | type, status, parent, assignee, blocked |
| `summary` | No | - |
| `plan` | No (scope param, not filter) | - |
| `agents` | Partial (stale threshold, not key-value filter) | - |
| **Total** | **2/6 = 33%** | |

### board-cli MCP server (separate from agentquery)

| Tool | Needs filtering? | Filter params |
|------|-----------------|---------------|
| `board_get_element` | No | - |
| `board_list_elements` | **Yes** | type, status, parent, assignee, blocked |
| `board_get_summary` | No | - |
| `board_plan` | Partial (activeOnly) | - |
| `board_agents` | Partial (stale/showAll) | - |
| `board_search` | No (pattern-based) | - |
| Write tools (6) | No | - |
| **Total** | **1/12 = 8%** (but it's the most complex one) | |

**Key insight**: Only ~30-50% of operations use the standard key=value filter pattern, but those operations (`list` and `count`) are the most frequently called and have the most complex boilerplate. The filter logic is also the part most likely to grow (new fields, new filter types).

---

## 4. All Filter Fields Used Across Consumers

| Field | Example CLI | board-cli (query) | board-cli (MCP) | board-cli (legacy) | Type |
|-------|:-----------:|:-----------------:|:---------------:|:------------------:|------|
| `status` | x | x | x | x | Parsed enum (case-insensitive) |
| `assignee` | x | x | x | - | Case-insensitive string equality |
| `type` | - | x | x | x (positional arg) | Parsed enum |
| `parent` | - | x | x | x (as `--epic`/`--story`) | Case-insensitive string equality |
| `blocked` | - | x | x | - | Boolean |
| `skip` | - | x (pagination) | - (uses `limit`) | - | Integer (not really a filter) |
| `take` | - | x (pagination) | - (uses `limit`) | - | Integer (not really a filter) |

**Unique filter fields (excluding pagination):** 5 total (`status`, `assignee`, `type`, `parent`, `blocked`)

---

## 5. Operators Actually Used

| Operator | Used? | Where |
|----------|-------|-------|
| Equality (`=`) | **Yes, everywhere** | All filter fields |
| Case-insensitive equality | **Yes** | assignee, parent, status (example) |
| Greater than (`>`) | No | - |
| Less than (`<`) | No | - |
| Contains / substring | No | - |
| Not equal (`!=`) | No | - |
| In / one-of | No | - |
| Regex match | No (only in grep/search, separate from filter) | - |

**Only equality is used.** All filtering is `field = value` with optional case-insensitivity. No range operators, no containment checks, no negation.

The case-insensitivity varies by field:
- `status` and `type`: parsed through a domain function that handles case normalization internally (e.g. `ParseStatus("DONE")` returns `StatusDone`)
- `assignee`: compared with `strings.EqualFold()`
- `parent`: compared with `strings.ToUpper()` on both sides

---

## 6. Pain Points

### P1: Quadruple implementation of the same filters (Critical)

The same 5 filter fields are implemented in **4 separate places** across board-cli:
1. `boardquery/operations.go` -- `applyListFilters()` for agentquery DSL
2. `boardquery/fields.go` -- standalone `filterByX()` helpers (partially dead code)
3. `cmd/mcp-server/tools.go` -- manual filter chain for MCP tools
4. `cmd/list.go` + `board/board.go` -- legacy CLI filters

Adding a new filter field (e.g. `priority`, `label`) requires changes in all 4 places. This is the #1 source of bugs and maintenance pain.

### P2: Arg-parsing boilerplate is repetitive and error-prone (High)

Every consumer must:
1. Iterate `ctx.Statement.Args`
2. Match `arg.Key` in a `switch` statement
3. Extract and validate `arg.Value` (parse enum, case-normalize, etc.)
4. Build a predicate or apply a filter
5. Handle unknown keys explicitly
6. Explicitly skip `skip`/`take` keys (handled by pagination)

This is ~57 lines in board-cli's `applyListFilters` and ~20 lines in example's `taskFilterFromArgs`. Most of it is structural, not domain-specific.

### P3: Filter metadata must be declared separately from filter logic (Medium)

The `OperationMetadata.Parameters` list duplicates filter field names:
```go
Parameters: []agentquery.ParameterDef{
    {Name: "type", Type: "string", Optional: true, Description: "Filter by element type"},
    {Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
    ...
}
```

These are declared in `registerOperations()` but the actual filtering logic lives in `applyListFilters()`. The two can drift apart. If someone adds a filter to `applyListFilters` but forgets the ParameterDef, `schema()` will report incomplete metadata.

This is duplicated again between `list` and `count` -- identical ParameterDefs minus `skip`/`take`.

### P4: No type safety for filter values (Medium)

All filter values arrive as `string`. Consumers must parse them (e.g. `board.ParseStatus(arg.Value)`) and handle errors. The library provides no mechanism to declare that a field accepts an enum with known values, or a boolean, or an integer.

If the library knew field types, it could:
- Validate filter values at parse time (not execution time)
- Auto-generate better error messages
- Auto-generate schema metadata

### P5: "Unknown filter" error handling is manual (Low)

Each consumer's filter function needs a `default` case in the switch to reject unknown keys. If `applyListFilters` doesn't have the `default` case, garbage like `list(bogus=x)` silently passes. The library should handle this, not each consumer.

### P6: Inconsistent case handling (Low)

- `assignee`: `strings.EqualFold()` in both example and board-cli
- `parent`: `strings.ToUpper()` in board-cli
- `status`: parsed through `ParseStatus()` which does `strings.ToLower()` internally
- `type`: parsed through `ParseElementType()` which does `strings.ToLower()` internally

All of these are "case-insensitive equality" but implemented differently. A library-level filter mechanism should normalize this.

### P7: Dead code accumulation (Low)

`boardquery/fields.go` has 5 standalone filter functions (`filterByElementType`, `filterByItemStatus`, `filterByItemParent`, `filterByItemAssignee`, `filterByItemBlocked`) totaling 43 lines. These are tested but appear to be remnants from before `applyListFilters` was written. They're not used by any operation handler -- the operations use `applyListFilters` which calls `agentquery.FilterItems` with inline predicates instead.

---

## Summary Table

| Metric | Value |
|--------|-------|
| Total filter boilerplate lines across all consumers | ~222 |
| Unique filter fields | 5 (status, type, parent, assignee, blocked) |
| Operators used | Only equality (=), always case-insensitive |
| Operations that need filtering | 30-50% (but they're the most-used ones) |
| Number of independent filter implementations in board-cli | 4 (quadruple duplication) |
| Biggest pain point | Adding a new filter field requires changes in 4 places |
