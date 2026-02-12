# Plan: Pagination, Count, Schema Introspection

**Date:** 2026-02-13
**Epic:** EPIC-260211-24a18e (generic-go-query-library)

## Phase 1: Core Features (parallel)

Pagination and count are independent — can be done in parallel.

### Stream A: Skip/Take Pagination (STORY-260213-kgw7l0)

| Order | Task | ID | Blocked By |
|-------|------|----|------------|
| 1 | Parse skip/take params | TASK-260213-3bst9i | — |
| 2 | Apply pagination to results | TASK-260213-26rxne | 3bst9i |
| 3a | Pagination tests | TASK-260213-28grki | 26rxne |
| 3b | Update example CLI | TASK-260213-mh0fu9 | 26rxne |

### Stream B: Count Operation (STORY-260213-ajyfs9)

| Order | Task | ID | Blocked By |
|-------|------|----|------------|
| 1 | Implement count() | TASK-260213-3sy7kc | — |
| 2 | Count compact output | TASK-260213-b7kl7h | 3sy7kc |
| 3a | Count tests | TASK-260213-3at5ci | b7kl7h |
| 3b | Update example CLI | TASK-260213-3rlqoy | 3sy7kc |

## Phase 2: Schema Introspection (STORY-260213-e59yqs)

Blocked by Phase 1 (needs pagination and count to exist first).

| Order | Task | ID | Blocked By |
|-------|------|----|------------|
| 1 | Add OperationMetadata types | TASK-260213-uhu3kk | Phase 1 stories |
| 2 | Add OperationWithMetadata() | TASK-260213-uzvohq | uhu3kk |
| 3 | Update introspect() output | TASK-260213-2jv2ub | uzvohq |
| 4a | Schema introspection tests | TASK-260213-2osrnu | 2jv2ub |
| 4b | Register metadata in example | TASK-260213-3bl0ru | 2jv2ub |

## Phase 3: Documentation (STORY-260213-2dgd4o)

Blocked by Phase 2.

| Order | Task | ID |
|-------|------|----|
| 1 | Update SKILL.md DSL grammar | TASK-260213-34oiv3 |
| 2 | Update query patterns catalog | TASK-260213-3tofjq |
| 3 | Update CLAUDE.md | TASK-260213-2sn6hb |
| 4 | Update reference implementations | TASK-260213-zqtw5j |

## Dependency Graph

```
Phase 1 (parallel):
  A: parse-params → apply-pagination → tests + example
  B: impl-count → compact-output → tests + example

Phase 2 (after Phase 1):
  metadata-types → with-metadata-method → update-introspect → tests + register-example

Phase 3 (after Phase 2):
  skill-md → patterns → claude-md → reference-impls
```

## Research
- .research/260213_pagination-discoverability.md — decision on schema() vs SKILL.md
