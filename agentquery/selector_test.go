package agentquery

import (
	"testing"
)

// testItem is a simple domain type for testing.
type testItem struct {
	ID     string
	Name   string
	Status string
	Score  int
	Tags   []string
}

// newTestSchema creates a Schema with a standard set of fields for testing.
func newTestSchema() *Schema[*testItem] {
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })
	s.Field("status", func(item *testItem) any { return item.Status })
	s.Field("score", func(item *testItem) any { return item.Score })
	s.Field("tags", func(item *testItem) any { return item.Tags })

	s.Preset("minimal", "id", "status")
	s.Preset("default", "id", "name", "status")
	s.Preset("full", "id", "name", "status", "score", "tags")

	s.DefaultFields("id", "name", "status")
	return s
}

func TestFieldRegistration(t *testing.T) {
	s := NewSchema[*testItem]()

	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })

	// Verify fields are registered by creating a selector
	sel, err := s.newSelector([]string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := &testItem{ID: "TASK-1", Name: "test task"}
	result := sel.Apply(item)

	if result["id"] != "TASK-1" {
		t.Errorf("id = %v, want TASK-1", result["id"])
	}
	if result["name"] != "test task" {
		t.Errorf("name = %v, want 'test task'", result["name"])
	}
}

func TestFieldRegistrationOverwrite(t *testing.T) {
	s := NewSchema[*testItem]()

	s.Field("id", func(item *testItem) any { return item.ID })
	// Re-register with a different accessor
	s.Field("id", func(item *testItem) any { return "overwritten-" + item.ID })

	sel, err := s.newSelector([]string{"id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := &testItem{ID: "TASK-1"}
	result := sel.Apply(item)

	if result["id"] != "overwritten-TASK-1" {
		t.Errorf("id = %v, want 'overwritten-TASK-1'", result["id"])
	}

	// Should not duplicate in fieldOrder
	if len(s.fieldOrder) != 1 {
		t.Errorf("fieldOrder length = %d, want 1 (no duplicates on overwrite)", len(s.fieldOrder))
	}
}

func TestPresetExpansion(t *testing.T) {
	s := newTestSchema()

	tests := []struct {
		name           string
		requested      []string
		expectedFields []string
	}{
		{
			name:           "minimal preset",
			requested:      []string{"minimal"},
			expectedFields: []string{"id", "status"},
		},
		{
			name:           "default preset",
			requested:      []string{"default"},
			expectedFields: []string{"id", "name", "status"},
		},
		{
			name:           "full preset",
			requested:      []string{"full"},
			expectedFields: []string{"id", "name", "status", "score", "tags"},
		},
		{
			name:           "preset plus extra field",
			requested:      []string{"minimal", "score"},
			expectedFields: []string{"id", "status", "score"},
		},
		{
			name:           "preset with overlapping field",
			requested:      []string{"minimal", "id"}, // id is already in minimal
			expectedFields: []string{"id", "status"},  // deduplicated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := s.newSelector(tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fields := sel.Fields()
			if len(fields) != len(tt.expectedFields) {
				t.Fatalf("fields count = %d, want %d; got %v", len(fields), len(tt.expectedFields), fields)
			}
			for i, f := range tt.expectedFields {
				if fields[i] != f {
					t.Errorf("field[%d] = %q, want %q", i, fields[i], f)
				}
			}
		})
	}
}

func TestDefaultFieldsWhenNoProjection(t *testing.T) {
	s := newTestSchema()

	sel, err := s.newSelector(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := sel.Fields()
	expected := []string{"id", "name", "status"}
	if len(fields) != len(expected) {
		t.Fatalf("fields = %v, want %v", fields, expected)
	}
	for i, f := range expected {
		if fields[i] != f {
			t.Errorf("field[%d] = %q, want %q", i, fields[i], f)
		}
	}
}

func TestEmptyProjectionNoDefaults(t *testing.T) {
	// When no defaults configured, all fields should be selected
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })
	// No DefaultFields call

	sel, err := s.newSelector(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := sel.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields (all), got %d: %v", len(fields), fields)
	}
	if fields[0] != "id" || fields[1] != "name" {
		t.Errorf("fields = %v, want [id name]", fields)
	}
}

func TestApplySelectedFields(t *testing.T) {
	s := newTestSchema()
	item := &testItem{
		ID:     "TASK-42",
		Name:   "implement feature",
		Status: "development",
		Score:  95,
		Tags:   []string{"go", "generics"},
	}

	tests := []struct {
		name      string
		requested []string
		checkKeys []string
		skipKeys  []string
	}{
		{
			name:      "only id and name",
			requested: []string{"id", "name"},
			checkKeys: []string{"id", "name"},
			skipKeys:  []string{"status", "score", "tags"},
		},
		{
			name:      "minimal preset",
			requested: []string{"minimal"},
			checkKeys: []string{"id", "status"},
			skipKeys:  []string{"name", "score", "tags"},
		},
		{
			name:      "full preset",
			requested: []string{"full"},
			checkKeys: []string{"id", "name", "status", "score", "tags"},
			skipKeys:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := s.newSelector(tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result := sel.Apply(item)

			for _, key := range tt.checkKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("missing key %q in result", key)
				}
			}
			for _, key := range tt.skipKeys {
				if _, ok := result[key]; ok {
					t.Errorf("unexpected key %q in result (should not be selected)", key)
				}
			}
		})
	}
}

func TestApplyFieldValues(t *testing.T) {
	s := newTestSchema()
	item := &testItem{
		ID:     "TASK-42",
		Name:   "implement feature",
		Status: "development",
		Score:  95,
		Tags:   []string{"go", "generics"},
	}

	sel, err := s.newSelector([]string{"full"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := sel.Apply(item)

	if result["id"] != "TASK-42" {
		t.Errorf("id = %v, want TASK-42", result["id"])
	}
	if result["name"] != "implement feature" {
		t.Errorf("name = %v, want 'implement feature'", result["name"])
	}
	if result["status"] != "development" {
		t.Errorf("status = %v, want 'development'", result["status"])
	}
	if result["score"] != 95 {
		t.Errorf("score = %v, want 95", result["score"])
	}
	tags, ok := result["tags"].([]string)
	if !ok {
		t.Fatalf("tags is not []string: %T", result["tags"])
	}
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "generics" {
		t.Errorf("tags = %v, want [go generics]", tags)
	}
}

func TestInclude(t *testing.T) {
	s := newTestSchema()

	sel, err := s.newSelector([]string{"id", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		field    string
		expected bool
	}{
		{"id", true},
		{"status", true},
		{"name", false},
		{"score", false},
		{"tags", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := sel.Include(tt.field)
			if got != tt.expected {
				t.Errorf("Include(%q) = %v, want %v", tt.field, got, tt.expected)
			}
		})
	}
}

func TestUnknownFieldError(t *testing.T) {
	s := newTestSchema()

	_, err := s.newSelector([]string{"id", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}

	// Verify it's a validation error
	agentErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if agentErr.Code != ErrValidation {
		t.Errorf("error code = %q, want %q", agentErr.Code, ErrValidation)
	}
	if agentErr.Details["field"] != "nonexistent" {
		t.Errorf("error details field = %v, want 'nonexistent'", agentErr.Details["field"])
	}
}

func TestUnknownFieldInMiddle(t *testing.T) {
	s := newTestSchema()

	_, err := s.newSelector([]string{"id", "bogus", "name"})
	if err == nil {
		t.Fatal("expected error for unknown field 'bogus', got nil")
	}

	agentErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if agentErr.Details["field"] != "bogus" {
		t.Errorf("error details field = %v, want 'bogus'", agentErr.Details["field"])
	}
}

func TestFieldsReturnsCopy(t *testing.T) {
	s := newTestSchema()

	sel, err := s.newSelector([]string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields1 := sel.Fields()
	fields1[0] = "mutated"

	fields2 := sel.Fields()
	if fields2[0] != "id" {
		t.Error("Fields() returned a reference to internal state, not a copy")
	}
}

func TestNewSchemaEmpty(t *testing.T) {
	s := NewSchema[*testItem]()

	// No fields, no defaults — selector should return empty
	sel, err := s.newSelector(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := sel.Fields()
	if len(fields) != 0 {
		t.Errorf("expected 0 fields on empty schema, got %d: %v", len(fields), fields)
	}
}

func TestApplyWithNilFields(t *testing.T) {
	s := newTestSchema()
	item := &testItem{
		ID:   "TASK-1",
		Tags: nil, // nil slice field
	}

	sel, err := s.newSelector([]string{"id", "tags"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := sel.Apply(item)
	if result["id"] != "TASK-1" {
		t.Errorf("id = %v, want TASK-1", result["id"])
	}
	// nil []string comes through as a typed nil ([]string(nil)), which is
	// not equal to untyped nil. Verify the accessor returns the value as-is.
	tags, ok := result["tags"].([]string)
	if !ok {
		t.Fatalf("tags type = %T, want []string", result["tags"])
	}
	if tags != nil {
		t.Errorf("tags = %v, want nil slice", tags)
	}
}

func TestLazyEvaluation(t *testing.T) {
	// Verify that accessors are only called for selected fields
	s := NewSchema[*testItem]()

	idCalled := false
	nameCalled := false
	statusCalled := false

	s.Field("id", func(item *testItem) any {
		idCalled = true
		return item.ID
	})
	s.Field("name", func(item *testItem) any {
		nameCalled = true
		return item.Name
	})
	s.Field("status", func(item *testItem) any {
		statusCalled = true
		return item.Status
	})

	sel, err := s.newSelector([]string{"id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := &testItem{ID: "TASK-1", Name: "test", Status: "open"}
	sel.Apply(item)

	if !idCalled {
		t.Error("id accessor was not called (should be selected)")
	}
	if nameCalled {
		t.Error("name accessor was called (should NOT be selected)")
	}
	if statusCalled {
		t.Error("status accessor was called (should NOT be selected)")
	}
}

func TestPresetOverlappingFields(t *testing.T) {
	// Two presets that share fields — should deduplicate
	s := NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })
	s.Field("status", func(item *testItem) any { return item.Status })

	s.Preset("set1", "id", "name")
	s.Preset("set2", "id", "status")

	sel, err := s.newSelector([]string{"set1", "set2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := sel.Fields()
	// Should be [id, name, status] — id deduplicated
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(fields), fields)
	}
	expected := []string{"id", "name", "status"}
	for i, f := range expected {
		if fields[i] != f {
			t.Errorf("field[%d] = %q, want %q", i, fields[i], f)
		}
	}
}

func TestErrorTypes(t *testing.T) {
	// Test ParseError formatting
	t.Run("ParseError with got and expected", func(t *testing.T) {
		e := &ParseError{
			Message:  "unexpected token",
			Pos:      Pos{Offset: 5, Line: 1, Column: 6},
			Got:      ")",
			Expected: "identifier",
		}
		msg := e.Error()
		if msg == "" {
			t.Error("ParseError.Error() returned empty string")
		}
	})

	t.Run("ParseError with got only", func(t *testing.T) {
		e := &ParseError{
			Message: "unexpected token",
			Pos:     Pos{Offset: 0, Line: 1, Column: 1},
			Got:     "EOF",
		}
		msg := e.Error()
		if msg == "" {
			t.Error("ParseError.Error() returned empty string")
		}
	})

	t.Run("ParseError minimal", func(t *testing.T) {
		e := &ParseError{
			Message: "empty input",
			Pos:     Pos{Offset: 0, Line: 0, Column: 0},
		}
		msg := e.Error()
		if msg == "" {
			t.Error("ParseError.Error() returned empty string")
		}
	})

	t.Run("Error", func(t *testing.T) {
		e := &Error{
			Code:    ErrNotFound,
			Message: "element not found",
			Details: map[string]any{"id": "TASK-99"},
		}
		if e.Error() != "element not found" {
			t.Errorf("Error() = %q, want %q", e.Error(), "element not found")
		}
	})
}

func TestErrorConstants(t *testing.T) {
	// Verify error code constants are distinct
	codes := map[string]bool{
		ErrParse:      true,
		ErrNotFound:   true,
		ErrValidation: true,
		ErrInternal:   true,
	}
	if len(codes) != 4 {
		t.Error("error code constants are not all distinct")
	}
}

func TestASTTypes(t *testing.T) {
	// Verify AST types can be constructed and used
	q := Query{
		Statements: []Statement{
			{
				Operation: "list",
				Args: []Arg{
					{Value: "open", Pos: Pos{Offset: 5, Line: 1, Column: 6}},
					{Key: "limit", Value: "10", Pos: Pos{Offset: 10, Line: 1, Column: 11}},
				},
				Fields: []string{"id", "name", "status"},
				Pos:    Pos{Offset: 0, Line: 1, Column: 1},
			},
		},
	}

	if len(q.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(q.Statements))
	}

	stmt := q.Statements[0]
	if stmt.Operation != "list" {
		t.Errorf("operation = %q, want 'list'", stmt.Operation)
	}
	if len(stmt.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(stmt.Args))
	}
	if stmt.Args[0].Key != "" || stmt.Args[0].Value != "open" {
		t.Errorf("arg[0] = {Key:%q Value:%q}, want positional 'open'", stmt.Args[0].Key, stmt.Args[0].Value)
	}
	if stmt.Args[1].Key != "limit" || stmt.Args[1].Value != "10" {
		t.Errorf("arg[1] = {Key:%q Value:%q}, want key=value limit=10", stmt.Args[1].Key, stmt.Args[1].Value)
	}
	if len(stmt.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(stmt.Fields))
	}
}

func TestSearchResultTypes(t *testing.T) {
	// Verify SearchResult/Source types are constructable
	sr := SearchResult{
		Source:  Source{Path: "EPIC-1/README.md", Line: 5},
		Content: "# Epic Title",
		IsMatch: true,
	}
	if sr.Source.Path != "EPIC-1/README.md" {
		t.Errorf("source path = %q", sr.Source.Path)
	}
	if sr.Source.Line != 5 {
		t.Errorf("source line = %d", sr.Source.Line)
	}
	if !sr.IsMatch {
		t.Error("IsMatch should be true")
	}
}

func TestSearchOptionsTypes(t *testing.T) {
	opts := SearchOptions{
		FileGlob:        "*.md",
		CaseInsensitive: true,
		ContextLines:    3,
	}
	if opts.FileGlob != "*.md" {
		t.Errorf("FileGlob = %q", opts.FileGlob)
	}
	if !opts.CaseInsensitive {
		t.Error("CaseInsensitive should be true")
	}
	if opts.ContextLines != 3 {
		t.Errorf("ContextLines = %d", opts.ContextLines)
	}
}
