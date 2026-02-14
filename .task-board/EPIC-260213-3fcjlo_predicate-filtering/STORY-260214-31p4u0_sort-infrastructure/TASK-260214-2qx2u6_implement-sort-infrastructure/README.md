# TASK-260214-2qx2u6: implement-sort-infrastructure

## Description
Create sort.go with:
- SortComparator[T any] = func(a, b T) int type alias
- SortSpec{Field, Direction} and SortDirection (Asc/Desc) types in types.go
- SortFieldOf[T any, V cmp.Ordered](accessor func(T) V) SortComparator[T] convenience
- Schema.SortField(name, SortComparator[T]) method — adds to sortFields map + sortFieldNames slice
- SortableField[T, V cmp.Ordered](schema, name, accessor) standalone convenience
- SortableFieldFunc[T](schema, name, func(a,b T) int) standalone for custom comparators
- ParseSortSpecs(args []Arg) ([]SortSpec, error) — extracts sort_ prefixed args, validates asc/desc
- BuildSortFunc[T](specs, sortFields) (func(T,T) int, error) — chains comparators, first-non-zero-wins, direction negation
- SortSlice[T](items, args, sortFields) error — parse + build + slices.SortStableFunc
- Add sortFields + sortFieldNames to Schema struct, init in NewSchema
- Schema.SortFields() getter for handler access
- Add sortableFields to introspect() output
- Tests in sort_test.go: registration, SortFieldOf for string/int, custom comparator, ParseSortSpecs valid/invalid, single-field sort, multi-field sort, direction handling, empty sort (no-op), unknown field error, introspection

Requires Go 1.21+ for cmp/slices packages.
Definition of Done: all tests pass, go vet clean, existing tests unbroken.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
