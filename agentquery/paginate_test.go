package agentquery

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- Unit tests for ParseSkipTake ---

func TestParseSkipTake_Defaults(t *testing.T) {
	skip, take, err := ParseSkipTake(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 0 {
		t.Errorf("skip = %d, want 0", skip)
	}
	if take != 0 {
		t.Errorf("take = %d, want 0", take)
	}
}

func TestParseSkipTake_SkipOnly(t *testing.T) {
	args := []Arg{{Key: "skip", Value: "2"}}
	skip, take, err := ParseSkipTake(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 2 {
		t.Errorf("skip = %d, want 2", skip)
	}
	if take != 0 {
		t.Errorf("take = %d, want 0 (no limit)", take)
	}
}

func TestParseSkipTake_TakeOnly(t *testing.T) {
	args := []Arg{{Key: "take", Value: "5"}}
	skip, take, err := ParseSkipTake(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 0 {
		t.Errorf("skip = %d, want 0", skip)
	}
	if take != 5 {
		t.Errorf("take = %d, want 5", take)
	}
}

func TestParseSkipTake_Both(t *testing.T) {
	args := []Arg{
		{Key: "skip", Value: "1"},
		{Key: "take", Value: "3"},
	}
	skip, take, err := ParseSkipTake(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("skip = %d, want 1", skip)
	}
	if take != 3 {
		t.Errorf("take = %d, want 3", take)
	}
}

func TestParseSkipTake_SkipZero(t *testing.T) {
	args := []Arg{{Key: "skip", Value: "0"}}
	skip, _, err := ParseSkipTake(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 0 {
		t.Errorf("skip = %d, want 0", skip)
	}
}

func TestParseSkipTake_NegativeSkip(t *testing.T) {
	args := []Arg{{Key: "skip", Value: "-1"}}
	_, _, err := ParseSkipTake(args)
	if err == nil {
		t.Fatal("expected error for negative skip")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
	if !strings.Contains(e.Message, "skip") {
		t.Errorf("error message %q should mention 'skip'", e.Message)
	}
}

func TestParseSkipTake_NegativeTake(t *testing.T) {
	args := []Arg{{Key: "take", Value: "-5"}}
	_, _, err := ParseSkipTake(args)
	if err == nil {
		t.Fatal("expected error for negative take")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
	if !strings.Contains(e.Message, "take") {
		t.Errorf("error message %q should mention 'take'", e.Message)
	}
}

func TestParseSkipTake_ZeroTake(t *testing.T) {
	args := []Arg{{Key: "take", Value: "0"}}
	_, _, err := ParseSkipTake(args)
	if err == nil {
		t.Fatal("expected error for take=0")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
}

func TestParseSkipTake_NonIntegerSkip(t *testing.T) {
	args := []Arg{{Key: "skip", Value: "abc"}}
	_, _, err := ParseSkipTake(args)
	if err == nil {
		t.Fatal("expected error for non-integer skip")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
}

func TestParseSkipTake_NonIntegerTake(t *testing.T) {
	args := []Arg{{Key: "take", Value: "xyz"}}
	_, _, err := ParseSkipTake(args)
	if err == nil {
		t.Fatal("expected error for non-integer take")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrValidation {
		t.Errorf("code = %s, want %s", e.Code, ErrValidation)
	}
}

func TestParseSkipTake_IgnoresNonPaginationArgs(t *testing.T) {
	args := []Arg{
		{Key: "status", Value: "done"},
		{Key: "skip", Value: "1"},
		{Key: "assignee", Value: "alice"},
		{Key: "take", Value: "2"},
	}
	skip, take, err := ParseSkipTake(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("skip = %d, want 1", skip)
	}
	if take != 2 {
		t.Errorf("take = %d, want 2", take)
	}
}

// --- Unit tests for PaginateSlice ---

func TestPaginateSlice_NoArgs(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	result, err := PaginateSlice(items, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 5 {
		t.Errorf("len = %d, want 5", len(result))
	}
}

func TestPaginateSlice_BasicSkip(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	args := []Arg{{Key: "skip", Value: "2"}}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0] != "c" || result[1] != "d" || result[2] != "e" {
		t.Errorf("result = %v, want [c d e]", result)
	}
}

func TestPaginateSlice_BasicTake(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	args := []Arg{{Key: "take", Value: "3"}}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("result = %v, want [a b c]", result)
	}
}

func TestPaginateSlice_SkipPlusTake(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	args := []Arg{
		{Key: "skip", Value: "1"},
		{Key: "take", Value: "2"},
	}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != "b" || result[1] != "c" {
		t.Errorf("result = %v, want [b c]", result)
	}
}

func TestPaginateSlice_SkipPastEnd(t *testing.T) {
	items := []string{"a", "b", "c"}
	args := []Arg{{Key: "skip", Value: "10"}}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("len = %d, want 0 (skip past end)", len(result))
	}
}

func TestPaginateSlice_SkipExactLength(t *testing.T) {
	items := []string{"a", "b", "c"}
	args := []Arg{{Key: "skip", Value: "3"}}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("len = %d, want 0 (skip == len)", len(result))
	}
}

func TestPaginateSlice_TakeMoreThanAvailable(t *testing.T) {
	items := []string{"a", "b", "c"}
	args := []Arg{{Key: "take", Value: "100"}}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("len = %d, want 3 (take > len)", len(result))
	}
}

func TestPaginateSlice_SkipPlusTakeMoreThanRemaining(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	args := []Arg{
		{Key: "skip", Value: "3"},
		{Key: "take", Value: "10"},
	}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != "d" || result[1] != "e" {
		t.Errorf("result = %v, want [d e]", result)
	}
}

func TestPaginateSlice_SkipZeroNoop(t *testing.T) {
	items := []string{"a", "b", "c"}
	args := []Arg{{Key: "skip", Value: "0"}}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("len = %d, want 3 (skip=0 is noop)", len(result))
	}
}

func TestPaginateSlice_EmptySlice(t *testing.T) {
	var items []string
	args := []Arg{
		{Key: "skip", Value: "0"},
		{Key: "take", Value: "5"},
	}
	result, err := PaginateSlice(items, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("len = %d, want 0", len(result))
	}
}

func TestPaginateSlice_NegativeSkipError(t *testing.T) {
	items := []string{"a", "b"}
	args := []Arg{{Key: "skip", Value: "-1"}}
	_, err := PaginateSlice(items, args)
	if err == nil {
		t.Fatal("expected error for negative skip")
	}
}

func TestPaginateSlice_NegativeTakeError(t *testing.T) {
	items := []string{"a", "b"}
	args := []Arg{{Key: "take", Value: "-1"}}
	_, err := PaginateSlice(items, args)
	if err == nil {
		t.Fatal("expected error for negative take")
	}
}

// --- Integration: pagination with schema queries ---

// newPaginationSchema creates a Schema with list supporting skip/take + status filter.
func newPaginationSchema() *Schema[*testItem] {
	s := newTestSchema()

	items := []*testItem{
		{ID: "T1", Name: "alpha", Status: "open", Score: 10, Tags: []string{"go"}},
		{ID: "T2", Name: "beta", Status: "closed", Score: 20, Tags: []string{"rust"}},
		{ID: "T3", Name: "gamma", Status: "open", Score: 30, Tags: nil},
		{ID: "T4", Name: "delta", Status: "open", Score: 40, Tags: []string{"go", "wasm"}},
		{ID: "T5", Name: "epsilon", Status: "closed", Score: 50, Tags: []string{"rust", "wasm"}},
	}
	s.SetLoader(func() ([]*testItem, error) {
		return items, nil
	})

	// list with optional status filter + skip/take pagination
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		// Filter by status if provided
		var filterStatus string
		for _, arg := range ctx.Statement.Args {
			if arg.Key == "status" {
				filterStatus = arg.Value
			}
		}
		if filterStatus != "" {
			var filtered []*testItem
			for _, item := range data {
				if strings.EqualFold(item.Status, filterStatus) {
					filtered = append(filtered, item)
				}
			}
			data = filtered
		}
		// Apply pagination
		data, err = PaginateSlice(data, ctx.Statement.Args)
		if err != nil {
			return nil, err
		}
		var out []map[string]any
		for _, item := range data {
			out = append(out, ctx.Selector.Apply(item))
		}
		return out, nil
	})

	return s
}

func TestPagination_BasicSkipQuery(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(skip=2) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (skip 2 of 5), got %d", len(items))
	}
	if items[0]["id"] != "T3" {
		t.Errorf("items[0].id = %v, want T3", items[0]["id"])
	}
}

func TestPagination_BasicTakeQuery(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(take=2) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["id"] != "T1" {
		t.Errorf("items[0].id = %v, want T1", items[0]["id"])
	}
	if items[1]["id"] != "T2" {
		t.Errorf("items[1].id = %v, want T2", items[1]["id"])
	}
}

func TestPagination_SkipTakeComboQuery(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(skip=1, take=2) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["id"] != "T2" {
		t.Errorf("items[0].id = %v, want T2", items[0]["id"])
	}
	if items[1]["id"] != "T3" {
		t.Errorf("items[1].id = %v, want T3", items[1]["id"])
	}
}

func TestPagination_SkipPastEndQuery(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(skip=100) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items (skip past end), got %d", len(items))
	}
}

func TestPagination_TakeMoreThanAvailableQuery(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(take=999) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items (take > len), got %d", len(items))
	}
}

func TestPagination_SkipZeroNoopQuery(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(skip=0) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items (skip=0 is noop), got %d", len(items))
	}
}

func TestPagination_NegativeSkipErrorQuery(t *testing.T) {
	s := newPaginationSchema()

	// Negative values must be quoted because the DSL tokenizer doesn't allow
	// unquoted negative numbers (- is not a valid identifier start).
	result, err := s.Query(`list(skip="-1") { id }`)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	// Handler error -> error map in result
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected error map, got %T", result)
	}
	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected 'error' key in result")
	}
	errMap := errObj.(map[string]any)
	msg := errMap["message"].(string)
	if !strings.Contains(msg, "skip") {
		t.Errorf("error message %q should mention 'skip'", msg)
	}
}

func TestPagination_NegativeTakeErrorQuery(t *testing.T) {
	s := newPaginationSchema()

	// Negative values must be quoted (see NegativeSkipErrorQuery comment).
	result, err := s.Query(`list(take="-5") { id }`)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected error map, got %T", result)
	}
	errObj, exists := m["error"]
	if !exists {
		t.Fatal("expected 'error' key in result")
	}
	errMap := errObj.(map[string]any)
	msg := errMap["message"].(string)
	if !strings.Contains(msg, "take") {
		t.Errorf("error message %q should mention 'take'", msg)
	}
}

func TestPagination_WithFilterQuery(t *testing.T) {
	s := newPaginationSchema()

	// 3 open items: T1, T3, T4. Skip 1, take 1 -> T3 only.
	result, err := s.Query("list(status=open, skip=1, take=1) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["id"] != "T3" {
		t.Errorf("items[0].id = %v, want T3", items[0]["id"])
	}
}

func TestPagination_WithFilterSkipPastFilteredEnd(t *testing.T) {
	s := newPaginationSchema()

	// 2 closed items: T2, T5. Skip 5 -> empty.
	result, err := s.Query("list(status=closed, skip=5) { id }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items (skip past filtered end), got %d", len(items))
	}
}

func TestPagination_WithFieldProjection(t *testing.T) {
	s := newPaginationSchema()

	// Take 2, project to id + score only.
	result, err := s.Query("list(take=2) { id score }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Check field projection: only id and score.
	for i, item := range items {
		if _, exists := item["id"]; !exists {
			t.Errorf("items[%d] missing 'id'", i)
		}
		if _, exists := item["score"]; !exists {
			t.Errorf("items[%d] missing 'score'", i)
		}
		if _, exists := item["name"]; exists {
			t.Errorf("items[%d] has 'name' (not in projection)", i)
		}
		if _, exists := item["status"]; exists {
			t.Errorf("items[%d] has 'status' (not in projection)", i)
		}
	}

	// Check values.
	if items[0]["id"] != "T1" || items[0]["score"] != 10 {
		t.Errorf("items[0] = %v, want {id:T1 score:10}", items[0])
	}
	if items[1]["id"] != "T2" || items[1]["score"] != 20 {
		t.Errorf("items[1] = %v, want {id:T2 score:20}", items[1])
	}
}

func TestPagination_WithPresetProjection(t *testing.T) {
	s := newPaginationSchema()

	result, err := s.Query("list(skip=3, take=2) { minimal }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// minimal preset = id, status
	if items[0]["id"] != "T4" {
		t.Errorf("items[0].id = %v, want T4", items[0]["id"])
	}
	if items[0]["status"] != "open" {
		t.Errorf("items[0].status = %v, want open", items[0]["status"])
	}
	if _, exists := items[0]["name"]; exists {
		t.Error("items[0] should not have 'name' (minimal preset)")
	}
}

func TestPagination_CompactOutput(t *testing.T) {
	s := newPaginationSchema()

	data, err := s.QueryJSONWithMode("list(skip=1, take=2) { id status }", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	// Header + 2 data rows
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "id,status" {
		t.Errorf("header = %q, want %q", lines[0], "id,status")
	}
	if lines[1] != "T2,closed" {
		t.Errorf("row 1 = %q, want %q", lines[1], "T2,closed")
	}
	if lines[2] != "T3,open" {
		t.Errorf("row 2 = %q, want %q", lines[2], "T3,open")
	}

	// Should NOT be valid JSON.
	if json.Valid(data) {
		t.Error("compact output should not be valid JSON")
	}
}

func TestPagination_CompactOutputWithFilter(t *testing.T) {
	s := newPaginationSchema()

	// 3 open: T1, T3, T4. Take first 2.
	data, err := s.QueryJSONWithMode("list(status=open, take=2) { id }", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	// Header + 2 rows
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "id" {
		t.Errorf("header = %q, want %q", lines[0], "id")
	}
	if lines[1] != "T1" {
		t.Errorf("row 1 = %q, want %q", lines[1], "T1")
	}
	if lines[2] != "T3" {
		t.Errorf("row 2 = %q, want %q", lines[2], "T3")
	}
}

func TestPagination_JSONOutput(t *testing.T) {
	s := newPaginationSchema()

	data, err := s.QueryJSON("list(skip=2, take=1) { id name }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("QueryJSON should return valid JSON, got: %s", string(data))
	}

	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["id"] != "T3" {
		t.Errorf("items[0].id = %v, want T3", items[0]["id"])
	}
	if items[0]["name"] != "gamma" {
		t.Errorf("items[0].name = %v, want gamma", items[0]["name"])
	}
}

// --- Table-driven tests ---

func TestPaginateSlice_TableDriven(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	tests := []struct {
		name     string
		args     []Arg
		wantLen  int
		wantVals []string
		wantErr  string
	}{
		{
			name:     "no pagination",
			args:     nil,
			wantLen:  5,
			wantVals: []string{"a", "b", "c", "d", "e"},
		},
		{
			name:     "skip only",
			args:     []Arg{{Key: "skip", Value: "3"}},
			wantLen:  2,
			wantVals: []string{"d", "e"},
		},
		{
			name:     "take only",
			args:     []Arg{{Key: "take", Value: "2"}},
			wantLen:  2,
			wantVals: []string{"a", "b"},
		},
		{
			name:     "skip+take",
			args:     []Arg{{Key: "skip", Value: "1"}, {Key: "take", Value: "3"}},
			wantLen:  3,
			wantVals: []string{"b", "c", "d"},
		},
		{
			name:     "skip past end",
			args:     []Arg{{Key: "skip", Value: "50"}},
			wantLen:  0,
			wantVals: []string{},
		},
		{
			name:     "take more than available",
			args:     []Arg{{Key: "take", Value: "50"}},
			wantLen:  5,
			wantVals: []string{"a", "b", "c", "d", "e"},
		},
		{
			name:     "skip=0 noop",
			args:     []Arg{{Key: "skip", Value: "0"}},
			wantLen:  5,
			wantVals: []string{"a", "b", "c", "d", "e"},
		},
		{
			name:    "negative skip",
			args:    []Arg{{Key: "skip", Value: "-2"}},
			wantErr: "skip must be >= 0",
		},
		{
			name:    "negative take",
			args:    []Arg{{Key: "take", Value: "-1"}},
			wantErr: "take must be > 0",
		},
		{
			name:    "take zero",
			args:    []Arg{{Key: "take", Value: "0"}},
			wantErr: "take must be > 0",
		},
		{
			name:    "non-integer skip",
			args:    []Arg{{Key: "skip", Value: "abc"}},
			wantErr: "skip must be an integer",
		},
		{
			name:    "non-integer take",
			args:    []Arg{{Key: "take", Value: "1.5"}},
			wantErr: "take must be an integer",
		},
		{
			name:     "extra args ignored",
			args:     []Arg{{Key: "foo", Value: "bar"}, {Key: "take", Value: "1"}},
			wantLen:  1,
			wantVals: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PaginateSlice(items, tt.args)
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
			if len(result) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(result), tt.wantLen)
			}
			for i, want := range tt.wantVals {
				if i >= len(result) {
					break
				}
				if result[i] != want {
					t.Errorf("result[%d] = %q, want %q", i, result[i], want)
				}
			}
		})
	}
}
