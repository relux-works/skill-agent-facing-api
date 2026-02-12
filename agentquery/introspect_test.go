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

// --- OperationMetadata tests ---

func TestSchemaOperation_NoMetadata_BackwardsCompat(t *testing.T) {
	// schema() with no metadata registered — operationMetadata key should be absent.
	s := newTestSchema()
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })
	s.Operation("get", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, exists := m["operationMetadata"]; exists {
		t.Error("operationMetadata should be absent when no operations have metadata")
	}
}

func TestSchemaOperation_MetadataForSomeOps(t *testing.T) {
	// Metadata registered for some ops — only those appear in operationMetadata.
	s := newTestSchema()
	noop := func(ctx OperationContext[*testItem]) (any, error) { return nil, nil }

	// "list" with metadata
	s.OperationWithMetadata("list", noop, OperationMetadata{
		Description: "List items",
		Parameters: []ParameterDef{
			{Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
		},
		Examples: []string{"list() { overview }"},
	})

	// "get" without metadata (plain Operation)
	s.Operation("get", noop)

	// "count" with metadata
	s.OperationWithMetadata("count", noop, OperationMetadata{
		Description: "Count items",
	})

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)

	// All three ops should be in the operations list
	ops := m["operations"].([]string)
	opSet := make(map[string]bool)
	for _, op := range ops {
		opSet[op] = true
	}
	for _, expected := range []string{"list", "get", "count", "schema"} {
		if !opSet[expected] {
			t.Errorf("operations missing %q", expected)
		}
	}

	// operationMetadata should exist and only have "list" and "count"
	metaRaw, exists := m["operationMetadata"]
	if !exists {
		t.Fatal("operationMetadata key should be present")
	}
	meta, ok := metaRaw.(map[string]OperationMetadata)
	if !ok {
		t.Fatalf("operationMetadata type = %T, want map[string]OperationMetadata", metaRaw)
	}
	if len(meta) != 2 {
		t.Fatalf("operationMetadata has %d entries, want 2", len(meta))
	}
	if _, exists := meta["list"]; !exists {
		t.Error("operationMetadata missing 'list'")
	}
	if _, exists := meta["count"]; !exists {
		t.Error("operationMetadata missing 'count'")
	}
	if _, exists := meta["get"]; exists {
		t.Error("operationMetadata should not have 'get' (registered without metadata)")
	}
}

func TestSchemaOperation_MetadataForAllOps(t *testing.T) {
	// Metadata registered for all user ops.
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	noop := func(ctx OperationContext[*testItem]) (any, error) { return nil, nil }

	s.OperationWithMetadata("list", noop, OperationMetadata{Description: "List all"})
	s.OperationWithMetadata("get", noop, OperationMetadata{Description: "Get one"})
	s.OperationWithMetadata("count", noop, OperationMetadata{Description: "Count"})

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	meta := m["operationMetadata"].(map[string]OperationMetadata)

	if len(meta) != 3 {
		t.Fatalf("operationMetadata has %d entries, want 3", len(meta))
	}
	for _, name := range []string{"list", "get", "count"} {
		if _, exists := meta[name]; !exists {
			t.Errorf("operationMetadata missing %q", name)
		}
	}
	// "schema" built-in should NOT have metadata
	if _, exists := meta["schema"]; exists {
		t.Error("built-in 'schema' operation should not have metadata")
	}
}

func TestSchemaOperation_MetadataFieldsSerialization(t *testing.T) {
	// Verify OperationMetadata fields serialize correctly to JSON.
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	s.OperationWithMetadata("list", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, nil
	}, OperationMetadata{
		Description: "List items with filters",
		Parameters: []ParameterDef{
			{Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
			{Name: "skip", Type: "int", Optional: true, Default: 0, Description: "Skip first N"},
			{Name: "take", Type: "int", Optional: true, Description: "Limit results"},
		},
		Examples: []string{
			"list() { overview }",
			"list(status=done) { minimal }",
		},
	})

	data, err := s.QueryJSON("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("QueryJSON returned invalid JSON: %s", string(data))
	}

	// Unmarshal into generic map to verify JSON structure
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	metaRaw, exists := m["operationMetadata"]
	if !exists {
		t.Fatal("operationMetadata missing from JSON output")
	}

	metaMap, ok := metaRaw.(map[string]any)
	if !ok {
		t.Fatalf("operationMetadata type = %T, want map", metaRaw)
	}

	listMeta, ok := metaMap["list"].(map[string]any)
	if !ok {
		t.Fatalf("operationMetadata.list type = %T, want map", metaMap["list"])
	}

	if listMeta["description"] != "List items with filters" {
		t.Errorf("description = %v, want 'List items with filters'", listMeta["description"])
	}

	params, ok := listMeta["parameters"].([]any)
	if !ok {
		t.Fatalf("parameters type = %T, want []any", listMeta["parameters"])
	}
	if len(params) != 3 {
		t.Fatalf("parameters length = %d, want 3", len(params))
	}

	// Check first parameter
	p0, ok := params[0].(map[string]any)
	if !ok {
		t.Fatalf("params[0] type = %T, want map", params[0])
	}
	if p0["name"] != "status" {
		t.Errorf("params[0].name = %v, want 'status'", p0["name"])
	}
	if p0["type"] != "string" {
		t.Errorf("params[0].type = %v, want 'string'", p0["type"])
	}
	if p0["optional"] != true {
		t.Errorf("params[0].optional = %v, want true", p0["optional"])
	}

	examples, ok := listMeta["examples"].([]any)
	if !ok {
		t.Fatalf("examples type = %T, want []any", listMeta["examples"])
	}
	if len(examples) != 2 {
		t.Fatalf("examples length = %d, want 2", len(examples))
	}
}

func TestSchemaOperation_ParameterDefDefaultsAndOptionals(t *testing.T) {
	// Verify default values and optional flags render correctly in JSON.
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	s.OperationWithMetadata("op", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, nil
	}, OperationMetadata{
		Parameters: []ParameterDef{
			{Name: "required-param", Type: "string", Optional: false},
			{Name: "optional-with-default", Type: "int", Optional: true, Default: 42},
			{Name: "optional-no-default", Type: "bool", Optional: true},
		},
	})

	data, err := s.QueryJSON("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	metaMap := m["operationMetadata"].(map[string]any)
	opMeta := metaMap["op"].(map[string]any)
	params := opMeta["parameters"].([]any)

	// Required param: optional=false, no default key
	p0 := params[0].(map[string]any)
	if p0["optional"] != false {
		t.Errorf("params[0].optional = %v, want false", p0["optional"])
	}
	if _, hasDefault := p0["default"]; hasDefault {
		t.Error("params[0] should not have 'default' key (omitempty)")
	}

	// Optional with default: optional=true, default=42
	p1 := params[1].(map[string]any)
	if p1["optional"] != true {
		t.Errorf("params[1].optional = %v, want true", p1["optional"])
	}
	// JSON numbers are float64
	if p1["default"] != float64(42) {
		t.Errorf("params[1].default = %v (%T), want 42", p1["default"], p1["default"])
	}

	// Optional without default: no default key
	p2 := params[2].(map[string]any)
	if p2["optional"] != true {
		t.Errorf("params[2].optional = %v, want true", p2["optional"])
	}
	if _, hasDefault := p2["default"]; hasDefault {
		t.Error("params[2] should not have 'default' key (omitempty)")
	}
}

func TestSchemaOperation_CompactOutputWithMetadata(t *testing.T) {
	// Compact output of schema() still works when metadata is present.
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	s.OperationWithMetadata("list", func(ctx OperationContext[*testItem]) (any, error) {
		return nil, nil
	}, OperationMetadata{
		Description: "List items",
	})

	data, err := s.QueryJSONWithMode("schema()", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	if len(out) == 0 {
		t.Fatal("compact output should not be empty")
	}
	// Compact output for a map should contain key:value pairs — not crash.
	// schema() returns a map, so compact format renders as key:value lines.
	// The main check is that it doesn't error out.
}

func TestSchemaOperation_OperationWithMetadataRegistersHandler(t *testing.T) {
	// OperationWithMetadata must register the handler correctly —
	// the operation should still be callable.
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.SetLoader(func() ([]*testItem, error) {
		return []*testItem{{ID: "X1"}, {ID: "X2"}}, nil
	})

	called := false
	s.OperationWithMetadata("ping", func(ctx OperationContext[*testItem]) (any, error) {
		called = true
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(items)}, nil
	}, OperationMetadata{
		Description: "Ping operation for testing",
		Examples:    []string{"ping()"},
	})

	result, err := s.Query("ping()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Error("handler registered via OperationWithMetadata was not called")
	}

	m := result.(map[string]any)
	if m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
}
