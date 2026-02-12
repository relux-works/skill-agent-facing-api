package agentquery

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- FilterItems tests ---

func TestFilterItems_All(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result := FilterItems(items, func(i int) bool { return true })
	if len(result) != 5 {
		t.Errorf("expected 5 items, got %d", len(result))
	}
}

func TestFilterItems_None(t *testing.T) {
	items := []int{1, 2, 3}
	result := FilterItems(items, func(i int) bool { return false })
	if len(result) != 0 {
		t.Errorf("expected 0 items, got %d", len(result))
	}
}

func TestFilterItems_Subset(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result := FilterItems(items, func(i int) bool { return i > 3 })
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	if result[0] != 4 || result[1] != 5 {
		t.Errorf("expected [4, 5], got %v", result)
	}
}

func TestFilterItems_Empty(t *testing.T) {
	var items []int
	result := FilterItems(items, func(i int) bool { return true })
	if len(result) != 0 {
		t.Errorf("expected 0 items from empty slice, got %d", len(result))
	}
}

func TestFilterItems_StructType(t *testing.T) {
	items := []*testItem{
		{ID: "T1", Status: "open"},
		{ID: "T2", Status: "closed"},
		{ID: "T3", Status: "open"},
	}
	result := FilterItems(items, func(item *testItem) bool {
		return item.Status == "open"
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 open items, got %d", len(result))
	}
	if result[0].ID != "T1" || result[1].ID != "T3" {
		t.Errorf("expected [T1, T3], got [%s, %s]", result[0].ID, result[1].ID)
	}
}

// --- CountItems tests ---

func TestCountItems_All(t *testing.T) {
	items := []int{1, 2, 3}
	n := CountItems(items, func(i int) bool { return true })
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}

func TestCountItems_None(t *testing.T) {
	items := []int{1, 2, 3}
	n := CountItems(items, func(i int) bool { return false })
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestCountItems_Subset(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	n := CountItems(items, func(i int) bool { return i%2 == 0 })
	if n != 2 {
		t.Errorf("expected 2 even numbers, got %d", n)
	}
}

func TestCountItems_Empty(t *testing.T) {
	var items []string
	n := CountItems(items, func(s string) bool { return true })
	if n != 0 {
		t.Errorf("expected 0 from empty slice, got %d", n)
	}
}

func TestCountItems_StructType(t *testing.T) {
	items := []*testItem{
		{ID: "T1", Status: "open"},
		{ID: "T2", Status: "closed"},
		{ID: "T3", Status: "open"},
		{ID: "T4", Status: "closed"},
	}
	n := CountItems(items, func(item *testItem) bool {
		return item.Status == "closed"
	})
	if n != 2 {
		t.Errorf("expected 2 closed items, got %d", n)
	}
}

// --- MatchAll tests ---

func TestMatchAll_AlwaysTrue(t *testing.T) {
	pred := MatchAll[int]()
	if !pred(0) || !pred(42) || !pred(-1) {
		t.Error("MatchAll should always return true")
	}
}

func TestMatchAll_WithFilterItems(t *testing.T) {
	items := []int{1, 2, 3}
	result := FilterItems(items, MatchAll[int]())
	if len(result) != 3 {
		t.Errorf("expected 3 items with MatchAll, got %d", len(result))
	}
}

func TestMatchAll_WithCountItems(t *testing.T) {
	items := []int{1, 2, 3, 4}
	n := CountItems(items, MatchAll[int]())
	if n != 4 {
		t.Errorf("expected 4 with MatchAll, got %d", n)
	}
}

// --- Count operation integration tests ---
// These test the count operation as wired up in the query schema (from query_test.go).

func TestCount_All(t *testing.T) {
	s := newQuerySchema()
	result, err := s.Query("count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["count"] != 3 {
		t.Errorf("count = %v, want 3", m["count"])
	}
}

func TestCount_ReturnsMapNotSlice(t *testing.T) {
	s := newQuerySchema()
	result, err := s.Query("count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single statement should return unwrapped map, not []any
	if _, isSlice := result.([]any); isSlice {
		t.Error("count() should return map, not []any")
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if _, hasCount := m["count"]; !hasCount {
		t.Error("result should have 'count' key")
	}
}

func TestCount_InBatchWithOtherOperations(t *testing.T) {
	s := newQuerySchema()
	result, err := s.Query("get(T1) { id }; count(); list() { id }")
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

	// First: get(T1)
	m0, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] type = %T, want map", results[0])
	}
	if m0["id"] != "T1" {
		t.Errorf("results[0].id = %v, want T1", m0["id"])
	}

	// Second: count()
	m1, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] type = %T, want map", results[1])
	}
	if m1["count"] != 3 {
		t.Errorf("results[1].count = %v, want 3", m1["count"])
	}

	// Third: list()
	items, ok := results[2].([]map[string]any)
	if !ok {
		t.Fatalf("results[2] type = %T, want []map", results[2])
	}
	if len(items) != 3 {
		t.Errorf("list returned %d items, want 3", len(items))
	}
}

func TestCount_InBatchWithFailure(t *testing.T) {
	s := newQuerySchema()
	result, err := s.Query("count(); fail(); count()")
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

	// First and third count() should succeed despite middle fail()
	m0 := results[0].(map[string]any)
	if m0["count"] != 3 {
		t.Errorf("results[0].count = %v, want 3", m0["count"])
	}
	m1 := results[1].(map[string]any)
	if _, hasError := m1["error"]; !hasError {
		t.Error("results[1] should be an error map")
	}
	m2 := results[2].(map[string]any)
	if m2["count"] != 3 {
		t.Errorf("results[2].count = %v, want 3", m2["count"])
	}
}

func TestCount_JSON(t *testing.T) {
	s := newQuerySchema()
	data, err := s.QueryJSON("count()")
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
	// JSON unmarshals numbers as float64
	if m["count"] != float64(3) {
		t.Errorf("count = %v (type %T), want 3", m["count"], m["count"])
	}
}

// --- Count compact output tests ---

func TestCount_CompactOutput(t *testing.T) {
	s := newLLMQuerySchema()
	data, err := s.QueryJSONWithMode("count()", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimRight(string(data), "\n")
	if out != "count:3" {
		t.Errorf("compact output = %q, want %q", out, "count:3")
	}
}

func TestCount_CompactOutput_InBatch(t *testing.T) {
	s := newLLMQuerySchema()
	data, err := s.QueryJSONWithMode("count(); get(T1) { id }", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := string(data)
	parts := strings.Split(out, "\n\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts separated by blank line, got %d:\n%s", len(parts), out)
	}

	// First part: count()
	part0 := strings.TrimRight(parts[0], "\n")
	if part0 != "count:3" {
		t.Errorf("part 0 = %q, want %q", part0, "count:3")
	}

	// Second part: get(T1) { id }
	part1 := strings.TrimRight(parts[1], "\n")
	if part1 != "id:T1" {
		t.Errorf("part 1 = %q, want %q", part1, "id:T1")
	}
}

// --- Count with filtered data tests ---
// These use a schema with a filter-aware count operation.

func newCountFilterSchema() *Schema[*testItem] {
	s := newTestSchema()

	items := []*testItem{
		{ID: "T1", Name: "alpha", Status: "open", Score: 10, Tags: []string{"go"}},
		{ID: "T2", Name: "beta", Status: "closed", Score: 20, Tags: []string{"rust"}},
		{ID: "T3", Name: "gamma", Status: "open", Score: 30, Tags: nil},
		{ID: "T4", Name: "delta", Status: "closed", Score: 40, Tags: []string{"go", "rust"}},
		{ID: "T5", Name: "epsilon", Status: "open", Score: 50, Tags: []string{"go"}},
	}

	s.SetLoader(func() ([]*testItem, error) {
		return items, nil
	})

	// count with status filter — mirrors the example CLI pattern
	s.Operation("count", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}

		var filterStatus string
		for _, arg := range ctx.Statement.Args {
			if arg.Key == "status" {
				filterStatus = arg.Value
			}
		}

		var pred func(*testItem) bool
		if filterStatus != "" {
			pred = func(item *testItem) bool {
				return strings.EqualFold(item.Status, filterStatus)
			}
		} else {
			pred = MatchAll[*testItem]()
		}

		n := CountItems(data, pred)
		return map[string]any{"count": n}, nil
	})

	// list with status filter for comparison
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}

		var filterStatus string
		for _, arg := range ctx.Statement.Args {
			if arg.Key == "status" {
				filterStatus = arg.Value
			}
		}

		var filtered []*testItem
		if filterStatus != "" {
			filtered = FilterItems(data, func(item *testItem) bool {
				return strings.EqualFold(item.Status, filterStatus)
			})
		} else {
			filtered = data
		}

		var out []map[string]any
		for _, item := range filtered {
			out = append(out, ctx.Selector.Apply(item))
		}
		if out == nil {
			out = []map[string]any{}
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
		return nil, &Error{Code: ErrNotFound, Message: "item not found: " + targetID}
	})

	return s
}

func TestCountFiltered_All(t *testing.T) {
	s := newCountFilterSchema()
	result, err := s.Query("count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 5 {
		t.Errorf("count = %v, want 5", m["count"])
	}
}

func TestCountFiltered_ByStatus_Open(t *testing.T) {
	s := newCountFilterSchema()
	result, err := s.Query("count(status=open)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 3 {
		t.Errorf("count(status=open) = %v, want 3", m["count"])
	}
}

func TestCountFiltered_ByStatus_Closed(t *testing.T) {
	s := newCountFilterSchema()
	result, err := s.Query("count(status=closed)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 2 {
		t.Errorf("count(status=closed) = %v, want 2", m["count"])
	}
}

func TestCountFiltered_NoMatches(t *testing.T) {
	s := newCountFilterSchema()
	result, err := s.Query("count(status=archived)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 0 {
		t.Errorf("count(status=archived) = %v, want 0", m["count"])
	}
}

func TestCountFiltered_CaseInsensitive(t *testing.T) {
	s := newCountFilterSchema()
	result, err := s.Query("count(status=OPEN)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 3 {
		t.Errorf("count(status=OPEN) = %v, want 3 (case-insensitive)", m["count"])
	}
}

func TestCountFiltered_ConsistentWithList(t *testing.T) {
	s := newCountFilterSchema()

	// count(status=open) should equal len(list(status=open))
	countResult, err := s.Query("count(status=open)")
	if err != nil {
		t.Fatalf("count error: %v", err)
	}
	countMap := countResult.(map[string]any)
	countVal := countMap["count"].(int)

	listResult, err := s.Query("list(status=open) { id }")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	listItems := listResult.([]map[string]any)

	if countVal != len(listItems) {
		t.Errorf("count(status=open) = %d, but list(status=open) has %d items", countVal, len(listItems))
	}
}

func TestCountFiltered_InBatchWithGet(t *testing.T) {
	s := newCountFilterSchema()
	result, err := s.Query("get(T1) { id }; count(status=open); count(status=closed)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := result.([]any)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// get(T1)
	m0 := results[0].(map[string]any)
	if m0["id"] != "T1" {
		t.Errorf("results[0].id = %v, want T1", m0["id"])
	}

	// count(status=open) = 3
	m1 := results[1].(map[string]any)
	if m1["count"] != 3 {
		t.Errorf("count(status=open) = %v, want 3", m1["count"])
	}

	// count(status=closed) = 2
	m2 := results[2].(map[string]any)
	if m2["count"] != 2 {
		t.Errorf("count(status=closed) = %v, want 2", m2["count"])
	}
}

func TestCountFiltered_CompactOutput(t *testing.T) {
	s := newCountFilterSchema()
	data, err := s.QueryJSONWithMode("count(status=open)", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimRight(string(data), "\n")
	if out != "count:3" {
		t.Errorf("compact output = %q, want %q", out, "count:3")
	}
}

func TestCountFiltered_CompactOutput_Zero(t *testing.T) {
	s := newCountFilterSchema()
	data, err := s.QueryJSONWithMode("count(status=archived)", LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimRight(string(data), "\n")
	if out != "count:0" {
		t.Errorf("compact output = %q, want %q", out, "count:0")
	}
}

func TestCountFiltered_JSON(t *testing.T) {
	s := newCountFilterSchema()
	data, err := s.QueryJSON("count(status=closed)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("invalid JSON: %s", string(data))
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if m["count"] != float64(2) {
		t.Errorf("count = %v, want 2", m["count"])
	}
}
