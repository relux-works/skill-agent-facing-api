# TASK-260214-2g0psc: implement-filterable-field

## Description
Create filter.go with:
- FilterableField[T](schema, name, func(T) string) standalone registration function
- Schema.filters map[string]func(T) string + filterOrder []string (add to schema.go, init in NewSchema)
- buildPredicate() method on Schema: equality, case-insensitive (strings.EqualFold), AND-only, skips positional args and unregistered keys
- Add Predicate func(T) bool field to OperationContext (types.go)
- Inject Predicate in executeStatement (query.go): s.buildPredicate(stmt.Args), never nil (returns MatchAll)
- Add filterableFields to introspect() output (schema.go)
- Tests in filter_test.go: registration, predicate building with single/multiple filters, unknown args ignored, empty filters return MatchAll, case insensitivity, introspection with/without filters, ctx.Predicate injection

Definition of Done: all tests pass, go vet clean, existing tests unbroken.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
