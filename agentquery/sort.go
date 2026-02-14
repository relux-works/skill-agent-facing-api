package agentquery

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// SortFieldOf creates a SortComparator from a typed accessor.
// Works for any cmp.Ordered type (int, string, float64, etc.).
// The comparator compares accessor(a) vs accessor(b) in natural order.
func SortFieldOf[T any, V cmp.Ordered](accessor func(T) V) SortComparator[T] {
	return func(a, b T) int {
		return cmp.Compare(accessor(a), accessor(b))
	}
}

// SortableField registers a field as sortable using a typed accessor.
// Convenience wrapper: creates a comparator via SortFieldOf and registers it.
//
// This is a package-level function (not a Schema method) because Go methods
// cannot introduce additional type parameters beyond the receiver's.
//
// Usage:
//
//	agentquery.SortableField(schema, "name", func(t Task) string { return t.Name })
func SortableField[T any, V cmp.Ordered](s *Schema[T], name string, accessor func(T) V) {
	s.SortField(name, SortFieldOf[T, V](accessor))
}

// SortableFieldFunc registers a field as sortable with a custom comparator.
// Use when sort order is not the natural order of a single field value
// (e.g., enum priority ranking, multi-field derived sort).
//
// This is a package-level function (not a Schema method) because Go methods
// cannot introduce additional type parameters beyond the receiver's.
func SortableFieldFunc[T any](s *Schema[T], name string, compare SortComparator[T]) {
	s.SortField(name, compare)
}

// ParseSortSpecs extracts sort specifications from args.
// Sort args are identified by the "sort_" prefix on the key.
// Value must be "asc" or "desc" (case-insensitive). Default (empty) is "asc".
//
// Example args: sort_priority=desc, sort_name=asc
// Returns: [{Field: "priority", Direction: Desc}, {Field: "name", Direction: Asc}]
func ParseSortSpecs(args []Arg) ([]SortSpec, error) {
	var specs []SortSpec
	for _, arg := range args {
		if !strings.HasPrefix(arg.Key, "sort_") {
			continue
		}
		field := arg.Key[5:] // strip "sort_" prefix
		if field == "" {
			return nil, &Error{
				Code:    ErrValidation,
				Message: "sort_ prefix requires a field name",
				Details: map[string]any{"arg": arg.Key},
			}
		}

		dir := Asc
		switch strings.ToLower(arg.Value) {
		case "asc", "":
			dir = Asc
		case "desc":
			dir = Desc
		default:
			return nil, &Error{
				Code:    ErrValidation,
				Message: fmt.Sprintf("sort direction must be 'asc' or 'desc', got %q", arg.Value),
				Details: map[string]any{"field": field, "value": arg.Value},
			}
		}

		specs = append(specs, SortSpec{Field: field, Direction: dir})
	}
	return specs, nil
}

// BuildSortFunc constructs a comparison function from sort specs and registered comparators.
// Returns nil if specs is empty (no sorting needed).
// Returns an error if a spec references an unregistered sort field.
// Multi-field sort uses "first non-zero wins" chaining (like cmp.Or).
func BuildSortFunc[T any](specs []SortSpec, sortFields map[string]SortComparator[T]) (func(T, T) int, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	type step struct {
		compare SortComparator[T]
		desc    bool
	}
	steps := make([]step, len(specs))

	for i, spec := range specs {
		cmpFn, ok := sortFields[spec.Field]
		if !ok {
			return nil, &Error{
				Code:    ErrValidation,
				Message: fmt.Sprintf("field %q is not sortable", spec.Field),
				Details: map[string]any{"field": spec.Field},
			}
		}
		steps[i] = step{compare: cmpFn, desc: spec.Direction == Desc}
	}

	return func(a, b T) int {
		for _, s := range steps {
			result := s.compare(a, b)
			if s.desc {
				result = -result
			}
			if result != 0 {
				return result
			}
		}
		return 0
	}, nil
}

// SortSlice sorts items in-place using sort specifications extracted from args.
// Sort args have the "sort_<field>=asc|desc" convention.
// If no sort_* args are present, items are returned unchanged (no-op).
// Uses slices.SortStableFunc for deterministic results.
//
// Usage in operation handlers:
//
//	items, err := ctx.Items()
//	// ... filter items ...
//	if err := agentquery.SortSlice(items, ctx.Statement.Args, schema.SortFields()); err != nil {
//	    return nil, err
//	}
//	// ... paginate items ...
func SortSlice[T any](items []T, args []Arg, sortFields map[string]SortComparator[T]) error {
	specs, err := ParseSortSpecs(args)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return nil // no sorting requested
	}

	cmpFunc, err := BuildSortFunc(specs, sortFields)
	if err != nil {
		return err
	}

	slices.SortStableFunc(items, cmpFunc)
	return nil
}
