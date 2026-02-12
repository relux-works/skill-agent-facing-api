package agentquery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatCompact_ListOfMaps(t *testing.T) {
	items := []map[string]any{
		{"id": "T1", "name": "alpha", "status": "open"},
		{"id": "T2", "name": "beta", "status": "closed"},
		{"id": "T3", "name": "gamma", "status": "open"},
	}

	out, err := FormatCompact(items, []string{"id", "name", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (1 header + 3 rows), got %d: %v", len(lines), lines)
	}

	if lines[0] != "id,name,status" {
		t.Errorf("header = %q, want %q", lines[0], "id,name,status")
	}
	if lines[1] != "T1,alpha,open" {
		t.Errorf("row 1 = %q, want %q", lines[1], "T1,alpha,open")
	}
	if lines[2] != "T2,beta,closed" {
		t.Errorf("row 2 = %q, want %q", lines[2], "T2,beta,closed")
	}
	if lines[3] != "T3,gamma,open" {
		t.Errorf("row 3 = %q, want %q", lines[3], "T3,gamma,open")
	}
}

func TestFormatCompact_ListOfAny(t *testing.T) {
	// []any where all elements are maps — should produce tabular output.
	items := []any{
		map[string]any{"id": "T1", "score": 10},
		map[string]any{"id": "T2", "score": 20},
	}

	out, err := FormatCompact(items, []string{"id", "score"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "id,score" {
		t.Errorf("header = %q, want %q", lines[0], "id,score")
	}
	if lines[1] != "T1,10" {
		t.Errorf("row 1 = %q, want %q", lines[1], "T1,10")
	}
}

func TestFormatCompact_SingleMap(t *testing.T) {
	m := map[string]any{
		"id":     "T1",
		"name":   "alpha",
		"status": "open",
	}

	out, err := FormatCompact(m, []string{"id", "name", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "id:T1" {
		t.Errorf("line 0 = %q, want %q", lines[0], "id:T1")
	}
	if lines[1] != "name:alpha" {
		t.Errorf("line 1 = %q, want %q", lines[1], "name:alpha")
	}
	if lines[2] != "status:open" {
		t.Errorf("line 2 = %q, want %q", lines[2], "status:open")
	}
}

func TestFormatCompact_ErrorMap_FallsBackToJSON(t *testing.T) {
	m := map[string]any{
		"error": map[string]any{
			"message": "something went wrong",
		},
	}

	out, err := FormatCompact(m, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON (not key:value format).
	if !json.Valid(out) {
		t.Fatalf("expected JSON for error map, got: %s", string(out))
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, hasError := parsed["error"]; !hasError {
		t.Error("expected 'error' key in JSON output")
	}
}

func TestFormatCompact_NonMapResult_FallsBackToJSON(t *testing.T) {
	// String result (e.g. from a custom operation) — should JSON-marshal.
	out, err := FormatCompact("hello world", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(out) {
		t.Fatalf("expected JSON for string result, got: %s", string(out))
	}

	var s string
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if s != "hello world" {
		t.Errorf("result = %q, want %q", s, "hello world")
	}
}

func TestFormatCompact_IntResult_FallsBackToJSON(t *testing.T) {
	out, err := FormatCompact(42, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(out) != "42" {
		t.Errorf("result = %q, want %q", string(out), "42")
	}
}

func TestFormatCompact_ValuesWithCommas(t *testing.T) {
	items := []map[string]any{
		{"name": "hello, world", "id": "T1"},
	}

	out, err := FormatCompact(items, []string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "id,name" {
		t.Errorf("header = %q, want %q", lines[0], "id,name")
	}
	// Value with comma should be quoted.
	if lines[1] != `T1,"hello, world"` {
		t.Errorf("row = %q, want %q", lines[1], `T1,"hello, world"`)
	}
}

func TestFormatCompact_ValuesWithQuotes(t *testing.T) {
	items := []map[string]any{
		{"desc": `he said "hello"`, "id": "T1"},
	}

	out, err := FormatCompact(items, []string{"id", "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Quotes should be doubled and the whole field quoted.
	if lines[1] != `T1,"he said ""hello"""` {
		t.Errorf("row = %q, want %q", lines[1], `T1,"he said ""hello"""`)
	}
}

func TestFormatCompact_ValuesWithNewlines(t *testing.T) {
	items := []map[string]any{
		{"desc": "line1\nline2", "id": "T1"},
	}

	out, err := FormatCompact(items, []string{"id", "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	// The newline in the value should be inside quotes.
	// The CSV field itself spans two raw lines but is one logical field.
	// After split we get: header, `T1,"line1`, `line2"`.
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d: %v", len(lines), lines)
	}
	// Verify the raw bytes contain the quoted value.
	raw := string(out)
	if !strings.Contains(raw, `"line1`) {
		t.Errorf("expected quoted multiline value, got: %s", raw)
	}
}

func TestFormatCompact_NilValues(t *testing.T) {
	items := []map[string]any{
		{"id": "T1", "name": nil},
	}

	out, err := FormatCompact(items, []string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	// Nil should produce empty string.
	if lines[1] != "T1," {
		t.Errorf("row = %q, want %q", lines[1], "T1,")
	}
}

func TestFormatCompact_MissingFieldInMap(t *testing.T) {
	// If fieldOrder includes a field not present in the map, should be empty.
	items := []map[string]any{
		{"id": "T1"},
	}

	out, err := FormatCompact(items, []string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if lines[1] != "T1," {
		t.Errorf("row = %q, want %q", lines[1], "T1,")
	}
}

func TestFormatCompact_EmptyList(t *testing.T) {
	items := []map[string]any{}

	out, err := FormatCompact(items, []string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (header only), got %d: %v", len(lines), lines)
	}
	if lines[0] != "id,name" {
		t.Errorf("header = %q, want %q", lines[0], "id,name")
	}
}

func TestFormatCompact_SliceValues(t *testing.T) {
	items := []map[string]any{
		{"id": "T1", "tags": []string{"go", "rust"}},
	}

	out, err := FormatCompact(items, []string{"id", "tags"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	// Tags is a slice — should be JSON-encoded, and since it contains commas
	// (from the JSON array brackets with quotes), it should be CSV-quoted.
	row := lines[1]
	if !strings.HasPrefix(row, "T1,") {
		t.Errorf("row should start with T1, got: %q", row)
	}
	// The tags value should contain ["go","rust"] in some form.
	if !strings.Contains(row, "go") || !strings.Contains(row, "rust") {
		t.Errorf("row should contain tags, got: %q", row)
	}
}

func TestFormatCompact_FieldOrderRespected(t *testing.T) {
	items := []map[string]any{
		{"a": 1, "b": 2, "c": 3},
	}

	// Explicit order: c, a, b
	out, err := FormatCompact(items, []string{"c", "a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if lines[0] != "c,a,b" {
		t.Errorf("header = %q, want %q", lines[0], "c,a,b")
	}
	if lines[1] != "3,1,2" {
		t.Errorf("row = %q, want %q", lines[1], "3,1,2")
	}
}

func TestFormatCompact_SingleMap_NilValue(t *testing.T) {
	m := map[string]any{
		"id":   "T1",
		"name": nil,
	}

	out, err := FormatCompact(m, []string{"id", "name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "id:T1" {
		t.Errorf("line 0 = %q, want %q", lines[0], "id:T1")
	}
	if lines[1] != "name:" {
		t.Errorf("line 1 = %q, want %q", lines[1], "name:")
	}
}

func TestFormatCompact_SingleMap_ValueWithNewline(t *testing.T) {
	m := map[string]any{
		"desc": "line1\nline2",
	}

	out, err := FormatCompact(m, []string{"desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (newlines escaped), got %d: %v", len(lines), lines)
	}
	if lines[0] != `desc:line1\nline2` {
		t.Errorf("line = %q, want %q", lines[0], `desc:line1\nline2`)
	}
}

func TestFormatCompact_NoFieldOrder_ListDeriveFromFirstItem(t *testing.T) {
	items := []map[string]any{
		{"b": 2, "a": 1},
	}

	// No field order — should derive from first item's keys (sorted).
	out, err := FormatCompact(items, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	// Keys sorted: a, b
	if lines[0] != "a,b" {
		t.Errorf("header = %q, want %q", lines[0], "a,b")
	}
	if lines[1] != "1,2" {
		t.Errorf("row = %q, want %q", lines[1], "1,2")
	}
}

func TestFormatCompact_NoFieldOrder_SingleMapSorted(t *testing.T) {
	m := map[string]any{
		"c": 3,
		"a": 1,
		"b": 2,
	}

	out, err := FormatCompact(m, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "a:1" {
		t.Errorf("line 0 = %q, want %q", lines[0], "a:1")
	}
	if lines[1] != "b:2" {
		t.Errorf("line 1 = %q, want %q", lines[1], "b:2")
	}
	if lines[2] != "c:3" {
		t.Errorf("line 2 = %q, want %q", lines[2], "c:3")
	}
}

func TestFormatCompact_MixedListFallsBackToJSON(t *testing.T) {
	// []any where not all elements are maps — should fall back to JSON.
	items := []any{
		map[string]any{"id": "T1"},
		"not a map",
	}

	out, err := FormatCompact(items, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(out) {
		t.Fatalf("expected JSON for mixed list, got: %s", string(out))
	}
}

func TestEscapeCSV(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, ""},
		{"simple string", "hello", "hello"},
		{"string with comma", "a,b", `"a,b"`},
		{"string with quote", `a"b`, `"a""b"`},
		{"string with newline", "a\nb", `"a` + "\n" + `b"`},
		{"integer", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool", true, "true"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeCSV(tt.val)
			if got != tt.want {
				t.Errorf("escapeCSV(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestEscapeKV(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, ""},
		{"simple string", "hello", "hello"},
		{"string with newline", "a\nb", `a\nb`},
		{"string with carriage return", "a\rb", `a\rb`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeKV(tt.val)
			if got != tt.want {
				t.Errorf("escapeKV(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool", true, "true"},
		{"string slice", []string{"a", "b"}, `["a","b"]`},
		{"int slice", []int{1, 2, 3}, `[1,2,3]`},
		{"nil slice", []string(nil), "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.val)
			if got != tt.want {
				t.Errorf("formatValue(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestMapKeys_Sorted(t *testing.T) {
	m := map[string]any{"c": 3, "a": 1, "b": 2}
	keys := mapKeys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("keys = %v, want [a b c]", keys)
	}
}
