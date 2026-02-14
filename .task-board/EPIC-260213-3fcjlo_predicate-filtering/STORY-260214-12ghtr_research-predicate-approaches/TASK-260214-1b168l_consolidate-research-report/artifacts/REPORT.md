# Predicate Filtering: Consolidated Research Report

## Date: 2026-02-14
## Status: Final

---

## Executive Summary

Four parallel research threads analyzed: (1) consumer pain points, (2) DSL grammar extensions, (3) declarative filter API design, (4) industry patterns from 7 major systems. This report synthesizes findings into an optimal approach for agentquery.

**Bottom line:** Two-phase approach. Phase 1 solves 100% of current pain with declarative equality filtering and zero parser changes. Phase 2 adds operators (`!=`, `>`, `<`, `~=`) when real demand materializes.

---

## The Problem (Quantified)

| Metric | Value |
|--------|-------|
| Total filter boilerplate across consumers | **~222 lines** |
| Unique filter fields | 5 (status, type, parent, assignee, blocked) |
| Independent filter implementations in board-cli | **4** (quadruple duplication) |
| Files to change when adding new filter field | **4** |
| Operators actually used | **Only equality** (`=`), always case-insensitive |
| Operations that need filtering | 30-50% (but they're the highest-frequency ones: `list`, `count`) |

The pain is real and concentrated: adding one filter field to board-cli requires touching 4 separate code locations with no enforcement of consistency. Filter metadata (ParameterDef) drifts from actual filter logic.

---

## Research Inputs

### Thread 1: Consumer Pain Analysis
- **Source:** `TASK-260214-29kw5b/artifacts/RESEARCH.md`
- **Key finding:** Only equality is used. No `>`, `<`, `contains`, `!=` anywhere in production. All filtering is case-insensitive string matching. Pain is not about operator expressiveness — it's about boilerplate and duplication.

### Thread 2: DSL Grammar Extensions
- **Source:** `TASK-260214-3mhkn4/artifacts/RESEARCH.md`
- **Winner:** Approach A (extended operator tokens: `!=`, `>`, `<`, `>=`, `<=`, `~=`). ~80-100 LOC parser changes, 100% backwards compatible, best agent ergonomics (SQL-like syntax).
- **Runner-up:** Approach D (keep grammar as-is) — no capability gain but no cost either.

### Thread 3: Declarative Filter API
- **Source:** `TASK-260214-2pjbph/artifacts/RESEARCH.md`
- **Winner:** Approach 3 (Predicate Builder on OperationContext). `ctx.Predicate` is pre-built from registered filters + args. Handler opts in via `FilterItems(items, ctx.Predicate)`. Follows `PaginateSlice` pattern — helper not mandate.
- **Key Go constraint:** `FilterableField` must be a standalone function, not a Schema method (Go methods can't have their own type parameters).

### Thread 4: Industry Patterns
- **Source:** `TASK-260214-1gj58t/artifacts/RESEARCH.md`
- **7 systems analyzed:** GraphQL, OData, Django, Prisma, GORM, PostgREST, Strapi/Directus.
- **Universal patterns:** equality-by-default, implicit AND, core operator set (eq/ne/gt/lt/contains/in).
- **Recommended syntax:** Django-style `__` operators (`status__ne=done`, `priority__gt=3`) — zero parser changes, proven 20 years.

---

## The Central Tension

Two grammar approaches compete:

| | Extended Operators (Thread 2) | Django `__` (Thread 4) |
|---|---|---|
| Syntax | `list(status!=done, priority>3)` | `list(status__ne=done, priority__gt=3)` |
| Parser changes | ~80-100 LOC (new tokens + AST field) | Zero |
| Validation | Parse-time (library principle) | Execution-time |
| Agent ergonomics | Natural (SQL-like, universal) | Good (but Django-specific convention) |
| Backwards compat | 100% | 100% |
| Token efficiency | Slightly better (14 vs 15 chars) | Slightly worse |

**But here's the thing:** neither matters for Phase 1. Current consumers use ONLY equality. The operator question is a Phase 2 concern.

---

## Recommended Approach: Two Phases

### Phase 1: Declarative Equality Filtering (Priority — solves 100% of current pain)

**Grammar:** No changes. The existing `key=value` syntax stays.

**API:**

```go
// Registration (standalone function — Go generics constraint)
agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })

// Handler usage — replaces taskFilterFromArgs entirely
func opList(ctx agentquery.OperationContext[Task]) (any, error) {
    items, err := ctx.Items()
    if err != nil { return nil, err }

    filtered := agentquery.FilterItems(items, ctx.Predicate)  // auto-built
    page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
    if err != nil { return nil, err }

    results := make([]map[string]any, 0, len(page))
    for _, task := range page {
        results = append(results, ctx.Selector.Apply(task))
    }
    return results, nil
}
```

**What changes:**

| Component | Change | LOC estimate |
|-----------|--------|-------------|
| `types.go` | Add `Predicate func(T) bool` to `OperationContext` | ~3 |
| `schema.go` | Add `filters map[string]func(T) string` + `filterOrder []string` to Schema. Init in `NewSchema`. | ~10 |
| `filter.go` (new) | `FilterableField[T]()` standalone function + `buildPredicate()` method | ~50 |
| `query.go` | Inject `Predicate` in `executeStatement` | ~3 |
| `schema.go` introspect | Add `filterableFields` to `schema()` output | ~8 |
| Tests | Registration, predicate building, edge cases, introspection | ~150 |
| **Total** | | **~225 LOC** |

**What it eliminates:**

| Consumer | Lines eliminated |
|----------|----------------|
| example/main.go | ~20 (entire `taskFilterFromArgs`) |
| board-cli/boardquery/operations.go | ~57 (`applyListFilters`) |
| board-cli/boardquery/fields.go | ~43 (dead filter helpers) |
| board-cli MCP server | ~30+ (manual filter chain) |
| **Total** | **~150+ lines of boilerplate** |

**Design decisions:**

1. **String-only accessors** — `func(T) string`. DSL args are strings. Comparing strings eliminates runtime type assertions. Non-string fields use `strconv.Itoa` etc. in the accessor. Covers 95%+ of cases.

2. **Case-insensitive by default** — `strings.EqualFold()`. Matches all current consumer behavior.

3. **AND-only** — multiple filter args are AND-ed. Covers 100% of current use cases.

4. **Silent ignore for unknown filter args** — matches current behavior. Strict mode can be added later.

5. **Skip pagination args** — `buildPredicate` only matches against registered filter names (positive match). `skip`, `take`, and any unregistered args are not treated as filters.

6. **`ctx.Predicate` never nil** — returns `MatchAll` when no filters match. No nil checks needed.

7. **Schema introspection** — `schema()` includes `filterableFields: ["status", "assignee", ...]` when any filters are registered.

**Migration path (non-breaking, gradual):**
1. Library adds `FilterableField` + `ctx.Predicate` (additive, no breakage)
2. Consumer adds filter registrations (additive, no breakage)
3. Consumer replaces `taskFilterFromArgs(ctx.Statement.Args)` with `ctx.Predicate` (one-liner swap)
4. Consumer deletes `taskFilterFromArgs` (cleanup)

Each step is independently committable and safe.

---

### Phase 2: Operator Support (Future — when demand materializes)

**Trigger:** First real-world need for `!=`, `>`, `<`, or `contains`.

**Grammar decision (deferred):** Two viable approaches:

| Approach | When to choose it |
|----------|------------------|
| **Extended operators** (`!=`, `>`) | If we value parse-time validation and natural syntax |
| **Django `__`** (`status__ne`) | If we value zero parser changes and want operators ASAP |

**Recommendation:** Extended operators. Reasons:
- Parse-time validation is a core library principle
- `status!=done` is universally understood by LLMs (SQL-like)
- The parser changes are small (~80-100 LOC) and well-scoped
- Adds `Operator` field to `Arg` AST — clean, explicit

**Phase 2 scope:**
- New tokens: `!=`, `>`, `<`, `>=`, `<=`, `~=`
- `Arg.Operator` field on AST
- `buildPredicate` becomes operator-aware (dispatch on operator)
- `FilterableFieldWithOps` for typed comparison registration
- Schema introspection adds supported operators per field

**Phase 2 can be further split:**
- 2a: `!=` only (most useful single addition)
- 2b: `>`, `<`, `>=`, `<=` (comparison)
- 2c: `~=` (contains/regex)

---

## What We're NOT Doing (and why)

| Rejected approach | Why |
|-------------------|-----|
| Filter middleware (auto-filter `ctx.Items()`) | Changes Items() semantics, causes double-filtering for migrating consumers, needs opt-out mechanism. Violates "helpers not mandates" principle. |
| `where()` clause in grammar | Overengineered for single-entity filtering. Adds verbosity (~10 chars per query). No need until multi-entity/JOIN operations exist. |
| Value-prefix convention (`status="!done"`) | Breaks parse-time validation. Requires quoting. Asymmetric syntax. Each consumer reimplements prefix parsing. |
| OR/NOT compound logic | Zero current use cases. AND covers everything. Can be added later without breaking changes. |
| Typed filter accessors (`func(T) int`, `func(T) bool`) | Runtime type assertions for heterogeneous storage. String-only is simpler and sufficient. |

---

## Development Decomposition (Phase 1)

### Stories and Tasks

**Story 1: Core filter infrastructure**
1. Add `filters` + `filterOrder` to `Schema` struct, init in `NewSchema`
2. Implement `FilterableField[T]()` standalone registration function
3. Implement `buildPredicate()` on Schema (equality, case-insensitive, AND-only)
4. Add `Predicate` field to `OperationContext`, inject in `executeStatement`
5. Tests: registration, predicate building, unknown args, empty filters, case insensitivity

**Story 2: Schema introspection**
1. Add `filterableFields` to `introspect()` output
2. Tests: introspection with/without filters

**Story 3: Example migration**
1. Add `FilterableField` registrations to example/main.go
2. Replace `taskFilterFromArgs` usage with `ctx.Predicate`
3. Delete `taskFilterFromArgs`
4. Verify all example queries still work

**Story 4: Documentation**
1. Update CLAUDE.md with filter API docs
2. Update SKILL.md DSL grammar section
3. Update assets/query-patterns-catalog if exists

### Dependency graph

```
Story 1 (core) → Story 2 (introspection)
Story 1 (core) → Story 3 (example migration)
Story 2 + Story 3 → Story 4 (docs)
```

Stories 2 and 3 can run in parallel after Story 1.

---

## Appendix: Full Research Artifacts

| Thread | File |
|--------|------|
| Consumer pain | `TASK-260214-29kw5b/artifacts/RESEARCH.md` |
| DSL grammar | `TASK-260214-3mhkn4/artifacts/RESEARCH.md` |
| Filter API | `TASK-260214-2pjbph/artifacts/RESEARCH.md` |
| Industry patterns | `TASK-260214-1gj58t/artifacts/RESEARCH.md` |
