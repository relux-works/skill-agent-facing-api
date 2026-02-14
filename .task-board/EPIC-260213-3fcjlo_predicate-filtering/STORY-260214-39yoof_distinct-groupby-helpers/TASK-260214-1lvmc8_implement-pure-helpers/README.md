# TASK-260214-1lvmc8: implement-pure-helpers

## Description
Add to helpers.go (or new distinct.go):
- Distinct[T](items []T, keyFn func(T) string) []string — unique values in first-seen order
- DistinctCount[T](items []T, keyFn func(T) string) map[string]int — count per value, no intermediate allocation
- GroupBy[T](items []T, keyFn func(T) string) map[string][]T — full grouping, preserves item order within groups

Tests: empty items, single item, all duplicates, mixed values, order preservation for Distinct, correct counts for DistinctCount, group integrity for GroupBy, empty string as valid key.

NO dependencies on filter/sort — pure generic functions.
Definition of Done: all tests pass, go vet clean.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
