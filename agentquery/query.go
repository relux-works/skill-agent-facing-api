package agentquery

import "encoding/json"

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
	}

	return handler(ctx)
}
