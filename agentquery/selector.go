package agentquery

import "fmt"

// FieldSelector controls which fields are included in query results.
// It is created internally by the Schema from parsed field projections
// and passed to operation handlers.
type FieldSelector[T any] struct {
	fields    map[string]bool
	accessors map[string]FieldAccessor[T]
	ordered   []string // selected field names in deterministic order
}

// Apply extracts the selected fields from a domain item, returning a map
// suitable for JSON serialization. Only accessors for selected fields are called
// (lazy evaluation).
func (fs *FieldSelector[T]) Apply(item T) map[string]any {
	result := make(map[string]any, len(fs.ordered))
	for _, name := range fs.ordered {
		if accessor, ok := fs.accessors[name]; ok {
			result[name] = accessor(item)
		}
	}
	return result
}

// Include reports whether the named field is in the current selection.
func (fs *FieldSelector[T]) Include(field string) bool {
	return fs.fields[field]
}

// Fields returns the list of selected field names in deterministic order.
func (fs *FieldSelector[T]) Fields() []string {
	out := make([]string, len(fs.ordered))
	copy(out, fs.ordered)
	return out
}

// newSelector creates a FieldSelector from a list of requested field names.
// Preset names are expanded inline. Unknown fields produce a validation error.
// If requested is empty, the schema's default fields are used.
func (s *Schema[T]) newSelector(requested []string) (*FieldSelector[T], error) {
	// Use defaults when no projection specified
	if len(requested) == 0 {
		requested = s.defaultFields
	}

	// If still empty after defaults (no defaults configured), select all fields
	if len(requested) == 0 {
		requested = s.fieldOrder
	}

	// Expand presets and validate
	var expanded []string
	for _, name := range requested {
		if presetFields, ok := s.presets[name]; ok {
			expanded = append(expanded, presetFields...)
			continue
		}
		if _, ok := s.fields[name]; !ok {
			return nil, &Error{
				Code:    ErrValidation,
				Message: fmt.Sprintf("unknown field: %s", name),
				Details: map[string]any{"field": name},
			}
		}
		expanded = append(expanded, name)
	}

	// Deduplicate while preserving order
	seen := make(map[string]bool, len(expanded))
	var ordered []string
	fields := make(map[string]bool, len(expanded))
	for _, name := range expanded {
		if !seen[name] {
			seen[name] = true
			ordered = append(ordered, name)
			fields[name] = true
		}
	}

	// Build accessor map for selected fields only
	accessors := make(map[string]FieldAccessor[T], len(ordered))
	for _, name := range ordered {
		if acc, ok := s.fields[name]; ok {
			accessors[name] = acc
		}
	}

	return &FieldSelector[T]{
		fields:    fields,
		accessors: accessors,
		ordered:   ordered,
	}, nil
}
