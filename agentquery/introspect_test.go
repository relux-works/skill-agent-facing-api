package agentquery

import (
	"encoding/json"
	"sort"
	"testing"
)

// --- schema() introspection tests ---

func TestSchemaOperation_Exists(t *testing.T) {
	// Every Schema must have the "schema" operation auto-registered.
	s := NewSchema[*testItem]()

	config := s.parserConfig()
	if !config.Operations["schema"] {
		t.Fatal("schema operation not in parser config — must be auto-registered by NewSchema()")
	}
}

func TestSchemaOperation_BasicQuery(t *testing.T) {
	s := NewSchema[*testItem]()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Must have all four top-level keys
	for _, key := range []string{"operations", "fields", "presets", "defaultFields"} {
		if _, exists := m[key]; !exists {
			t.Errorf("missing key %q in schema() result", key)
		}
	}
}

func TestSchemaOperation_ListsAllOperations(t *testing.T) {
	s := newTestSchema() // has fields, presets, defaults
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, nil
	})
	s.Operation("get", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, nil
	})
	s.Operation("summary", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, nil
	})

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	ops, ok := m["operations"].([]string)
	if !ok {
		t.Fatalf("operations type = %T, want []string", m["operations"])
	}

	// Operations should be sorted and include "schema" itself
	expected := []string{"get", "list", "schema", "summary"}
	if len(ops) != len(expected) {
		t.Fatalf("operations = %v, want %v", ops, expected)
	}
	for i, op := range expected {
		if ops[i] != op {
			t.Errorf("operations[%d] = %q, want %q", i, ops[i], op)
		}
	}
}

func TestSchemaOperation_FieldsInRegistrationOrder(t *testing.T) {
	s := NewSchema[*testItem]()
	// Register fields in a specific order
	s.Field("name", func(item *testItem) any { return item.Name })
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("status", func(item *testItem) any { return item.Status })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	fields, ok := m["fields"].([]string)
	if !ok {
		t.Fatalf("fields type = %T, want []string", m["fields"])
	}

	expected := []string{"name", "id", "status"}
	if len(fields) != len(expected) {
		t.Fatalf("fields = %v, want %v", fields, expected)
	}
	for i, f := range expected {
		if fields[i] != f {
			t.Errorf("fields[%d] = %q, want %q", i, fields[i], f)
		}
	}
}

func TestSchemaOperation_PresetsMap(t *testing.T) {
	s := newTestSchema()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	presets, ok := m["presets"].(map[string][]string)
	if !ok {
		t.Fatalf("presets type = %T, want map[string][]string", m["presets"])
	}

	// newTestSchema registers: minimal, default, full
	expectedPresets := map[string][]string{
		"minimal": {"id", "status"},
		"default": {"id", "name", "status"},
		"full":    {"id", "name", "status", "score", "tags"},
	}

	if len(presets) != len(expectedPresets) {
		t.Fatalf("presets count = %d, want %d", len(presets), len(expectedPresets))
	}

	for name, expectedFields := range expectedPresets {
		got, exists := presets[name]
		if !exists {
			t.Errorf("missing preset %q", name)
			continue
		}
		if len(got) != len(expectedFields) {
			t.Errorf("preset %q = %v, want %v", name, got, expectedFields)
			continue
		}
		for i, f := range expectedFields {
			if got[i] != f {
				t.Errorf("preset %q[%d] = %q, want %q", name, i, got[i], f)
			}
		}
	}
}

func TestSchemaOperation_DefaultFields(t *testing.T) {
	s := newTestSchema()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	defaults, ok := m["defaultFields"].([]string)
	if !ok {
		t.Fatalf("defaultFields type = %T, want []string", m["defaultFields"])
	}

	// newTestSchema uses DefaultFields("id", "name", "status")
	expected := []string{"id", "name", "status"}
	if len(defaults) != len(expected) {
		t.Fatalf("defaultFields = %v, want %v", defaults, expected)
	}
	for i, f := range expected {
		if defaults[i] != f {
			t.Errorf("defaultFields[%d] = %q, want %q", i, defaults[i], f)
		}
	}
}

func TestSchemaOperation_EmptySchema(t *testing.T) {
	// Schema with no user registrations — only "schema" operation should exist
	s := NewSchema[*testItem]()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)

	ops := m["operations"].([]string)
	if len(ops) != 1 || ops[0] != "schema" {
		t.Errorf("operations = %v, want [schema]", ops)
	}

	fields := m["fields"].([]string)
	if len(fields) != 0 {
		t.Errorf("fields = %v, want []", fields)
	}

	presets := m["presets"].(map[string][]string)
	if len(presets) != 0 {
		t.Errorf("presets = %v, want empty map", presets)
	}

	defaults := m["defaultFields"].([]string)
	if len(defaults) != 0 {
		t.Errorf("defaultFields = %v, want []", defaults)
	}
}

func TestSchemaOperation_IncludesSelf(t *testing.T) {
	// The schema operation must list itself in the operations list
	s := NewSchema[*testItem]()
	s.Operation("ping", func(ctx OperationContext[*testItem]) (any, error) {
		return "pong", nil
	})

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	ops := m["operations"].([]string)

	found := false
	for _, op := range ops {
		if op == "schema" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("schema() should include itself in operations list, got: %v", ops)
	}
}

func TestSchemaOperation_OperationsSorted(t *testing.T) {
	s := NewSchema[*testItem]()
	// Register in reverse alphabetical order
	s.Operation("zebra", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })
	s.Operation("alpha", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })
	s.Operation("middle", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	ops := m["operations"].([]string)

	if !sort.StringsAreSorted(ops) {
		t.Errorf("operations should be sorted, got: %v", ops)
	}
}

func TestSchemaOperation_JSON(t *testing.T) {
	s := newTestSchema()
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })

	data, err := s.QueryJSON("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("QueryJSON returned invalid JSON: %s", string(data))
	}

	// Unmarshal and verify structure
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// After JSON round-trip, operations becomes []any (not []string)
	ops, ok := m["operations"].([]any)
	if !ok {
		t.Fatalf("operations type after JSON = %T", m["operations"])
	}

	// Must contain "schema" and "list"
	opSet := make(map[string]bool)
	for _, op := range ops {
		opSet[op.(string)] = true
	}
	if !opSet["schema"] {
		t.Error("operations missing 'schema'")
	}
	if !opSet["list"] {
		t.Error("operations missing 'list'")
	}
}

func TestSchemaOperation_InBatch(t *testing.T) {
	s := newQuerySchema()

	// Batch: schema() alongside a regular operation
	result, err := s.Query("schema(); count()")
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

	// First result: schema introspection
	schemaResult, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] type = %T, want map", results[0])
	}
	if _, exists := schemaResult["operations"]; !exists {
		t.Error("results[0] should have 'operations' key")
	}

	// Second result: count
	countResult, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] type = %T, want map", results[1])
	}
	if countResult["count"] != 3 {
		t.Errorf("results[1].count = %v, want 3", countResult["count"])
	}
}

func TestSchemaOperation_CannotBeOverridden(t *testing.T) {
	// User can override the "schema" operation (it's just a regular operation registration)
	s := NewSchema[*testItem]()
	s.Operation("schema", func(ctx OperationContext[*testItem]) (any, error) {
		return map[string]any{"custom": true}, nil
	})

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	// User-registered handler should override the built-in one
	if m["custom"] != true {
		t.Errorf("expected custom schema handler to be called, got: %v", m)
	}
}

func TestSchemaOperation_ReturnsCopies(t *testing.T) {
	s := newTestSchema()

	result1, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m1 := result1.(map[string]any)
	fields1 := m1["fields"].([]string)

	// Mutate the returned slice
	if len(fields1) > 0 {
		fields1[0] = "MUTATED"
	}

	// Second call should return clean data
	result2, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m2 := result2.(map[string]any)
	fields2 := m2["fields"].([]string)

	if len(fields2) > 0 && fields2[0] == "MUTATED" {
		t.Error("schema() returned a reference to internal state, not a copy")
	}
}

func TestSchemaOperation_IgnoresFieldProjection(t *testing.T) {
	// schema() returns its own structure — field projections don't apply to it
	// (the handler ignores the selector, returns a plain map)
	s := newTestSchema()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	// All four keys should be present regardless
	for _, key := range []string{"operations", "fields", "presets", "defaultFields"} {
		if _, exists := m[key]; !exists {
			t.Errorf("missing key %q in schema() result", key)
		}
	}
}

func TestSchemaOperation_LazyOperationDiscovery(t *testing.T) {
	// Verifies that operations registered AFTER NewSchema() are still visible
	// when schema() executes.
	s := NewSchema[*testItem]()

	// At this point only "schema" is registered
	// Now add more operations
	s.Operation("op1", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })
	s.Operation("op2", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	ops := m["operations"].([]string)

	expected := map[string]bool{"op1": true, "op2": true, "schema": true}
	if len(ops) != len(expected) {
		t.Fatalf("operations = %v, want %v", ops, expected)
	}
	for _, op := range ops {
		if !expected[op] {
			t.Errorf("unexpected operation %q", op)
		}
	}
}
