package agentquery

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// --- Test helpers ---

// testData returns a fixed set of test items for operation handlers.
func testData() []*testItem {
	return []*testItem{
		{ID: "T1", Name: "alpha", Status: "open", Score: 10, Tags: []string{"go"}},
		{ID: "T2", Name: "beta", Status: "closed", Score: 20, Tags: []string{"rust"}},
		{ID: "T3", Name: "gamma", Status: "open", Score: 30, Tags: nil},
	}
}

// newQuerySchema creates a Schema wired up with test data and standard operations.
func newQuerySchema() *Schema[*testItem] {
	s := newTestSchema() // fields + presets + defaults from selector_test.go

	items := testData()
	s.SetLoader(func() ([]*testItem, error) {
		return items, nil
	})

	// "list" — returns all items projected through the selector
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		var out []map[string]any
		for _, item := range data {
			out = append(out, ctx.Selector.Apply(item))
		}
		return out, nil
	})

	// "get" — returns a single item by first positional arg (ID match)
	s.Operation("get", func(ctx OperationContext[*testItem]) (any, error) {
		if len(ctx.Statement.Args) == 0 {
			return nil, &Error{Code: ErrValidation, Message: "get requires an ID argument"}
		}
		targetID := ctx.Statement.Args[0].Value
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		for _, item := range data {
			if item.ID == targetID {
				return ctx.Selector.Apply(item), nil
			}
		}
		return nil, &Error{
			Code:    ErrNotFound,
			Message: "item not found: " + targetID,
			Details: map[string]any{"id": targetID},
		}
	})

	// "count" — returns a count, ignores selector
	s.Operation("count", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(data)}, nil
	})

	// "fail" — always returns an error (for testing error handling)
	s.Operation("fail", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, &Error{Code: ErrInternal, Message: "intentional failure"}
	})

	return s
}

// --- Tests ---

func TestQuery_SingleOperation(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("get(T1)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Default fields: id, name, status
	if m["id"] != "T1" {
		t.Errorf("id = %v, want T1", m["id"])
	}
	if m["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", m["name"])
	}
	if m["status"] != "open" {
		t.Errorf("status = %v, want open", m["status"])
	}
	// Non-default fields should not be present
	if _, exists := m["score"]; exists {
		t.Error("score should not be in result (not in default fields)")
	}
}

func TestQuery_FieldProjection(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("get(T2) { id score }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if m["id"] != "T2" {
		t.Errorf("id = %v, want T2", m["id"])
	}
	if m["score"] != 20 {
		t.Errorf("score = %v, want 20", m["score"])
	}
	// Fields not in projection should be absent
	if _, exists := m["name"]; exists {
		t.Error("name should not be in result (not in projection)")
	}
	if _, exists := m["status"]; exists {
		t.Error("status should not be in result (not in projection)")
	}
}

func TestQuery_PresetInProjection(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("get(T1) { full }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Full preset: id, name, status, score, tags
	for _, field := range []string{"id", "name", "status", "score", "tags"} {
		if _, exists := m[field]; !exists {
			t.Errorf("missing field %q in result (full preset)", field)
		}
	}
}

func TestQuery_ListOperation(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("list() { id status }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Check first item
	if items[0]["id"] != "T1" {
		t.Errorf("items[0].id = %v, want T1", items[0]["id"])
	}
	if items[0]["status"] != "open" {
		t.Errorf("items[0].status = %v, want open", items[0]["status"])
	}
	// name should not be present (not in projection)
	if _, exists := items[0]["name"]; exists {
		t.Error("name should not be in result (not in projection)")
	}
}

func TestQuery_UnknownOperation(t *testing.T) {
	s := newQuerySchema()

	_, err := s.Query("bogus()")
	if err == nil {
		t.Fatal("expected error for unknown operation, got nil")
	}

	// Parser should reject unknown operation since we have a config
	var pe *ParseError
	if errors.As(err, &pe) {
		if !strings.Contains(pe.Message, "unknown operation") {
			t.Errorf("expected 'unknown operation' in error, got: %v", pe.Message)
		}
	} else {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
}

func TestQuery_UnknownField(t *testing.T) {
	s := newQuerySchema()

	_, err := s.Query("get(T1) { nonexistent }")
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}

	// Parser's FieldResolver should reject unknown fields
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' in error, got: %v", err)
	}
}

func TestQuery_HandlerError(t *testing.T) {
	s := newQuerySchema()

	// Single failing statement should return error directly
	_, err := s.Query("fail()")
	if err != nil {
		// The error from the handler is wrapped in the error map for single statements too.
		// Wait — actually for single statements, if the handler returns an error,
		// it becomes an error map in the results[0], and since len==1 we return results[0].
		// That means a single failing statement returns the error map, not an error.
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQuery_SingleFailReturnsErrorMap(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("fail()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected 'error' key in result map")
	}

	errMap, ok := errObj.(map[string]any)
	if !ok {
		t.Fatalf("expected error to be map[string]any, got %T", errObj)
	}

	msg, ok := errMap["message"].(string)
	if !ok || msg == "" {
		t.Errorf("expected non-empty error message, got %v", errMap["message"])
	}
	if !strings.Contains(msg, "intentional failure") {
		t.Errorf("error message = %q, want 'intentional failure'", msg)
	}
}

func TestQuery_BatchMultipleStatements(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("get(T1); get(T2); count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any for batch, got %T", result)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First: get(T1) -> map with id=T1
	m1, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] expected map[string]any, got %T", results[0])
	}
	if m1["id"] != "T1" {
		t.Errorf("results[0].id = %v, want T1", m1["id"])
	}

	// Second: get(T2) -> map with id=T2
	m2, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] expected map[string]any, got %T", results[1])
	}
	if m2["id"] != "T2" {
		t.Errorf("results[1].id = %v, want T2", m2["id"])
	}

	// Third: count() -> map with count=3
	m3, ok := results[2].(map[string]any)
	if !ok {
		t.Fatalf("results[2] expected map[string]any, got %T", results[2])
	}
	if m3["count"] != 3 {
		t.Errorf("results[2].count = %v, want 3", m3["count"])
	}
}

func TestQuery_BatchWithOneFailure(t *testing.T) {
	s := newQuerySchema()

	// Three statements: get(T1), fail(), get(T2)
	// The middle one fails but should not abort the batch
	result, err := s.Query("get(T1); fail(); get(T2)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any for batch, got %T", result)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First: success
	m1, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] expected map[string]any, got %T", results[0])
	}
	if m1["id"] != "T1" {
		t.Errorf("results[0].id = %v, want T1", m1["id"])
	}

	// Second: error map
	m2, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] expected map[string]any, got %T", results[1])
	}
	errObj, exists := m2["error"]
	if !exists {
		t.Fatal("results[1] should have 'error' key")
	}
	errMap, ok := errObj.(map[string]any)
	if !ok {
		t.Fatalf("error value expected map[string]any, got %T", errObj)
	}
	if !strings.Contains(errMap["message"].(string), "intentional failure") {
		t.Errorf("error message = %q, want 'intentional failure'", errMap["message"])
	}

	// Third: success (not aborted by second's failure)
	m3, ok := results[2].(map[string]any)
	if !ok {
		t.Fatalf("results[2] expected map[string]any, got %T", results[2])
	}
	if m3["id"] != "T2" {
		t.Errorf("results[2].id = %v, want T2", m3["id"])
	}
}

func TestQuery_SingleReturnUnwrapped(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single statement should NOT be wrapped in []any
	_, isSlice := result.([]any)
	if isSlice {
		t.Error("single statement result should not be wrapped in []any")
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["count"] != 3 {
		t.Errorf("count = %v, want 3", m["count"])
	}
}

func TestQueryJSON_Valid(t *testing.T) {
	s := newQuerySchema()

	data, err := s.QueryJSON("get(T1) { id name }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must be valid JSON
	if !json.Valid(data) {
		t.Fatalf("QueryJSON returned invalid JSON: %s", string(data))
	}

	// Unmarshal and check
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if m["id"] != "T1" {
		t.Errorf("id = %v, want T1", m["id"])
	}
	if m["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", m["name"])
	}
}

func TestQueryJSON_Batch(t *testing.T) {
	s := newQuerySchema()

	data, err := s.QueryJSON("get(T1); count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("QueryJSON returned invalid JSON: %s", string(data))
	}

	var results []any
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("failed to unmarshal as array: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestQueryJSON_ParseError(t *testing.T) {
	s := newQuerySchema()

	_, err := s.QueryJSON("") // empty query
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestQuery_ParseError(t *testing.T) {
	s := newQuerySchema()

	_, err := s.Query("")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
}

func TestQuery_NotFoundItem(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("get(NONEXISTENT)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single statement with handler error: returns error map
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected 'error' key in result for not-found item")
	}
	errMap, ok := errObj.(map[string]any)
	if !ok {
		t.Fatalf("error expected map[string]any, got %T", errObj)
	}
	if !strings.Contains(errMap["message"].(string), "not found") {
		t.Errorf("error message = %q, want 'not found'", errMap["message"])
	}
}

func TestSchema_ImplementsFieldResolver(t *testing.T) {
	s := newQuerySchema()

	// Test: known field returns itself
	resolved, err := s.ResolveField("id")
	if err != nil {
		t.Fatalf("unexpected error resolving 'id': %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "id" {
		t.Errorf("ResolveField('id') = %v, want [id]", resolved)
	}

	// Test: preset expands
	resolved, err = s.ResolveField("minimal")
	if err != nil {
		t.Fatalf("unexpected error resolving 'minimal': %v", err)
	}
	if len(resolved) != 2 || resolved[0] != "id" || resolved[1] != "status" {
		t.Errorf("ResolveField('minimal') = %v, want [id status]", resolved)
	}

	// Test: unknown returns error
	_, err = s.ResolveField("bogus")
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error = %v, want 'unknown field'", err)
	}
}

func TestSchema_ParserConfigHasOperations(t *testing.T) {
	s := newQuerySchema()

	config := s.parserConfig()
	if config == nil {
		t.Fatal("parserConfig() returned nil")
	}
	if config.Operations == nil {
		t.Fatal("parserConfig().Operations is nil")
	}

	// Should have all registered operations
	for _, op := range []string{"list", "get", "count", "fail"} {
		if !config.Operations[op] {
			t.Errorf("operation %q not in parser config", op)
		}
	}

	// Should not have unregistered operations
	if config.Operations["bogus"] {
		t.Error("bogus should not be in parser config operations")
	}

	if config.FieldResolver == nil {
		t.Error("parserConfig().FieldResolver should not be nil")
	}
}

func TestQuery_EmptySchemaNoOperations(t *testing.T) {
	s := NewSchema[*testItem]()

	// No operations registered — any query should fail at parse time
	_, err := s.Query("get(T1)")
	if err == nil {
		t.Fatal("expected error for empty schema (no operations), got nil")
	}

	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
}

func TestQuery_OperationRegistration(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	called := false
	s.Operation("ping", func(ctx OperationContext[*testItem]) (any, error) {
		called = true
		return map[string]any{"pong": true}, nil
	})

	result, err := s.Query("ping()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Error("operation handler was not called")
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["pong"] != true {
		t.Errorf("pong = %v, want true", m["pong"])
	}
}

func TestQuery_SelectorPassedToHandler(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })

	var receivedFields []string
	s.Operation("inspect", func(ctx OperationContext[*testItem]) (any, error) {
		receivedFields = ctx.Selector.Fields()
		return "ok", nil
	})

	_, err := s.Query("inspect() { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(receivedFields) != 1 || receivedFields[0] != "id" {
		t.Errorf("handler received fields %v, want [id]", receivedFields)
	}
}

func TestQuery_StatementPassedToHandler(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	var receivedStmt Statement
	s.Operation("echo", func(ctx OperationContext[*testItem]) (any, error) {
		receivedStmt = ctx.Statement
		return "ok", nil
	})

	_, err := s.Query("echo(hello, key=val)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedStmt.Operation != "echo" {
		t.Errorf("stmt.Operation = %q, want echo", receivedStmt.Operation)
	}
	if len(receivedStmt.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(receivedStmt.Args))
	}
	if receivedStmt.Args[0].Value != "hello" {
		t.Errorf("args[0].Value = %q, want hello", receivedStmt.Args[0].Value)
	}
	if receivedStmt.Args[1].Key != "key" || receivedStmt.Args[1].Value != "val" {
		t.Errorf("args[1] = {%q, %q}, want key=val", receivedStmt.Args[1].Key, receivedStmt.Args[1].Value)
	}
}

func TestQuery_ItemsLoaderPassed(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	expectedItems := []*testItem{{ID: "X1"}, {ID: "X2"}}
	s.SetLoader(func() ([]*testItem, error) {
		return expectedItems, nil
	})

	s.Operation("check-items", func(ctx OperationContext[*testItem]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(items)}, nil
	})

	result, err := s.Query("check-items()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

func TestQuery_NilLoaderPassedAsItems(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	// No SetLoader call — loader is nil

	var itemsFn func() ([]*testItem, error)
	s.Operation("check-nil", func(ctx OperationContext[*testItem]) (any, error) {
		itemsFn = ctx.Items
		return "ok", nil
	})

	_, err := s.Query("check-nil()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Items should be nil (no loader set)
	if itemsFn != nil {
		t.Error("Items function should be nil when no loader is set")
	}
}

func TestQuery_DefaultFieldsUsedWhenNoProjection(t *testing.T) {
	s := newQuerySchema()

	var receivedFields []string
	s.Operation("check-defaults", func(ctx OperationContext[*testItem]) (any, error) {
		receivedFields = ctx.Selector.Fields()
		return "ok", nil
	})

	_, err := s.Query("check-defaults()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Schema has DefaultFields("id", "name", "status")
	expected := []string{"id", "name", "status"}
	if len(receivedFields) != len(expected) {
		t.Fatalf("fields = %v, want %v", receivedFields, expected)
	}
	for i, f := range expected {
		if receivedFields[i] != f {
			t.Errorf("field[%d] = %q, want %q", i, receivedFields[i], f)
		}
	}
}

func TestQuery_BatchTwoStatements(t *testing.T) {
	s := newQuerySchema()

	result, err := s.Query("get(T1) { id }; count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any for batch, got %T", result)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestQuery_TableDriven(t *testing.T) {
	s := newQuerySchema()

	tests := []struct {
		name    string
		input   string
		wantErr string // error from Parse (before dispatch)
		checkFn func(t *testing.T, result any)
	}{
		{
			name:  "simple get",
			input: "get(T1)",
			checkFn: func(t *testing.T, result any) {
				m := result.(map[string]any)
				if m["id"] != "T1" {
					t.Errorf("id = %v, want T1", m["id"])
				}
			},
		},
		{
			name:  "get with projection",
			input: "get(T1) { id score }",
			checkFn: func(t *testing.T, result any) {
				m := result.(map[string]any)
				if m["id"] != "T1" {
					t.Errorf("id = %v, want T1", m["id"])
				}
				if m["score"] != 10 {
					t.Errorf("score = %v, want 10", m["score"])
				}
				if _, exists := m["name"]; exists {
					t.Error("name should not be present")
				}
			},
		},
		{
			name:  "count operation",
			input: "count()",
			checkFn: func(t *testing.T, result any) {
				m := result.(map[string]any)
				if m["count"] != 3 {
					t.Errorf("count = %v, want 3", m["count"])
				}
			},
		},
		{
			name:  "list all",
			input: "list() { id }",
			checkFn: func(t *testing.T, result any) {
				items := result.([]map[string]any)
				if len(items) != 3 {
					t.Errorf("len = %d, want 3", len(items))
				}
			},
		},
		{
			name:    "parse error - empty",
			input:   "",
			wantErr: "empty query",
		},
		{
			name:    "parse error - unknown op",
			input:   "nope()",
			wantErr: "unknown operation",
		},
		{
			name:    "parse error - unknown field",
			input:   "get(T1) { nonexistent }",
			wantErr: "unknown field",
		},
		{
			name:    "parse error - syntax",
			input:   "get T1",
			wantErr: "expected '('",
		},
		{
			name:  "batch: two gets",
			input: "get(T1) { id }; get(T2) { id }",
			checkFn: func(t *testing.T, result any) {
				results := result.([]any)
				if len(results) != 2 {
					t.Fatalf("expected 2 results, got %d", len(results))
				}
				m1 := results[0].(map[string]any)
				m2 := results[1].(map[string]any)
				if m1["id"] != "T1" {
					t.Errorf("results[0].id = %v, want T1", m1["id"])
				}
				if m2["id"] != "T2" {
					t.Errorf("results[1].id = %v, want T2", m2["id"])
				}
			},
		},
		{
			name:  "batch: one fail in middle",
			input: "count(); fail(); get(T3)",
			checkFn: func(t *testing.T, result any) {
				results := result.([]any)
				if len(results) != 3 {
					t.Fatalf("expected 3 results, got %d", len(results))
				}
				// First: count
				m1 := results[0].(map[string]any)
				if m1["count"] != 3 {
					t.Errorf("results[0].count = %v, want 3", m1["count"])
				}
				// Second: error
				m2 := results[1].(map[string]any)
				if _, exists := m2["error"]; !exists {
					t.Error("results[1] should have 'error' key")
				}
				// Third: get(T3) succeeds
				m3 := results[2].(map[string]any)
				if m3["id"] != "T3" {
					t.Errorf("results[2].id = %v, want T3", m3["id"])
				}
			},
		},
		{
			name:  "single fail returns error map not error",
			input: "fail()",
			checkFn: func(t *testing.T, result any) {
				m := result.(map[string]any)
				if _, exists := m["error"]; !exists {
					t.Error("expected 'error' key in result")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := s.Query(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

func TestQuery_SetLoaderOverwrite(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	s.SetLoader(func() ([]*testItem, error) {
		return []*testItem{{ID: "OLD"}}, nil
	})

	s.SetLoader(func() ([]*testItem, error) {
		return []*testItem{{ID: "NEW"}}, nil
	})

	s.Operation("first", func(ctx OperationContext[*testItem]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return ctx.Selector.Apply(items[0]), nil
	})

	result, err := s.Query("first()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["id"] != "NEW" {
		t.Errorf("id = %v, want NEW (loader should be overwritten)", m["id"])
	}
}

func TestQuery_OperationOverwrite(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	s.Operation("op", func(ctx OperationContext[*testItem]) (any, error) {
		return "v1", nil
	})
	s.Operation("op", func(ctx OperationContext[*testItem]) (any, error) {
		return "v2", nil
	})

	result, err := s.Query("op()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "v2" {
		t.Errorf("result = %v, want v2 (operation should be overwritten)", result)
	}
}

// --- LLMReadable output mode tests ---

// newLLMQuerySchema creates a Schema with LLMReadable output mode.
func newLLMQuerySchema() *Schema[*testItem] {
	s := NewSchema[*testItem](WithOutputMode(LLMReadable))
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })
	s.Field("status", func(item *testItem) any { return item.Status })
	s.Field("score", func(item *testItem) any { return item.Score })
	s.Field("tags", func(item *testItem) any { return item.Tags })

	s.Preset("minimal", "id", "status")
	s.Preset("default", "id", "name", "status")
	s.Preset("full", "id", "name", "status", "score", "tags")

	s.DefaultFields("id", "name", "status")

	items := testData()
	s.SetLoader(func() ([]*testItem, error) {
		return items, nil
	})

	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		var out []map[string]any
		for _, item := range data {
			out = append(out, ctx.Selector.Apply(item))
		}
		return out, nil
	})

	s.Operation("get", func(ctx OperationContext[*testItem]) (any, error) {
		if len(ctx.Statement.Args) == 0 {
			return nil, &Error{Code: ErrValidation, Message: "get requires an ID argument"}
		}
		targetID := ctx.Statement.Args[0].Value
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		for _, item := range data {
			if item.ID == targetID {
				return ctx.Selector.Apply(item), nil
			}
		}
		return nil, &Error{
			Code:    ErrNotFound,
			Message: "item not found: " + targetID,
			Details: map[string]any{"id": targetID},
		}
	})

	s.Operation("count", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(data)}, nil
	})

	s.Operation("fail", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, &Error{Code: ErrInternal, Message: "intentional failure"}
	})

	return s
}

func TestQueryJSON_LLMReadable_List(t *testing.T) {
	s := newLLMQuerySchema()

	data, err := s.QueryJSON("list() { id status }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	// Header + 3 data rows
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "id,status" {
		t.Errorf("header = %q, want %q", lines[0], "id,status")
	}
	if lines[1] != "T1,open" {
		t.Errorf("row 1 = %q, want %q", lines[1], "T1,open")
	}
	if lines[2] != "T2,closed" {
		t.Errorf("row 2 = %q, want %q", lines[2], "T2,closed")
	}
	if lines[3] != "T3,open" {
		t.Errorf("row 3 = %q, want %q", lines[3], "T3,open")
	}

	// Should NOT be valid JSON (it's tabular).
	if json.Valid(data) {
		t.Error("LLMReadable list output should not be valid JSON")
	}
}

func TestQueryJSON_LLMReadable_Get(t *testing.T) {
	s := newLLMQuerySchema()

	data, err := s.QueryJSON("get(T1) { id name }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "id:T1" {
		t.Errorf("line 0 = %q, want %q", lines[0], "id:T1")
	}
	if lines[1] != "name:alpha" {
		t.Errorf("line 1 = %q, want %q", lines[1], "name:alpha")
	}
}

func TestQueryJSON_LLMReadable_Count(t *testing.T) {
	s := newLLMQuerySchema()

	// count() returns map[string]any{"count": 3} — the "count" field is not
	// a schema-registered field, so field order comes from the map keys.
	data, err := s.QueryJSON("count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "count:3" {
		t.Errorf("line = %q, want %q", lines[0], "count:3")
	}
}

func TestQueryJSON_LLMReadable_DefaultFields(t *testing.T) {
	s := newLLMQuerySchema()

	// No explicit projection — uses default fields (id, name, status).
	data, err := s.QueryJSON("get(T2)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (id, name, status), got %d:\n%s", len(lines), out)
	}
	if lines[0] != "id:T2" {
		t.Errorf("line 0 = %q, want %q", lines[0], "id:T2")
	}
	if lines[1] != "name:beta" {
		t.Errorf("line 1 = %q, want %q", lines[1], "name:beta")
	}
	if lines[2] != "status:closed" {
		t.Errorf("line 2 = %q, want %q", lines[2], "status:closed")
	}
}

func TestQueryJSON_LLMReadable_PresetExpansion(t *testing.T) {
	s := newLLMQuerySchema()

	data, err := s.QueryJSON("list() { minimal }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	// minimal = id, status
	if lines[0] != "id,status" {
		t.Errorf("header = %q, want %q", lines[0], "id,status")
	}
}

func TestQueryJSON_LLMReadable_ErrorFallsBackToJSON(t *testing.T) {
	s := newLLMQuerySchema()

	// A failing operation produces an error map — should fall back to JSON.
	data, err := s.QueryJSON("fail()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("error result should be valid JSON, got: %s", string(data))
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, hasError := m["error"]; !hasError {
		t.Error("expected 'error' key in JSON output")
	}
}

func TestQueryJSON_LLMReadable_Batch(t *testing.T) {
	s := newLLMQuerySchema()

	data, err := s.QueryJSON("get(T1) { id }; list() { id status }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	// Batch: two parts separated by a blank line.
	parts := strings.Split(out, "\n\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts separated by blank line, got %d:\n%s", len(parts), out)
	}

	// First part: get(T1) { id } — single object.
	part1Lines := strings.Split(strings.TrimRight(parts[0], "\n"), "\n")
	if len(part1Lines) != 1 {
		t.Fatalf("part 1: expected 1 line, got %d: %v", len(part1Lines), part1Lines)
	}
	if part1Lines[0] != "id:T1" {
		t.Errorf("part 1 line = %q, want %q", part1Lines[0], "id:T1")
	}

	// Second part: list() { id status } — tabular.
	part2Lines := strings.Split(strings.TrimRight(parts[1], "\n"), "\n")
	if len(part2Lines) != 4 {
		t.Fatalf("part 2: expected 4 lines, got %d: %v", len(part2Lines), part2Lines)
	}
	if part2Lines[0] != "id,status" {
		t.Errorf("part 2 header = %q, want %q", part2Lines[0], "id,status")
	}
}

func TestQueryJSON_LLMReadable_BatchWithError(t *testing.T) {
	s := newLLMQuerySchema()

	data, err := s.QueryJSON("get(T1) { id }; fail(); list() { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	// Three parts separated by blank lines.
	parts := strings.Split(out, "\n\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d:\n%s", len(parts), out)
	}

	// Middle part (fail()) should be valid JSON with error key.
	if !json.Valid([]byte(parts[1])) {
		t.Fatalf("error part should be valid JSON, got: %s", parts[1])
	}
}

func TestQueryJSON_HumanReadable_StillJSON(t *testing.T) {
	// Verify that the default mode (HumanReadable) still produces JSON.
	s := newQuerySchema() // default mode = HumanReadable

	data, err := s.QueryJSON("list() { id status }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("HumanReadable QueryJSON should produce valid JSON, got: %s", string(data))
	}
}

func TestQueryJSON_LLMReadable_ParseError(t *testing.T) {
	s := newLLMQuerySchema()

	_, err := s.QueryJSON("")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestOutputMode_Getter(t *testing.T) {
	s1 := NewSchema[*testItem]()
	if s1.OutputMode() != HumanReadable {
		t.Errorf("default OutputMode = %v, want HumanReadable", s1.OutputMode())
	}

	s2 := NewSchema[*testItem](WithOutputMode(LLMReadable))
	if s2.OutputMode() != LLMReadable {
		t.Errorf("OutputMode = %v, want LLMReadable", s2.OutputMode())
	}
}

// --- ApplyValues tests ---

func TestApplyValues_Basic(t *testing.T) {
	s := newTestSchema()
	item := &testItem{
		ID:     "TASK-42",
		Name:   "implement feature",
		Status: "development",
		Score:  95,
		Tags:   []string{"go", "generics"},
	}

	sel, err := s.newSelector([]string{"id", "name", "score"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values := sel.ApplyValues(item)
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(values))
	}
	if values[0] != "TASK-42" {
		t.Errorf("values[0] = %v, want TASK-42", values[0])
	}
	if values[1] != "implement feature" {
		t.Errorf("values[1] = %v, want 'implement feature'", values[1])
	}
	if values[2] != 95 {
		t.Errorf("values[2] = %v, want 95", values[2])
	}
}

func TestApplyValues_MatchesFieldsOrder(t *testing.T) {
	s := newTestSchema()
	item := &testItem{ID: "T1", Name: "alpha", Status: "open"}

	sel, err := s.newSelector([]string{"status", "id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := sel.Fields()
	values := sel.ApplyValues(item)

	if len(values) != len(fields) {
		t.Fatalf("values length %d != fields length %d", len(values), len(fields))
	}

	// Values should match field order: status first, then id.
	if fields[0] != "status" || values[0] != "open" {
		t.Errorf("field[0]=%q val[0]=%v, want status=open", fields[0], values[0])
	}
	if fields[1] != "id" || values[1] != "T1" {
		t.Errorf("field[1]=%q val[1]=%v, want id=T1", fields[1], values[1])
	}
}

func TestApplyValues_NilSliceField(t *testing.T) {
	s := newTestSchema()
	item := &testItem{ID: "T1", Tags: nil}

	sel, err := s.newSelector([]string{"id", "tags"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values := sel.ApplyValues(item)
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "T1" {
		t.Errorf("values[0] = %v, want T1", values[0])
	}
	// The accessor returns []string(nil) which is a typed nil.
	tags, ok := values[1].([]string)
	if !ok {
		t.Fatalf("values[1] type = %T, want []string", values[1])
	}
	if tags != nil {
		t.Errorf("values[1] = %v, want nil", values[1])
	}
}
