# TASK-260214-3tomuk: migrate-example

## Description
Update example/main.go:
- Register FilterableField for status, assignee
- Register SortField for id (string), name (string), status (custom enum order), assignee (string)
- Replace taskFilterFromArgs with ctx.Predicate in opList and opCount
- Add SortSlice call in opList between filter and paginate
- Delete taskFilterFromArgs function entirely
- Update opCount to use ctx.Predicate
- Add sort parameter metadata to list OperationMetadata
- Verify all existing example queries still work
- Add new example queries in comments: sort, filtered distinct

Tests: run all existing example queries, verify same output. Run new sort/distinct queries.
DEPENDS ON: filter + sort + distinct tasks.
Definition of Done: example builds, all queries produce correct output.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
