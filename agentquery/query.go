package agentquery

import (
	"bytes"
	"encoding/json"
)

// Query parses and executes the input query string against the schema.
// For a single statement, it returns the handler's result directly.
// For multiple semicolon-separated statements (batch), it returns []any
// where each element is either the handler's result or an error map.
// Per-statement errors do not abort the batch — remaining statements
// continue executing.
func (s *Schema[T]) Query(input string) (any, error) {
	q, err := Parse(input, s.parserConfig())
	if err != nil {
		return nil, err
	}

	results := make([]any, 0, len(q.Statements))
	for _, stmt := range q.Statements {
		result, execErr := s.executeStatement(stmt)
		if execErr != nil {
			results = append(results, map[string]any{
				"error": map[string]any{
					"message": execErr.Error(),
				},
			})
			continue
		}
		results = append(results, result)
	}

	// Single statement: return unwrapped result
	if len(results) == 1 {
		return results[0], nil
	}

	return results, nil
}

// QueryJSON is a convenience method that executes the query and marshals
// the result to JSON bytes.
func (s *Schema[T]) QueryJSON(input string) ([]byte, error) {
	result, err := s.Query(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// formatLLMReadable formats query results in compact tabular format.
// For batch queries, each statement result is formatted individually
// and separated by a blank line.
func (s *Schema[T]) formatLLMReadable(input string, result any) ([]byte, error) {
	// Re-parse to extract field order from the query.
	// This is cheap (no execution) and gives us the explicit field projection.
	q, _ := Parse(input, s.parserConfig())

	// Single statement: format with field order from the first (only) statement.
	if q != nil && len(q.Statements) == 1 {
		fieldOrder := s.fieldOrderFromStatement(q.Statements[0])
		return FormatCompact(result, fieldOrder)
	}

	// Batch: format each result separately, join with blank line.
	results, ok := result.([]any)
	if !ok {
		// Shouldn't happen for multi-statement, but be safe.
		return FormatCompact(result, nil)
	}

	var parts [][]byte
	for i, r := range results {
		var fieldOrder []string
		if q != nil && i < len(q.Statements) {
			fieldOrder = s.fieldOrderFromStatement(q.Statements[i])
		}
		part, err := FormatCompact(r, fieldOrder)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}

	// Join parts with blank lines.
	// Each part is normalized to end with exactly one newline before the separator.
	var combined []byte
	for i, part := range parts {
		if i > 0 {
			combined = append(combined, '\n')
		}
		// Ensure part ends with a newline for consistent blank-line separation.
		trimmed := bytes.TrimRight(part, "\n")
		combined = append(combined, trimmed...)
		combined = append(combined, '\n')
	}
	return combined, nil
}

// fieldOrderFromStatement extracts field names for a statement by building
// a selector from the statement's field projection.
func (s *Schema[T]) fieldOrderFromStatement(stmt Statement) []string {
	sel, err := s.newSelector(stmt.Fields)
	if err != nil {
		return nil
	}
	return sel.Fields()
}

// executeStatement dispatches a single parsed statement to its registered handler.
func (s *Schema[T]) executeStatement(stmt Statement) (any, error) {
	handler, ok := s.operations[stmt.Operation]
	if !ok {
		return nil, &Error{
			Code:    ErrNotFound,
			Message: "unknown operation: " + stmt.Operation,
			Details: map[string]any{"operation": stmt.Operation},
		}
	}

	selector, err := s.newSelector(stmt.Fields)
	if err != nil {
		return nil, err
	}

	ctx := OperationContext[T]{
		Statement: stmt,
		Selector:  selector,
		Items:     s.loader,
		Predicate: s.buildPredicate(stmt.Args),
	}

	return handler(ctx)
}
