package agentquery

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- FilterableField registration tests ---

func TestFilterableField_SingleRegistration(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	if len(s.filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(s.filters))
	}
	if _, exists := s.filters["status"]; !exists {
		t.Error("filter 'status' not registered")
	}
	if len(s.filterOrder) != 1 || s.filterOrder[0] != "status" {
		t.Errorf("filterOrder = %v, want [status]", s.filterOrder)
	}
}

func TestFilterableField_MultipleRegistrations(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	FilterableField(s, "name", func(item *testItem) string { return item.Name })

	if len(s.filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(s.filters))
	}
	if len(s.filterOrder) != 2 {
		t.Fatalf("filterOrder length = %d, want 2", len(s.filterOrder))
	}
	if s.filterOrder[0] != "status" || s.filterOrder[1] != "name" {
		t.Errorf("filterOrder = %v, want [status name]", s.filterOrder)
	}
}

func TestFilterableField_Overwrite(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	// Overwrite with a different accessor
	FilterableField(s, "status", func(item *testItem) string { return strings.ToUpper(item.Status) })

	if len(s.filters) != 1 {
		t.Fatalf("expected 1 filter after overwrite, got %d", len(s.filters))
	}
	// filterOrder should not duplicate on overwrite
	if len(s.filterOrder) != 1 {
		t.Errorf("filterOrder length = %d, want 1 (no duplicate on overwrite)", len(s.filterOrder))
	}

	// Verify the overwritten accessor is used
	item := &testItem{Status: "open"}
	accessor := s.filters["status"]
	if accessor(item) != "OPEN" {
		t.Errorf("accessor returned %q, want OPEN (overwritten accessor)", accessor(item))
	}
}

func TestFilterableField_NilInitialMaps(t *testing.T) {
	// Verify that FilterableField works on a fresh schema where
	// filters and filterOrder are nil (zero values).
	s := NewSchema[*testItem]()
	if s.filters != nil {
		t.Fatal("expected nil filters map on fresh schema")
	}
	if s.filterOrder != nil {
		t.Fatal("expected nil filterOrder on fresh schema")
	}

	// This should not panic
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	if s.filters == nil {
		t.Fatal("filters should be initialized after FilterableField")
	}
	if len(s.filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(s.filters))
	}
}

// --- buildPredicate tests ---

func TestBuildPredicate_SingleFilterMatch(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	args := []Arg{{Key: "status", Value: "open"}}
	pred := s.buildPredicate(args)

	open := &testItem{Status: "open"}
	closed := &testItem{Status: "closed"}

	if !pred(open) {
		t.Error("predicate should match item with status=open")
	}
	if pred(closed) {
		t.Error("predicate should not match item with status=closed")
	}
}

func TestBuildPredicate_MultipleFiltersAND(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	FilterableField(s, "name", func(item *testItem) string { return item.Name })

	args := []Arg{
		{Key: "status", Value: "open"},
		{Key: "name", Value: "alpha"},
	}
	pred := s.buildPredicate(args)

	matchBoth := &testItem{Status: "open", Name: "alpha"}
	matchStatus := &testItem{Status: "open", Name: "beta"}
	matchName := &testItem{Status: "closed", Name: "alpha"}
	matchNeither := &testItem{Status: "closed", Name: "beta"}

	if !pred(matchBoth) {
		t.Error("predicate should match item with status=open AND name=alpha")
	}
	if pred(matchStatus) {
		t.Error("predicate should not match item with only status=open (name mismatch)")
	}
	if pred(matchName) {
		t.Error("predicate should not match item with only name=alpha (status mismatch)")
	}
	if pred(matchNeither) {
		t.Error("predicate should not match item with neither matching")
	}
}

func TestBuildPredicate_NoMatchingFilters(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	// Args with keys that don't match any registered filter
	args := []Arg{
		{Key: "unknown", Value: "whatever"},
		{Key: "skip", Value: "5"},
	}
	pred := s.buildPredicate(args)

	// Should behave like MatchAll
	item := &testItem{Status: "anything"}
	if !pred(item) {
		t.Error("predicate should match all items when no args match registered filters")
	}
}

func TestBuildPredicate_NoFiltersRegistered(t *testing.T) {
	s := NewSchema[*testItem]()
	// No FilterableField calls — filters map is nil

	args := []Arg{{Key: "status", Value: "open"}}
	pred := s.buildPredicate(args)

	// Should behave like MatchAll
	item := &testItem{Status: "closed"}
	if !pred(item) {
		t.Error("predicate should match all items when no filters are registered")
	}
}

func TestBuildPredicate_SkipsPositionalArgs(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	// Positional arg (Key="") should be ignored even if Value matches something
	args := []Arg{
		{Key: "", Value: "open"},
	}
	pred := s.buildPredicate(args)

	// Should behave like MatchAll since no key-matched args
	item := &testItem{Status: "closed"}
	if !pred(item) {
		t.Error("predicate should match all items when only positional args are present")
	}
}

func TestBuildPredicate_SkipsUnregisteredArgKeys(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	// Mix of registered and unregistered arg keys
	args := []Arg{
		{Key: "skip", Value: "2"},
		{Key: "status", Value: "open"},
		{Key: "take", Value: "5"},
		{Key: "unknown", Value: "foo"},
	}
	pred := s.buildPredicate(args)

	open := &testItem{Status: "open"}
	closed := &testItem{Status: "closed"}

	if !pred(open) {
		t.Error("predicate should match item with status=open")
	}
	if pred(closed) {
		t.Error("predicate should not match item with status=closed")
	}
}

func TestBuildPredicate_CaseInsensitive(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	tests := []struct {
		argValue   string
		itemStatus string
		wantMatch  bool
	}{
		{"open", "open", true},
		{"OPEN", "open", true},
		{"Open", "open", true},
		{"open", "OPEN", true},
		{"open", "Open", true},
		{"closed", "open", false},
	}

	for _, tt := range tests {
		args := []Arg{{Key: "status", Value: tt.argValue}}
		pred := s.buildPredicate(args)
		item := &testItem{Status: tt.itemStatus}
		got := pred(item)
		if got != tt.wantMatch {
			t.Errorf("buildPredicate(status=%q) on Status=%q = %v, want %v",
				tt.argValue, tt.itemStatus, got, tt.wantMatch)
		}
	}
}

func TestBuildPredicate_EmptyArgs(t *testing.T) {
	s := NewSchema[*testItem]()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	pred := s.buildPredicate(nil)

	item := &testItem{Status: "anything"}
	if !pred(item) {
		t.Error("predicate should match all items when args is nil")
	}
}

// --- Predicate injection in full query flow ---

func TestPredicate_InjectedInOperationContext(t *testing.T) {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	items := []*testItem{
		{ID: "T1", Name: "alpha", Status: "open"},
		{ID: "T2", Name: "beta", Status: "closed"},
		{ID: "T3", Name: "gamma", Status: "open"},
	}
	s.SetLoader(func() ([]*testItem, error) { return items, nil })

	// Use ctx.Predicate in the operation handler
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		filtered := FilterItems(data, ctx.Predicate)
		var out []map[string]any
		for _, item := range filtered {
			out = append(out, ctx.Selector.Apply(item))
		}
		if out == nil {
			out = []map[string]any{}
		}
		return out, nil
	})

	result, err := s.Query("list(status=open) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items2, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items2) != 2 {
		t.Fatalf("expected 2 open items, got %d", len(items2))
	}
	if items2[0]["id"] != "T1" {
		t.Errorf("items[0].id = %v, want T1", items2[0]["id"])
	}
	if items2[1]["id"] != "T3" {
		t.Errorf("items[1].id = %v, want T3", items2[1]["id"])
	}
}

func TestPredicate_MatchAllWhenNoFilterArgs(t *testing.T) {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	items := []*testItem{
		{ID: "T1", Status: "open"},
		{ID: "T2", Status: "closed"},
	}
	s.SetLoader(func() ([]*testItem, error) { return items, nil })

	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		filtered := FilterItems(data, ctx.Predicate)
		var out []map[string]any
		for _, item := range filtered {
			out = append(out, ctx.Selector.Apply(item))
		}
		return out, nil
	})

	// No filter args — predicate should be MatchAll
	result, err := s.Query("list() { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items2, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items2) != 2 {
		t.Errorf("expected 2 items (no filter), got %d", len(items2))
	}
}

func TestPredicate_WithCountItems(t *testing.T) {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	items := []*testItem{
		{ID: "T1", Status: "open"},
		{ID: "T2", Status: "closed"},
		{ID: "T3", Status: "open"},
		{ID: "T4", Status: "closed"},
		{ID: "T5", Status: "open"},
	}
	s.SetLoader(func() ([]*testItem, error) { return items, nil })

	s.Operation("count", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		n := CountItems(data, ctx.Predicate)
		return map[string]any{"count": n}, nil
	})

	result, err := s.Query("count(status=closed)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 2 {
		t.Errorf("count(status=closed) = %v, want 2", m["count"])
	}
}

func TestPredicate_CaseInsensitiveInQuery(t *testing.T) {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	items := []*testItem{
		{ID: "T1", Status: "open"},
		{ID: "T2", Status: "closed"},
	}
	s.SetLoader(func() ([]*testItem, error) { return items, nil })

	s.Operation("count", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": CountItems(data, ctx.Predicate)}, nil
	})

	// Use uppercase filter value
	result, err := s.Query("count(status=OPEN)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 1 {
		t.Errorf("count(status=OPEN) = %v, want 1", m["count"])
	}
}

// --- Schema introspection tests ---

func TestIntrospect_IncludesFilterableFields(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("status", func(item *testItem) any { return item.Status })
	s.Field("name", func(item *testItem) any { return item.Name })

	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	FilterableField(s, "name", func(item *testItem) string { return item.Name })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	filterableRaw, exists := m["filterableFields"]
	if !exists {
		t.Fatal("filterableFields key should be present when filters are registered")
	}

	filterable, ok := filterableRaw.([]string)
	if !ok {
		t.Fatalf("filterableFields type = %T, want []string", filterableRaw)
	}

	if len(filterable) != 2 {
		t.Fatalf("expected 2 filterable fields, got %d", len(filterable))
	}
	if filterable[0] != "status" || filterable[1] != "name" {
		t.Errorf("filterableFields = %v, want [status name]", filterable)
	}
}

func TestIntrospect_OmitsFilterableFieldsWhenNone(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, exists := m["filterableFields"]; exists {
		t.Error("filterableFields should be absent when no filters are registered")
	}
}

func TestIntrospect_FilterableFieldsReturnsCopy(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	result1, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m1 := result1.(map[string]any)
	filterable1 := m1["filterableFields"].([]string)

	// Mutate the returned slice
	if len(filterable1) > 0 {
		filterable1[0] = "MUTATED"
	}

	// Second call should return clean data
	result2, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m2 := result2.(map[string]any)
	filterable2 := m2["filterableFields"].([]string)

	if len(filterable2) > 0 && filterable2[0] == "MUTATED" {
		t.Error("schema() returned a reference to internal state, not a copy")
	}
}

func TestIntrospect_FilterableFieldsInRegistrationOrder(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	// Register in specific order
	FilterableField(s, "name", func(item *testItem) string { return item.Name })
	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	FilterableField(s, "id", func(item *testItem) string { return item.ID })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	filterable := m["filterableFields"].([]string)

	expected := []string{"name", "status", "id"}
	if len(filterable) != len(expected) {
		t.Fatalf("filterableFields = %v, want %v", filterable, expected)
	}
	for i, f := range expected {
		if filterable[i] != f {
			t.Errorf("filterableFields[%d] = %q, want %q", i, filterable[i], f)
		}
	}
}

// --- Distinct operation tests ---

func newDistinctTestSchema() *Schema[*testItem] {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	FilterableField(s, "name", func(item *testItem) string { return item.Name })

	items := []*testItem{
		{ID: "T1", Name: "alice", Status: "open"},
		{ID: "T2", Name: "bob", Status: "closed"},
		{ID: "T3", Name: "alice", Status: "in-progress"},
		{ID: "T4", Name: "charlie", Status: "open"},
		{ID: "T5", Name: "bob", Status: "open"},
	}
	s.SetLoader(func() ([]*testItem, error) { return items, nil })
	return s
}

func TestDistinct_ReturnsUniqueValues(t *testing.T) {
	s := newDistinctTestSchema()

	result, err := s.Query("distinct(status)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}

	// First-seen order: open, closed, in-progress
	expected := []string{"open", "closed", "in-progress"}
	if len(values) != len(expected) {
		t.Fatalf("distinct(status) = %v, want %v", values, expected)
	}
	for i, v := range expected {
		if values[i] != v {
			t.Errorf("distinct(status)[%d] = %q, want %q", i, values[i], v)
		}
	}
}

func TestDistinct_PreservesExactCase(t *testing.T) {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	// Items with mixed-case status values
	items := []*testItem{
		{ID: "T1", Status: "Open"},
		{ID: "T2", Status: "CLOSED"},
		{ID: "T3", Status: "open"},
	}
	s.SetLoader(func() ([]*testItem, error) { return items, nil })

	result, err := s.Query("distinct(status)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values := result.([]string)
	// All three are distinct because Distinct uses exact string comparison
	expected := []string{"Open", "CLOSED", "open"}
	if len(values) != len(expected) {
		t.Fatalf("distinct(status) = %v, want %v", values, expected)
	}
	for i, v := range expected {
		if values[i] != v {
			t.Errorf("distinct(status)[%d] = %q, want %q", i, values[i], v)
		}
	}
}

func TestDistinct_NoArgReturnsErrorResult(t *testing.T) {
	s := newDistinctTestSchema()

	result, err := s.Query("distinct()")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Single statement handler error is inlined as error map
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any (error), got %T: %v", result, result)
	}
	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected error key in result")
	}
	errMap := errObj.(map[string]any)
	msg := errMap["message"].(string)
	if !strings.Contains(msg, "requires a field name") {
		t.Errorf("error message = %q, want it to contain 'requires a field name'", msg)
	}
}

func TestDistinct_UnknownFieldReturnsError(t *testing.T) {
	s := newDistinctTestSchema()

	result, err := s.Query("distinct(nonexistent)")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any (error), got %T: %v", result, result)
	}
	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected error key in result")
	}
	errMap := errObj.(map[string]any)
	msg := errMap["message"].(string)
	if !strings.Contains(msg, "unknown filterable field") {
		t.Errorf("error message = %q, want it to contain 'unknown filterable field'", msg)
	}
}

func TestDistinct_AppearsInSchemaIntrospection(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)

	// Check distinct is in operations list
	ops := m["operations"].([]string)
	found := false
	for _, op := range ops {
		if op == "distinct" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("operations = %v, want 'distinct' to be present", ops)
	}

	// Check distinct has metadata
	metaRaw, exists := m["operationMetadata"]
	if !exists {
		t.Fatal("operationMetadata should be present when distinct is auto-registered")
	}
	meta := metaRaw.(map[string]OperationMetadata)
	distinctMeta, exists := meta["distinct"]
	if !exists {
		t.Fatal("operationMetadata should contain 'distinct' entry")
	}
	if distinctMeta.Description == "" {
		t.Error("distinct metadata should have a description")
	}
	if len(distinctMeta.Parameters) == 0 {
		t.Error("distinct metadata should have parameters")
	}
	if len(distinctMeta.Examples) == 0 {
		t.Error("distinct metadata should have examples")
	}
}

func TestDistinct_EndToEndViaQueryJSON(t *testing.T) {
	s := newDistinctTestSchema()

	data, err := s.QueryJSON("distinct(name)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v (raw: %s)", err, data)
	}

	// First-seen order: alice, bob, charlie
	expected := []string{"alice", "bob", "charlie"}
	if len(values) != len(expected) {
		t.Fatalf("distinct(name) = %v, want %v", values, expected)
	}
	for i, v := range expected {
		if values[i] != v {
			t.Errorf("distinct(name)[%d] = %q, want %q", i, values[i], v)
		}
	}
}

func TestDistinct_NotAutoRegisteredWithoutFilterableFields(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	// No FilterableField calls — distinct should NOT be registered
	if _, exists := s.operations["distinct"]; exists {
		t.Error("distinct operation should not exist when no filterable fields are registered")
	}
}

func TestDistinct_NotOverriddenByManualRegistration(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	// Manually register distinct before any FilterableField calls
	customCalled := false
	s.Operation("distinct", func(ctx OperationContext[*testItem]) (any, error) {
		customCalled = true
		return "custom", nil
	})

	// Now register a filterable field — should NOT overwrite the manual registration
	FilterableField(s, "status", func(item *testItem) string { return item.Status })

	s.SetLoader(func() ([]*testItem, error) {
		return []*testItem{{ID: "T1", Status: "open"}}, nil
	})

	result, err := s.Query("distinct(status)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !customCalled {
		t.Error("expected custom distinct handler to be called, but auto-registered one was used")
	}
	if result != "custom" {
		t.Errorf("result = %v, want 'custom'", result)
	}
}

func TestDistinct_EmptyDataset(t *testing.T) {
	s := newTestSchema()
	FilterableField(s, "status", func(item *testItem) string { return item.Status })
	s.SetLoader(func() ([]*testItem, error) { return nil, nil })

	result, err := s.Query("distinct(status)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Distinct on empty dataset returns nil (empty slice)
	if result != nil {
		values := result.([]string)
		if len(values) != 0 {
			t.Errorf("distinct(status) on empty dataset = %v, want empty", values)
		}
	}
}
