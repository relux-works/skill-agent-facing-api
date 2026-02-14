# TASK-260214-35j9ma: implement-distinct-operation

## Description
Auto-register built-in 'distinct' DSL operation:
- distinctHandler on Schema: first positional arg = field name, looks up FilterableField accessor
- Applies ctx.Predicate for filtered distinct (e.g. distinct(assignee, status=done))
- count=true parameter switches to DistinctCount output
- Auto-register in FilterableField() on first registration (lazy — operation only appears when filters exist)
- OperationMetadata with description, parameters, examples
- Tests: distinct(field), distinct with filter, distinct with count=true, unknown field error, no positional arg error, batch with distinct, introspection shows distinct operation

DEPENDS ON: implement-filterable-field (needs FilterableField accessors + ctx.Predicate)
Definition of Done: all tests pass, go vet clean.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
