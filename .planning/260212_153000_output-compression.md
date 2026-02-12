# Output Compression — Execution Plan

**Epic:** EPIC-260212-1d8i05
**Date:** 2026-02-12

---

## Architecture Decision

Based on research (.research/260212_output-compression-formats.md):

- **OutputMode**: `HumanReadable` (default, JSON) / `LLMReadable` (compact tabular)
- **Format**: Schema-once header + CSV-style value rows for `list()`, JSON for `get()` single objects
- **Config**: Via `WithOutputMode()` functional option on `NewSchema()`
- **Backward compatible**: HumanReadable is default, nothing breaks

## Serialization Points (where to inject)

1. `query.go:46` — `Schema.QueryJSON()` → `json.Marshal(result)`
2. `schema.go:158` — `Schema.SearchJSON()` → `json.MarshalIndent(results)`
3. `cobraext/command.go:22,55` — stdout output

## Key Insight

`FieldSelector.ordered` already holds the field names in deterministic order → perfect for header generation. `FieldSelector.Apply()` returns `map[string]any` → we iterate `ordered` to get values in column order. No new parsing needed.

---

## Phase 1: Research (fast-track)

Research is 80% done from TOON article analysis. Fast-track: benchmark current output, finalize doc.

**Agent:** coordinator (me)
**Tasks:**
- TASK-260212-2y6k7j: benchmark-current-json-output
- TASK-260212-3mpvv2: analyze-toon-format (done in .research/)
- TASK-260212-35m3bj: analyze-csv-tsv-alternatives
- TASK-260212-kvy4rk: evaluate-hybrid-approach
- TASK-260212-287z5q: write-research-document

**Duration:** Quick — mostly documenting known decisions.

---

## Phase 2: Implementation (parallel agents)

### Agent A: Query Output Compression

**Scope:** Core library changes for compact query output.

**Files to modify:**
- `agentquery/schema.go` — add `OutputMode`, `WithOutputMode()`, store in Schema struct
- `agentquery/types.go` — add `OutputMode` type + constants
- NEW `agentquery/format.go` — tabular formatter (FormatTabular function)
- `agentquery/query.go` — `QueryJSON()` switches on output mode
- `agentquery/selector.go` — add `ApplyValues()` method (returns `[]any` in field order, no keys)

**Tasks:**
- TASK-260212-fgxx71: design-output-format-api
- TASK-260212-sf45bz: implement-tabular-serializer
- TASK-260212-392fwd: implement-format-selection
- TASK-260212-22gc0v: tests-and-benchmarks (query part)

### Agent B: Search Output Compression + Cobraext (parallel with A)

**Scope:** Compact search output + CLI flag.

**Files to modify:**
- `agentquery/search.go` — add `FormatSearchCompact()` grouped-by-file output
- `agentquery/schema.go` — `SearchJSON()` respects OutputMode
- `agentquery/cobraext/command.go` — add `--format` flag to both commands

**Tasks:**
- TASK-260212-11nbdd: analyze-search-output-overhead
- TASK-260212-2adqct: implement-grouped-search-output
- TASK-260212-rwcy06: search-output-tests
- TASK-260212-13kshq: add-cobraext-format-flag

**Conflict avoidance:** Agent A owns types.go + format.go + query.go + selector.go. Agent B owns search.go + cobraext/command.go. schema.go is shared — Agent A adds OutputMode field/option first, Agent B reads it.

→ **Agent A starts first, Agent B starts after OutputMode type is defined** (TASK-260212-fgxx71 done).

---

## Phase 3: Docs (after Phase 2)

### Agent C: Documentation

**Tasks:**
- TASK-260212-1yzng0: update-skill-md
- TASK-260212-5w79t2: update-readme
- TASK-260212-yq8e9i: update-example

---

## Compact Output Format Spec

### Query list() — LLMReadable:
```
id,name,status,assignee
task-1,Auth service refactor,in-progress,alice
task-2,Dashboard performance,todo,bob
task-3,Fix login redirect bug,done,alice
```

### Query get() — LLMReadable (single object, no repeated keys anyway):
```
id:task-1
name:Auth service refactor
status:in-progress
assignee:alice
```

### Search — LLMReadable (grouped by file):
```
README.md
  3: This line matches
  4: context line
progress.md
  12: another match
```

### Fallback to JSON:
- Nested values (maps, slices inside fields)
- Batch with mixed operation types
- Error responses
