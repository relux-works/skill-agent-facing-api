package agentquery

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Integration test domain ---

type project struct {
	ID       string
	Title    string
	Status   string
	Priority int
	Labels   []string
}

func sampleProjects() []*project {
	return []*project{
		{ID: "P1", Title: "Alpha Project", Status: "active", Priority: 1, Labels: []string{"backend", "go"}},
		{ID: "P2", Title: "Beta Project", Status: "archived", Priority: 2, Labels: []string{"frontend"}},
		{ID: "P3", Title: "Gamma Project", Status: "active", Priority: 3, Labels: nil},
	}
}

// buildIntegrationSchema creates a fully-wired Schema for integration testing.
func buildIntegrationSchema(t *testing.T, dataDir string) *Schema[*project] {
	t.Helper()

	opts := []Option{
		WithExtensions(".md", ".txt"),
	}
	if dataDir != "" {
		opts = append(opts, WithDataDir(dataDir))
	}

	s := NewSchema[*project](opts...)

	// Register fields
	s.Field("id", func(p *project) any { return p.ID })
	s.Field("title", func(p *project) any { return p.Title })
	s.Field("status", func(p *project) any { return p.Status })
	s.Field("priority", func(p *project) any { return p.Priority })
	s.Field("labels", func(p *project) any { return p.Labels })

	// Register presets
	s.Preset("brief", "id", "title")
	s.Preset("all", "id", "title", "status", "priority", "labels")

	// Set defaults
	s.DefaultFields("id", "title", "status")

	// Set loader
	items := sampleProjects()
	s.SetLoader(func() ([]*project, error) {
		return items, nil
	})

	// Register operations
	s.Operation("list", func(ctx OperationContext[*project]) (any, error) {
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

	s.Operation("get", func(ctx OperationContext[*project]) (any, error) {
		if len(ctx.Statement.Args) == 0 {
			return nil, &Error{Code: ErrValidation, Message: "get requires an ID"}
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
		return nil, &Error{Code: ErrNotFound, Message: "project not found: " + targetID}
	})

	s.Operation("count", func(ctx OperationContext[*project]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(data)}, nil
	})

	s.Operation("fail", func(ctx OperationContext[*project]) (any, error) {
		return nil, &Error{Code: ErrInternal, Message: "deliberate failure"}
	})

	return s
}

// setupIntegrationDataDir creates a temp directory with files for search integration tests.
func setupIntegrationDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create some markdown files
	writeTestFile(t, dir, "project-alpha.md", "# Alpha Project\nStatus: active\nPriority: high\nTeam: backend")
	writeTestFile(t, dir, "project-beta.md", "# Beta Project\nStatus: archived\nPriority: low")
	writeTestFile(t, dir, "notes.txt", "Meeting notes\nDiscuss Alpha Project timeline\nAction items pending")

	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, sub, "details.md", "## Implementation Details\nStatus: in-progress\nBlocked by: Beta Project")

	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- Integration Tests ---

func TestIntegration_FullPipeline(t *testing.T) {
	// Full pipeline: create Schema, register everything, query
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	// Simple get with default fields
	result, err := s.Query("get(P1)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Default fields: id, title, status
	if m["id"] != "P1" {
		t.Errorf("id = %v, want P1", m["id"])
	}
	if m["title"] != "Alpha Project" {
		t.Errorf("title = %v, want Alpha Project", m["title"])
	}
	if m["status"] != "active" {
		t.Errorf("status = %v, want active", m["status"])
	}
	// Non-default fields absent
	if _, exists := m["priority"]; exists {
		t.Error("priority should not be in result (not in defaults)")
	}
	if _, exists := m["labels"]; exists {
		t.Error("labels should not be in result (not in defaults)")
	}
}

func TestIntegration_QueryWithFieldProjection(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	// Explicit projection overrides defaults
	result, err := s.Query("get(P1) { id priority labels }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if m["id"] != "P1" {
		t.Errorf("id = %v, want P1", m["id"])
	}
	if m["priority"] != 1 {
		t.Errorf("priority = %v, want 1", m["priority"])
	}

	labels, ok := m["labels"].([]string)
	if !ok {
		t.Fatalf("labels type = %T, want []string", m["labels"])
	}
	if len(labels) != 2 || labels[0] != "backend" {
		t.Errorf("labels = %v, want [backend go]", labels)
	}

	// title and status should be absent (not in projection)
	if _, exists := m["title"]; exists {
		t.Error("title should not be in result")
	}
	if _, exists := m["status"]; exists {
		t.Error("status should not be in result")
	}
}

func TestIntegration_QueryWithPreset(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	result, err := s.Query("get(P2) { all }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// All fields should be present
	for _, field := range []string{"id", "title", "status", "priority", "labels"} {
		if _, exists := m[field]; !exists {
			t.Errorf("missing field %q in result with 'all' preset", field)
		}
	}
}

func TestIntegration_ListWithProjection(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	result, err := s.Query("list() { brief }")
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

	for i, item := range items {
		if _, exists := item["id"]; !exists {
			t.Errorf("item[%d] missing 'id'", i)
		}
		if _, exists := item["title"]; !exists {
			t.Errorf("item[%d] missing 'title'", i)
		}
		// Should NOT have status/priority/labels
		if _, exists := item["status"]; exists {
			t.Errorf("item[%d] has 'status' (not in 'brief' preset)", i)
		}
	}
}

func TestIntegration_BatchMixedResults(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	// Batch: get, count, fail, get
	result, err := s.Query("get(P1) { id }; count(); fail(); get(P3) { id status }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any for batch, got %T", result)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Result 0: get(P1)
	m0, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] type = %T, want map", results[0])
	}
	if m0["id"] != "P1" {
		t.Errorf("results[0].id = %v, want P1", m0["id"])
	}

	// Result 1: count()
	m1, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] type = %T, want map", results[1])
	}
	if m1["count"] != 3 {
		t.Errorf("results[1].count = %v, want 3", m1["count"])
	}

	// Result 2: fail() — should be error map, NOT abort batch
	m2, ok := results[2].(map[string]any)
	if !ok {
		t.Fatalf("results[2] type = %T, want map", results[2])
	}
	if _, exists := m2["error"]; !exists {
		t.Error("results[2] should have 'error' key")
	}

	// Result 3: get(P3) — should succeed despite earlier failure
	m3, ok := results[3].(map[string]any)
	if !ok {
		t.Fatalf("results[3] type = %T, want map", results[3])
	}
	if m3["id"] != "P3" {
		t.Errorf("results[3].id = %v, want P3", m3["id"])
	}
	if m3["status"] != "active" {
		t.Errorf("results[3].status = %v, want active", m3["status"])
	}
}

func TestIntegration_SearchViaSchema(t *testing.T) {
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	// Search .md and .txt files for "Alpha Project"
	results, err := s.Search("Alpha Project", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find matches in project-alpha.md and notes.txt
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	paths := make(map[string]bool)
	for _, r := range results {
		paths[r.Source.Path] = true
		if !r.IsMatch {
			t.Errorf("expected IsMatch=true for all results without context")
		}
	}

	if !paths["project-alpha.md"] {
		t.Error("expected match in project-alpha.md")
	}
	if !paths["notes.txt"] {
		t.Error("expected match in notes.txt")
	}
}

func TestIntegration_SearchWithOptions(t *testing.T) {
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	// Case-insensitive search with context, filtered to .md only
	results, err := s.Search("status", SearchOptions{
		CaseInsensitive: true,
		ContextLines:    1,
		FileGlob:        "*.md",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results for case-insensitive 'status' search")
	}

	// All results should be from .md files
	for _, r := range results {
		if filepath.Ext(r.Source.Path) != ".md" {
			t.Errorf("non-.md result: %s", r.Source.Path)
		}
	}

	// Should have context lines (IsMatch=false)
	hasContext := false
	for _, r := range results {
		if !r.IsMatch {
			hasContext = true
			break
		}
	}
	if !hasContext {
		t.Error("expected context lines in results with ContextLines=1")
	}
}

func TestIntegration_QueryJSON_Valid(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	// Single query
	data, err := s.QueryJSON("get(P1) { id title }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("QueryJSON returned invalid JSON: %s", string(data))
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if m["id"] != "P1" {
		t.Errorf("id = %v, want P1", m["id"])
	}
}

func TestIntegration_QueryJSON_Batch(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	data, err := s.QueryJSON("get(P1) { id }; count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("QueryJSON returned invalid JSON: %s", string(data))
	}

	var results []any
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("failed to unmarshal batch: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestIntegration_SearchJSON_Valid(t *testing.T) {
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	data, err := s.SearchJSON("Alpha", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("SearchJSON returned invalid JSON: %s", string(data))
	}

	var results []SearchResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 search result")
	}
}

func TestIntegration_SearchJSON_Empty(t *testing.T) {
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	data, err := s.SearchJSON("zzz_nonexistent_pattern_zzz", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("SearchJSON returned invalid JSON: %s", string(data))
	}

	var results []SearchResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestIntegration_NoLoader(t *testing.T) {
	s := NewSchema[*project]()
	s.Field("id", func(p *project) any { return p.ID })
	// No SetLoader call

	var itemsFn func() ([]*project, error)
	s.Operation("check", func(ctx OperationContext[*project]) (any, error) {
		itemsFn = ctx.Items
		return "ok", nil
	})

	result, err := s.Query("check()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "ok" {
		t.Errorf("result = %v, want ok", result)
	}

	// Items should be nil when no loader is set
	if itemsFn != nil {
		t.Error("Items function should be nil when no loader is set")
	}
}

func TestIntegration_ErrorPropagation_ParseError(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	// Empty query
	_, err := s.Query("")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}

	// Syntax error
	_, err = s.Query("get T1")
	if err == nil {
		t.Fatal("expected error for syntax error")
	}
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
}

func TestIntegration_ErrorPropagation_UnknownOperation(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	_, err := s.Query("unknown_op()")
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}

	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
	if !strings.Contains(pe.Message, "unknown operation") {
		t.Errorf("error = %v, want 'unknown operation'", pe.Message)
	}
}

func TestIntegration_ErrorPropagation_UnknownField(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	_, err := s.Query("get(P1) { nonexistent_field }")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error = %v, want 'unknown field'", err)
	}
}

func TestIntegration_ErrorPropagation_SearchRegexError(t *testing.T) {
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	_, err := s.Search("[invalid(regex", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}

	agentErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if agentErr.Code != ErrParse {
		t.Errorf("error code = %q, want %q", agentErr.Code, ErrParse)
	}
}

func TestIntegration_ErrorPropagation_SearchJSONRegexError(t *testing.T) {
	dataDir := setupIntegrationDataDir(t)
	s := buildIntegrationSchema(t, dataDir)

	_, err := s.SearchJSON("[bad", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for invalid regex in SearchJSON")
	}

	agentErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if agentErr.Code != ErrParse {
		t.Errorf("error code = %q, want %q", agentErr.Code, ErrParse)
	}
}

func TestIntegration_SchemaOptions(t *testing.T) {
	// Verify WithDataDir and WithExtensions work correctly
	dir := t.TempDir()
	writeTestFile(t, dir, "data.json", `{"key": "value"}`)
	writeTestFile(t, dir, "notes.md", "Some markdown notes")

	s := NewSchema[*project](
		WithDataDir(dir),
		WithExtensions(".json"),
	)

	// Search should only find .json files
	results, err := s.Search("key", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result from .json file, got %d", len(results))
	}
	if results[0].Source.Path != "data.json" {
		t.Errorf("path = %s, want data.json", results[0].Source.Path)
	}
}

func TestIntegration_DefaultExtensions(t *testing.T) {
	// Without WithExtensions, default should be [".md"]
	dir := t.TempDir()
	writeTestFile(t, dir, "readme.md", "Markdown content here")
	writeTestFile(t, dir, "readme.txt", "Text content here")

	s := NewSchema[*project](
		WithDataDir(dir),
	)

	results, err := s.Search("content", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only .md file should match
	if len(results) != 1 {
		t.Fatalf("expected 1 result (default .md extension), got %d", len(results))
	}
	if results[0].Source.Path != "readme.md" {
		t.Errorf("path = %s, want readme.md", results[0].Source.Path)
	}
}

func TestIntegration_HandlerError_ReturnsErrorMap(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	// Single failing operation returns error map (not Go error)
	result, err := s.Query("fail()")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
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
		t.Fatalf("error value type = %T, want map", errObj)
	}

	msg, ok := errMap["message"].(string)
	if !ok || !strings.Contains(msg, "deliberate failure") {
		t.Errorf("error message = %q, want 'deliberate failure'", msg)
	}
}

func TestIntegration_NotFound_ReturnsErrorMap(t *testing.T) {
	s := buildIntegrationSchema(t, "")

	result, err := s.Query("get(NONEXISTENT)")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected 'error' key in result")
	}

	errMap, ok := errObj.(map[string]any)
	if !ok {
		t.Fatalf("error type = %T", errObj)
	}

	msg := errMap["message"].(string)
	if !strings.Contains(msg, "not found") {
		t.Errorf("error message = %q, want 'not found'", msg)
	}
}
