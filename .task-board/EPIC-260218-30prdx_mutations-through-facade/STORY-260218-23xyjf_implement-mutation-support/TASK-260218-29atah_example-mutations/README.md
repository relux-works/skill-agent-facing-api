# TASK-260218-29atah: example-mutations

## Description
Phase 4: Update example CLI with mutation support.

EXTEND: example/main.go
- Replace sampleTasks() with a mutable in-memory store (slice + mutex or simple map)
- Register mutation operations: create, update, delete with MutationWithMetadata()
- Implement handlers: createHandler, updateHandler, deleteHandler
- Each handler supports dry_run via ctx.DryRun
- create: generates ID, adds to store, returns created entity
- update: finds by positional ID arg, updates fields from ArgMap, returns updated entity
- delete: finds by positional ID arg, removes from store, returns confirmation

TESTS: Manual — run taskdemo CLI with mutation commands, verify output in both json and compact formats

Design: .research/260218_mutation_design_proposal.md section 2 (example code)

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
