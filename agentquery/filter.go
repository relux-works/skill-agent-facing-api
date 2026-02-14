package agentquery

import (
	"fmt"
	"strings"
)

// FilterableField registers a field as filterable on the schema.
// The accessor extracts the string value used for case-insensitive equality
// comparison against query arguments.
//
// This is a package-level function (not a Schema method) because Go methods
// cannot introduce additional type parameters beyond the receiver's.
//
// On first registration, auto-registers a built-in "distinct" operation
// (if not already manually registered) that returns unique values for any
// filterable field: distinct(field_name) -> ["val1", "val2", ...].
//
// Usage:
//
//	agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
func FilterableField[T any](s *Schema[T], name string, accessor func(T) string) {
	if s.filters == nil {
		s.filters = make(map[string]func(T) string)
	}
	if _, exists := s.filters[name]; !exists {
		s.filterOrder = append(s.filterOrder, name)
	}
	s.filters[name] = accessor

	// Auto-register distinct operation on first filterable field registration.
	if _, exists := s.operations["distinct"]; !exists {
		s.registerDistinctOperation()
	}
}

// buildPredicate creates a predicate from registered filters and query args.
// Returns MatchAll if no args match registered filters.
// Only matches args whose Key is a registered filter name (positive match).
// Positional args (Key="") are skipped.
// Comparison is case-insensitive string equality (strings.EqualFold).
// Multiple matching filters are AND-ed.
func (s *Schema[T]) buildPredicate(args []Arg) func(T) bool {
	type pair struct {
		accessor func(T) string
		value    string
	}

	var pairs []pair
	for _, arg := range args {
		if arg.Key == "" {
			continue
		}
		if accessor, ok := s.filters[arg.Key]; ok {
			pairs = append(pairs, pair{accessor: accessor, value: arg.Value})
		}
	}

	if len(pairs) == 0 {
		return MatchAll[T]()
	}

	return func(item T) bool {
		for _, p := range pairs {
			if !strings.EqualFold(p.accessor(item), p.value) {
				return false
			}
		}
		return true
	}
}

// registerDistinctOperation registers the built-in "distinct" operation
// with metadata for schema introspection. The handler extracts unique values
// for a named filterable field from the dataset.
//
// DSL: distinct(field_name) -> ["val1", "val2", ...]
func (s *Schema[T]) registerDistinctOperation() {
	handler := func(ctx OperationContext[T]) (any, error) {
		// Extract field name from positional arg (first arg with Key="").
		var fieldName string
		for _, arg := range ctx.Statement.Args {
			if arg.Key == "" {
				fieldName = arg.Value
				break
			}
		}
		if fieldName == "" {
			return nil, &Error{
				Code:    ErrValidation,
				Message: "distinct requires a field name argument: distinct(field_name)",
			}
		}

		accessor, ok := s.filters[fieldName]
		if !ok {
			known := make([]string, len(s.filterOrder))
			copy(known, s.filterOrder)
			return nil, &Error{
				Code:    ErrValidation,
				Message: fmt.Sprintf("unknown filterable field: %s", fieldName),
				Details: map[string]any{"field": fieldName, "available": known},
			}
		}

		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}

		return Distinct(items, accessor), nil
	}

	s.OperationWithMetadata("distinct", handler, OperationMetadata{
		Description: "Returns unique values for a filterable field.",
		Parameters: []ParameterDef{
			{
				Name:        "field",
				Type:        "string",
				Optional:    false,
				Description: "Name of a registered filterable field.",
			},
		},
		Examples: []string{
			"distinct(status)",
			"distinct(assignee)",
		},
	})
}
