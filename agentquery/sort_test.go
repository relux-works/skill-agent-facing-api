package agentquery

import (
	"strings"
	"testing"
)

// --- SortFieldOf tests ---

func TestSortFieldOf_String(t *testing.T) {
	cmp := SortFieldOf[*testItem](func(item *testItem) string { return item.Name })

	a := &testItem{Name: "alpha"}
	b := &testItem{Name: "beta"}

	if result := cmp(a, b); result >= 0 {
		t.Errorf("expected alpha < beta, got %d", result)
	}
	if result := cmp(b, a); result <= 0 {
		t.Errorf("expected beta > alpha, got %d", result)
	}
	if result := cmp(a, a); result != 0 {
		t.Errorf("expected alpha == alpha, got %d", result)
	}
}

func TestSortFieldOf_Int(t *testing.T) {
	cmp := SortFieldOf[*testItem](func(item *testItem) int { return item.Score })

	low := &testItem{Score: 10}
	high := &testItem{Score: 50}

	if result := cmp(low, high); result >= 0 {
		t.Errorf("expected 10 < 50, got %d", result)
	}
	if result := cmp(high, low); result <= 0 {
		t.Errorf("expected 50 > 10, got %d", result)
	}
	if result := cmp(low, low); result != 0 {
		t.Errorf("expected equal, got %d", result)
	}
}

func TestSortFieldOf_CustomComparator(t *testing.T) {
	// Custom comparator: enum priority ranking (not alphabetical).
	priorityRank := map[string]int{
		"critical": 0,
		"high":     1,
		"medium":   2,
		"low":      3,
	}

	cmp := func(a, b *testItem) int {
		ra := priorityRank[a.Status]
		rb := priorityRank[b.Status]
		if ra < rb {
			return -1
		}
		if ra > rb {
			return 1
		}
		return 0
	}

	critical := &testItem{Status: "critical"}
	low := &testItem{Status: "low"}

	if result := cmp(critical, low); result >= 0 {
		t.Errorf("expected critical < low in priority ranking, got %d", result)
	}
	if result := cmp(low, critical); result <= 0 {
		t.Errorf("expected low > critical in priority ranking, got %d", result)
	}
}

// --- SortableField convenience tests ---

func TestSortableField_RegistersOnSchema(t *testing.T) {
	s := NewSchema[*testItem]()
	SortableField(s, "name", func(item *testItem) string { return item.Name })

	sf := s.SortFields()
	if sf == nil {
		t.Fatal("SortFields() returned nil after SortableField registration")
	}
	if _, exists := sf["name"]; !exists {
		t.Error("sort field 'name' not found after SortableField registration")
	}
}

func TestSortableFieldFunc_RegistersOnSchema(t *testing.T) {
	s := NewSchema[*testItem]()
	SortableFieldFunc(s, "priority", func(a, b *testItem) int {
		return 0 // dummy comparator
	})

	sf := s.SortFields()
	if sf == nil {
		t.Fatal("SortFields() returned nil after SortableFieldFunc registration")
	}
	if _, exists := sf["priority"]; !exists {
		t.Error("sort field 'priority' not found after SortableFieldFunc registration")
	}
}

// --- Schema.SortField tests ---

func TestSchemaSortField_Single(t *testing.T) {
	s := NewSchema[*testItem]()
	s.SortField("name", func(a, b *testItem) int { return 0 })

	if len(s.sortFields) != 1 {
		t.Errorf("expected 1 sort field, got %d", len(s.sortFields))
	}
	if len(s.sortFieldNames) != 1 || s.sortFieldNames[0] != "name" {
		t.Errorf("sortFieldNames = %v, want [name]", s.sortFieldNames)
	}
}

func TestSchemaSortField_Multiple(t *testing.T) {
	s := NewSchema[*testItem]()
	s.SortField("name", func(a, b *testItem) int { return 0 })
	s.SortField("score", func(a, b *testItem) int { return 0 })
	s.SortField("status", func(a, b *testItem) int { return 0 })

	if len(s.sortFields) != 3 {
		t.Errorf("expected 3 sort fields, got %d", len(s.sortFields))
	}
	expected := []string{"name", "score", "status"}
	if len(s.sortFieldNames) != len(expected) {
		t.Fatalf("sortFieldNames = %v, want %v", s.sortFieldNames, expected)
	}
	for i, name := range expected {
		if s.sortFieldNames[i] != name {
			t.Errorf("sortFieldNames[%d] = %q, want %q", i, s.sortFieldNames[i], name)
		}
	}
}

func TestSchemaSortField_Overwrite(t *testing.T) {
	s := NewSchema[*testItem]()
	s.SortField("name", func(a, b *testItem) int { return 1 })
	s.SortField("name", func(a, b *testItem) int { return -1 })

	// Should overwrite comparator but not duplicate the name.
	if len(s.sortFields) != 1 {
		t.Errorf("expected 1 sort field after overwrite, got %d", len(s.sortFields))
	}
	if len(s.sortFieldNames) != 1 {
		t.Errorf("expected 1 name after overwrite, got %d", len(s.sortFieldNames))
	}

	// The overwritten comparator should be the second one.
	a := &testItem{Name: "a"}
	b := &testItem{Name: "b"}
	if result := s.sortFields["name"](a, b); result != -1 {
		t.Errorf("expected overwritten comparator to return -1, got %d", result)
	}
}

func TestSchemaSortFields_Getter(t *testing.T) {
	s := NewSchema[*testItem]()

	// Before registration — nil.
	if sf := s.SortFields(); sf != nil {
		t.Errorf("expected nil before registration, got %v", sf)
	}

	s.SortField("name", func(a, b *testItem) int { return 0 })

	sf := s.SortFields()
	if sf == nil {
		t.Fatal("SortFields() returned nil after registration")
	}
	if len(sf) != 1 {
		t.Errorf("expected 1 sort field, got %d", len(sf))
	}
}

// --- ParseSortSpecs tests ---

func TestParseSortSpecs_SingleAsc(t *testing.T) {
	args := []Arg{{Key: "sort_name", Value: "asc"}}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Field != "name" || specs[0].Direction != Asc {
		t.Errorf("spec = %+v, want {Field:name Direction:Asc}", specs[0])
	}
}

func TestParseSortSpecs_SingleDesc(t *testing.T) {
	args := []Arg{{Key: "sort_score", Value: "desc"}}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Field != "score" || specs[0].Direction != Desc {
		t.Errorf("spec = %+v, want {Field:score Direction:Desc}", specs[0])
	}
}

func TestParseSortSpecs_Multi(t *testing.T) {
	args := []Arg{
		{Key: "sort_status", Value: "asc"},
		{Key: "sort_score", Value: "desc"},
	}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Field != "status" || specs[0].Direction != Asc {
		t.Errorf("specs[0] = %+v, want {Field:status Direction:Asc}", specs[0])
	}
	if specs[1].Field != "score" || specs[1].Direction != Desc {
		t.Errorf("specs[1] = %+v, want {Field:score Direction:Desc}", specs[1])
	}
}

func TestParseSortSpecs_NoSortArgs(t *testing.T) {
	args := []Arg{
		{Key: "status", Value: "open"},
		{Key: "skip", Value: "1"},
	}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected 0 specs (no sort args), got %d", len(specs))
	}
}

func TestParseSortSpecs_EmptyArgs(t *testing.T) {
	specs, err := ParseSortSpecs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for nil args, got %d", len(specs))
	}
}

func TestParseSortSpecs_DefaultDirectionAsc(t *testing.T) {
	// Empty value should default to Asc.
	args := []Arg{{Key: "sort_name", Value: ""}}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Direction != Asc {
		t.Errorf("expected Asc for empty value, got %d", specs[0].Direction)
	}
}

func TestParseSortSpecs_CaseInsensitiveDirection(t *testing.T) {
	args := []Arg{{Key: "sort_name", Value: "DESC"}}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs[0].Direction != Desc {
		t.Errorf("expected Desc for 'DESC', got %d", specs[0].Direction)
	}
}

func TestParseSortSpecs_InvalidDirection(t *testing.T) {
	args := []Arg{{Key: "sort_name", Value: "upward"}}
	_, err := ParseSortSpecs(args)
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
	if !strings.Contains(e.Message, "asc") || !strings.Contains(e.Message, "desc") {
		t.Errorf("error message %q should mention 'asc' and 'desc'", e.Message)
	}
}

func TestParseSortSpecs_EmptyFieldAfterPrefix(t *testing.T) {
	args := []Arg{{Key: "sort_", Value: "asc"}}
	_, err := ParseSortSpecs(args)
	if err == nil {
		t.Fatal("expected error for empty field after sort_ prefix")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
	if !strings.Contains(e.Message, "field name") {
		t.Errorf("error message %q should mention 'field name'", e.Message)
	}
}

func TestParseSortSpecs_IgnoresNonSortArgs(t *testing.T) {
	args := []Arg{
		{Key: "status", Value: "open"},
		{Key: "sort_name", Value: "asc"},
		{Key: "skip", Value: "1"},
		{Key: "sort_score", Value: "desc"},
		{Key: "take", Value: "5"},
	}
	specs, err := ParseSortSpecs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 sort specs, got %d", len(specs))
	}
	if specs[0].Field != "name" {
		t.Errorf("specs[0].Field = %q, want 'name'", specs[0].Field)
	}
	if specs[1].Field != "score" {
		t.Errorf("specs[1].Field = %q, want 'score'", specs[1].Field)
	}
}

// --- BuildSortFunc tests ---

func TestBuildSortFunc_SingleAsc(t *testing.T) {
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	specs := []SortSpec{{Field: "name", Direction: Asc}}
	cmpFunc, err := BuildSortFunc(specs, sortFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmpFunc == nil {
		t.Fatal("expected non-nil cmpFunc")
	}

	a := &testItem{Name: "alpha"}
	b := &testItem{Name: "beta"}

	if result := cmpFunc(a, b); result >= 0 {
		t.Errorf("expected alpha < beta asc, got %d", result)
	}
}

func TestBuildSortFunc_SingleDesc(t *testing.T) {
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	specs := []SortSpec{{Field: "name", Direction: Desc}}
	cmpFunc, err := BuildSortFunc(specs, sortFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := &testItem{Name: "alpha"}
	b := &testItem{Name: "beta"}

	// Descending: alpha should come AFTER beta.
	if result := cmpFunc(a, b); result <= 0 {
		t.Errorf("expected alpha > beta desc, got %d", result)
	}
}

func TestBuildSortFunc_MultiField(t *testing.T) {
	sortFields := map[string]SortComparator[*testItem]{
		"status": SortFieldOf[*testItem](func(item *testItem) string { return item.Status }),
		"score":  SortFieldOf[*testItem](func(item *testItem) int { return item.Score }),
	}

	// Primary: status asc, Secondary: score desc.
	specs := []SortSpec{
		{Field: "status", Direction: Asc},
		{Field: "score", Direction: Desc},
	}
	cmpFunc, err := BuildSortFunc(specs, sortFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same status, different scores — secondary sort kicks in.
	a := &testItem{Status: "open", Score: 10}
	b := &testItem{Status: "open", Score: 50}

	// Score desc: 50 should come before 10.
	if result := cmpFunc(a, b); result <= 0 {
		t.Errorf("expected a (score=10) after b (score=50) in desc, got %d", result)
	}

	// Different status — primary sort decides.
	c := &testItem{Status: "closed", Score: 100}
	d := &testItem{Status: "open", Score: 1}

	if result := cmpFunc(c, d); result >= 0 {
		t.Errorf("expected closed < open in asc, got %d", result)
	}
}

func TestBuildSortFunc_UnknownField(t *testing.T) {
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	specs := []SortSpec{{Field: "nonexistent", Direction: Asc}}
	_, err := BuildSortFunc(specs, sortFields)
	if err == nil {
		t.Fatal("expected error for unknown sort field")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
	if !strings.Contains(e.Message, "nonexistent") {
		t.Errorf("error message %q should mention field name", e.Message)
	}
}

func TestBuildSortFunc_EmptySpecs(t *testing.T) {
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	cmpFunc, err := BuildSortFunc[*testItem](nil, sortFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmpFunc != nil {
		t.Error("expected nil cmpFunc for empty specs")
	}
}

// --- SortSlice tests ---

func TestSortSlice_SortsInPlace(t *testing.T) {
	items := []*testItem{
		{Name: "gamma", Score: 30},
		{Name: "alpha", Score: 10},
		{Name: "beta", Score: 20},
	}

	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	args := []Arg{{Key: "sort_name", Value: "asc"}}
	if err := SortSlice(items, args, sortFields); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"alpha", "beta", "gamma"}
	for i, want := range expected {
		if items[i].Name != want {
			t.Errorf("items[%d].Name = %q, want %q", i, items[i].Name, want)
		}
	}
}

func TestSortSlice_Desc(t *testing.T) {
	items := []*testItem{
		{Score: 10},
		{Score: 50},
		{Score: 30},
	}

	sortFields := map[string]SortComparator[*testItem]{
		"score": SortFieldOf[*testItem](func(item *testItem) int { return item.Score }),
	}

	args := []Arg{{Key: "sort_score", Value: "desc"}}
	if err := SortSlice(items, args, sortFields); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []int{50, 30, 10}
	for i, want := range expected {
		if items[i].Score != want {
			t.Errorf("items[%d].Score = %d, want %d", i, items[i].Score, want)
		}
	}
}

func TestSortSlice_StableSort(t *testing.T) {
	// Items with same status should preserve original order (stable sort).
	items := []*testItem{
		{ID: "T1", Status: "open", Score: 10},
		{ID: "T2", Status: "closed", Score: 20},
		{ID: "T3", Status: "open", Score: 30},
		{ID: "T4", Status: "closed", Score: 40},
		{ID: "T5", Status: "open", Score: 50},
	}

	sortFields := map[string]SortComparator[*testItem]{
		"status": SortFieldOf[*testItem](func(item *testItem) string { return item.Status }),
	}

	args := []Arg{{Key: "sort_status", Value: "asc"}}
	if err := SortSlice(items, args, sortFields); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "closed" < "open" alphabetically.
	// closed items: T2, T4 (original order preserved)
	// open items: T1, T3, T5 (original order preserved)
	expectedIDs := []string{"T2", "T4", "T1", "T3", "T5"}
	for i, want := range expectedIDs {
		if items[i].ID != want {
			t.Errorf("items[%d].ID = %q, want %q", i, items[i].ID, want)
		}
	}
}

func TestSortSlice_NoSortArgsNoop(t *testing.T) {
	items := []*testItem{
		{Name: "gamma"},
		{Name: "alpha"},
		{Name: "beta"},
	}

	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	// No sort_ args — should be a no-op.
	args := []Arg{{Key: "status", Value: "open"}, {Key: "skip", Value: "1"}}
	if err := SortSlice(items, args, sortFields); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original order preserved.
	expected := []string{"gamma", "alpha", "beta"}
	for i, want := range expected {
		if items[i].Name != want {
			t.Errorf("items[%d].Name = %q, want %q (should be unchanged)", i, items[i].Name, want)
		}
	}
}

func TestSortSlice_NilArgsNoop(t *testing.T) {
	items := []*testItem{
		{Name: "gamma"},
		{Name: "alpha"},
	}

	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	if err := SortSlice(items, nil, sortFields); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[0].Name != "gamma" || items[1].Name != "alpha" {
		t.Error("items should be unchanged with nil args")
	}
}

func TestSortSlice_ErrorPropagation_InvalidDirection(t *testing.T) {
	items := []*testItem{{Name: "a"}}
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	args := []Arg{{Key: "sort_name", Value: "sideways"}}
	err := SortSlice(items, args, sortFields)
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
}

func TestSortSlice_ErrorPropagation_UnknownField(t *testing.T) {
	items := []*testItem{{Name: "a"}}
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	args := []Arg{{Key: "sort_unknown", Value: "asc"}}
	err := SortSlice(items, args, sortFields)
	if err == nil {
		t.Fatal("expected error for unknown sort field")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
}

func TestSortSlice_EmptySlice(t *testing.T) {
	var items []*testItem
	sortFields := map[string]SortComparator[*testItem]{
		"name": SortFieldOf[*testItem](func(item *testItem) string { return item.Name }),
	}

	args := []Arg{{Key: "sort_name", Value: "asc"}}
	if err := SortSlice(items, args, sortFields); err != nil {
		t.Fatalf("unexpected error on empty slice: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

// --- Schema introspection tests ---

func TestIntrospect_IncludesSortableFields(t *testing.T) {
	s := newTestSchema()
	SortableField(s, "name", func(item *testItem) string { return item.Name })
	SortableField(s, "score", func(item *testItem) int { return item.Score })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	raw, exists := m["sortableFields"]
	if !exists {
		t.Fatal("sortableFields key missing from schema() output")
	}

	sortable, ok := raw.([]string)
	if !ok {
		t.Fatalf("sortableFields type = %T, want []string", raw)
	}
	if len(sortable) != 2 {
		t.Fatalf("sortableFields = %v, want [name score]", sortable)
	}
	if sortable[0] != "name" || sortable[1] != "score" {
		t.Errorf("sortableFields = %v, want [name score]", sortable)
	}
}

func TestIntrospect_OmitsSortableFieldsWhenNone(t *testing.T) {
	s := newTestSchema()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, exists := m["sortableFields"]; exists {
		t.Error("sortableFields should be absent when no sort fields are registered")
	}
}

func TestIntrospect_SortableFieldsInRegistrationOrder(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })

	// Register in non-alphabetical order.
	SortableField(s, "score", func(item *testItem) int { return item.Score })
	SortableField(s, "name", func(item *testItem) string { return item.Name })
	SortableField(s, "id", func(item *testItem) string { return item.ID })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	sortable := m["sortableFields"].([]string)

	expected := []string{"score", "name", "id"}
	if len(sortable) != len(expected) {
		t.Fatalf("sortableFields = %v, want %v", sortable, expected)
	}
	for i, want := range expected {
		if sortable[i] != want {
			t.Errorf("sortableFields[%d] = %q, want %q", i, sortable[i], want)
		}
	}
}

func TestIntrospect_SortableFieldsCopied(t *testing.T) {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	SortableField(s, "name", func(item *testItem) string { return item.Name })

	result1, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m1 := result1.(map[string]any)
	sortable1 := m1["sortableFields"].([]string)

	// Mutate the returned slice.
	if len(sortable1) > 0 {
		sortable1[0] = "MUTATED"
	}

	// Second call should return clean data.
	result2, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m2 := result2.(map[string]any)
	sortable2 := m2["sortableFields"].([]string)

	if len(sortable2) > 0 && sortable2[0] == "MUTATED" {
		t.Error("schema() returned a reference to internal state, not a copy")
	}
}

// --- Table-driven tests for ParseSortSpecs ---

func TestParseSortSpecs_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		args      []Arg
		wantLen   int
		wantSpecs []SortSpec
		wantErr   string
	}{
		{
			name:    "nil args",
			args:    nil,
			wantLen: 0,
		},
		{
			name:    "no sort args",
			args:    []Arg{{Key: "status", Value: "open"}},
			wantLen: 0,
		},
		{
			name:    "single asc",
			args:    []Arg{{Key: "sort_name", Value: "asc"}},
			wantLen: 1,
			wantSpecs: []SortSpec{
				{Field: "name", Direction: Asc},
			},
		},
		{
			name:    "single desc",
			args:    []Arg{{Key: "sort_score", Value: "desc"}},
			wantLen: 1,
			wantSpecs: []SortSpec{
				{Field: "score", Direction: Desc},
			},
		},
		{
			name:    "empty value defaults to asc",
			args:    []Arg{{Key: "sort_name", Value: ""}},
			wantLen: 1,
			wantSpecs: []SortSpec{
				{Field: "name", Direction: Asc},
			},
		},
		{
			name: "multi sort mixed with other args",
			args: []Arg{
				{Key: "status", Value: "open"},
				{Key: "sort_status", Value: "asc"},
				{Key: "sort_score", Value: "desc"},
				{Key: "take", Value: "10"},
			},
			wantLen: 2,
			wantSpecs: []SortSpec{
				{Field: "status", Direction: Asc},
				{Field: "score", Direction: Desc},
			},
		},
		{
			name:    "invalid direction",
			args:    []Arg{{Key: "sort_name", Value: "random"}},
			wantErr: "sort direction must be",
		},
		{
			name:    "empty field name",
			args:    []Arg{{Key: "sort_", Value: "asc"}},
			wantErr: "requires a field name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs, err := ParseSortSpecs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(specs) != tt.wantLen {
				t.Fatalf("len(specs) = %d, want %d", len(specs), tt.wantLen)
			}
			for i, want := range tt.wantSpecs {
				if i >= len(specs) {
					break
				}
				if specs[i].Field != want.Field {
					t.Errorf("specs[%d].Field = %q, want %q", i, specs[i].Field, want.Field)
				}
				if specs[i].Direction != want.Direction {
					t.Errorf("specs[%d].Direction = %d, want %d", i, specs[i].Direction, want.Direction)
				}
			}
		})
	}
}
