package agentquery

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

// newMutationSchema creates a Schema with standard fields, operations, and
// mutation registrations for testing.
func newMutationSchema() *Schema[*testItem] {
	s := newTestSchema()

	items := testData()
	s.SetLoader(func() ([]*testItem, error) {
		return items, nil
	})

	// Read operation for mixed-batch tests.
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

	s.Operation("count", func(ctx OperationContext[*testItem]) (any, error) {
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(data)}, nil
	})

	// Simple mutation without metadata — no framework validation.
	s.Mutation("update", func(ctx MutationContext[*testItem]) (any, error) {
		return map[string]any{
			"id":     ctx.Args[0].Value,
			"status": ctx.ArgMap["status"],
		}, nil
	})

	// Mutation with metadata — framework validates required/enum.
	s.MutationWithMetadata("create", func(ctx MutationContext[*testItem]) (any, error) {
		title := ctx.ArgMap["title"]
		status := ctx.ArgMap["status"]
		if status == "" {
			status = "todo"
		}
		if ctx.DryRun {
			return map[string]any{
				"dry_run":      true,
				"would_create": map[string]any{"title": title, "status": status},
			}, nil
		}
		return map[string]any{"id": "new-1", "title": title, "status": status}, nil
	}, MutationMetadata{
		Description: "Create a new task",
		Parameters: []ParameterDef{
			{Name: "title", Type: "string", Required: true, Description: "Task title"},
			{Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}, Default: "todo"},
		},
		Examples: []string{`create(title="Fix bug")`},
	})

	// Mutation that returns an error from the handler.
	s.Mutation("fail-mutation", func(ctx MutationContext[*testItem]) (any, error) {
		return nil, errors.New("domain error: cannot process")
	})

	return s
}

// --- Mutation registration and basic execution ---

func TestMutation_BasicExecution(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query(`update(T1, status=done)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr, ok := result.(MutationResult)
	if !ok {
		t.Fatalf("expected MutationResult, got %T", result)
	}
	if !mr.Ok {
		t.Fatalf("expected ok=true, got errors: %v", mr.Errors)
	}

	m, ok := mr.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", mr.Result)
	}
	if m["id"] != "T1" {
		t.Errorf("id = %v, want T1", m["id"])
	}
	if m["status"] != "done" {
		t.Errorf("status = %v, want done", m["status"])
	}
}

func TestMutation_WithMetadataSuccess(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query(`create(title="Fix login bug", status=todo)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := result.(MutationResult)
	if !mr.Ok {
		t.Fatalf("expected ok=true, errors: %v", mr.Errors)
	}

	m := mr.Result.(map[string]any)
	if m["title"] != "Fix login bug" {
		t.Errorf("title = %v, want 'Fix login bug'", m["title"])
	}
	if m["status"] != "todo" {
		t.Errorf("status = %v, want todo", m["status"])
	}
}

// --- Framework-level validation ---

func TestMutation_RequiredParamMissing(t *testing.T) {
	s := newMutationSchema()

	// "create" requires "title" — call without it.
	result, err := s.Query(`create(status=todo)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := result.(MutationResult)
	if mr.Ok {
		t.Fatal("expected ok=false for missing required param")
	}
	if len(mr.Errors) == 0 {
		t.Fatal("expected at least one error")
	}

	found := false
	for _, e := range mr.Errors {
		if e.Code == ErrRequired && e.Field == "title" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected REQUIRED error for 'title', got: %v", mr.Errors)
	}
}

func TestMutation_EnumViolation(t *testing.T) {
	s := newMutationSchema()

	// "create" has status enum: [todo, in-progress, done]
	result, err := s.Query(`create(title="Test", status=invalid)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := result.(MutationResult)
	if mr.Ok {
		t.Fatal("expected ok=false for enum violation")
	}
	if len(mr.Errors) == 0 {
		t.Fatal("expected at least one error")
	}

	found := false
	for _, e := range mr.Errors {
		if e.Code == ErrInvalidValue && e.Field == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected INVALID_VALUE error for 'status', got: %v", mr.Errors)
	}
}

func TestMutation_EnumCaseInsensitive(t *testing.T) {
	s := newMutationSchema()

	// Enum values are compared case-insensitively.
	result, err := s.Query(`create(title="Test", status=TODO)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := result.(MutationResult)
	if !mr.Ok {
		t.Fatalf("expected ok=true (case-insensitive enum match), errors: %v", mr.Errors)
	}
}

// --- DryRun ---

func TestMutation_DryRunFlag(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query(`create(title="Dry test", dry_run=true)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := result.(MutationResult)
	if !mr.Ok {
		t.Fatalf("expected ok=true, errors: %v", mr.Errors)
	}

	m := mr.Result.(map[string]any)
	if m["dry_run"] != true {
		t.Errorf("expected dry_run=true in result, got: %v", m)
	}
}

func TestMutation_DryRunRemovedFromArgMap(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var receivedArgMap map[string]string
	s.Mutation("check-args", func(ctx MutationContext[*testItem]) (any, error) {
		receivedArgMap = ctx.ArgMap
		return "ok", nil
	})

	_, err := s.Query(`check-args(foo=bar, dry_run=true)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := receivedArgMap["dry_run"]; ok {
		t.Error("dry_run should be removed from ArgMap before reaching handler")
	}
	if receivedArgMap["foo"] != "bar" {
		t.Errorf("foo = %v, want bar", receivedArgMap["foo"])
	}
}

func TestMutation_DryRunVariants(t *testing.T) {
	for _, val := range []string{"true", "1", "yes"} {
		t.Run("dry_run="+val, func(t *testing.T) {
			s := newTestSchema()
			s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

			var gotDryRun bool
			s.Mutation("check", func(ctx MutationContext[*testItem]) (any, error) {
				gotDryRun = ctx.DryRun
				return "ok", nil
			})

			_, err := s.Query(`check(dry_run=` + val + `)`)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !gotDryRun {
				t.Errorf("DryRun should be true for dry_run=%s", val)
			}
		})
	}
}

// --- Handler error ---

func TestMutation_HandlerError(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query(`fail-mutation()`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := result.(MutationResult)
	if mr.Ok {
		t.Fatal("expected ok=false for handler error")
	}
	if len(mr.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	if !strings.Contains(mr.Errors[0].Message, "domain error") {
		t.Errorf("error message = %q, want 'domain error'", mr.Errors[0].Message)
	}
}

// --- Batch execution ---

func TestMutation_BatchMultiple(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query(`update(T1, status=done); update(T2, status=closed)`)
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

	for i, r := range results {
		mr, ok := r.(MutationResult)
		if !ok {
			t.Fatalf("results[%d] type = %T, want MutationResult", i, r)
		}
		if !mr.Ok {
			t.Errorf("results[%d] ok=false, errors: %v", i, mr.Errors)
		}
	}
}

func TestMutation_MixedBatchQueryAndMutation(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query(`update(T1, status=done); get(T1) { id status }; count()`)
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

	// First: mutation result
	mr, ok := results[0].(MutationResult)
	if !ok {
		t.Fatalf("results[0] type = %T, want MutationResult", results[0])
	}
	if !mr.Ok {
		t.Errorf("results[0] (mutation) ok=false: %v", mr.Errors)
	}

	// Second: query result (map)
	m, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] type = %T, want map[string]any", results[1])
	}
	if m["id"] != "T1" {
		t.Errorf("results[1].id = %v, want T1", m["id"])
	}

	// Third: query result (count)
	m3, ok := results[2].(map[string]any)
	if !ok {
		t.Fatalf("results[2] type = %T, want map[string]any", results[2])
	}
	if m3["count"] != 3 {
		t.Errorf("results[2].count = %v, want 3", m3["count"])
	}
}

// --- Mutation without metadata ---

func TestMutation_WithoutMetadata_NoValidation(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	called := false
	s.Mutation("simple", func(ctx MutationContext[*testItem]) (any, error) {
		called = true
		return map[string]any{"done": true}, nil
	})

	result, err := s.Query(`simple()`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler not called")
	}

	mr := result.(MutationResult)
	if !mr.Ok {
		t.Fatalf("expected ok=true, got errors: %v", mr.Errors)
	}
}

// --- HasMutations ---

func TestHasMutations_True(t *testing.T) {
	s := newMutationSchema()
	if !s.HasMutations() {
		t.Error("HasMutations() should return true when mutations are registered")
	}
}

func TestHasMutations_False(t *testing.T) {
	s := NewSchema[*testItem]()
	if s.HasMutations() {
		t.Error("HasMutations() should return false for a fresh schema")
	}
}

// --- MutationContext fields ---

func TestMutation_ContextFields(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var gotCtx MutationContext[*testItem]
	s.Mutation("inspect", func(ctx MutationContext[*testItem]) (any, error) {
		gotCtx = ctx
		return "ok", nil
	})

	_, err := s.Query(`inspect(T1, status=done, priority=high)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotCtx.Mutation != "inspect" {
		t.Errorf("Mutation = %q, want inspect", gotCtx.Mutation)
	}
	if gotCtx.Statement.Operation != "inspect" {
		t.Errorf("Statement.Operation = %q, want inspect", gotCtx.Statement.Operation)
	}
	if len(gotCtx.Args) != 3 {
		t.Fatalf("Args length = %d, want 3", len(gotCtx.Args))
	}
	if gotCtx.ArgMap["status"] != "done" {
		t.Errorf("ArgMap[status] = %q, want done", gotCtx.ArgMap["status"])
	}
	if gotCtx.ArgMap["priority"] != "high" {
		t.Errorf("ArgMap[priority] = %q, want high", gotCtx.ArgMap["priority"])
	}
	if gotCtx.DryRun {
		t.Error("DryRun should be false when not specified")
	}

	// Items loader should be available.
	items, loadErr := gotCtx.Items()
	if loadErr != nil {
		t.Fatalf("Items() error: %v", loadErr)
	}
	if len(items) != 3 {
		t.Errorf("Items() returned %d items, want 3", len(items))
	}
}

// --- Schema introspection with mutations ---

func TestIntrospect_MutationsInSchema(t *testing.T) {
	s := newMutationSchema()

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)

	// operations should NOT include mutation names
	ops := m["operations"].([]string)
	for _, op := range ops {
		if op == "update" || op == "create" || op == "fail-mutation" {
			t.Errorf("operations should not include mutation %q", op)
		}
	}
	// But should still include "get", "count", "schema"
	opSet := make(map[string]bool)
	for _, op := range ops {
		opSet[op] = true
	}
	for _, expected := range []string{"get", "count", "schema"} {
		if !opSet[expected] {
			t.Errorf("operations missing %q", expected)
		}
	}

	// mutations key should be present and sorted
	mutsRaw, exists := m["mutations"]
	if !exists {
		t.Fatal("mutations key missing from schema()")
	}
	muts := mutsRaw.([]string)
	if !sort.StringsAreSorted(muts) {
		t.Errorf("mutations should be sorted, got: %v", muts)
	}
	mutSet := make(map[string]bool)
	for _, mut := range muts {
		mutSet[mut] = true
	}
	for _, expected := range []string{"create", "update", "fail-mutation"} {
		if !mutSet[expected] {
			t.Errorf("mutations missing %q", expected)
		}
	}

	// mutationMetadata should only include "create" (registered with metadata)
	metaRaw, exists := m["mutationMetadata"]
	if !exists {
		t.Fatal("mutationMetadata key missing from schema()")
	}
	meta := metaRaw.(map[string]MutationMetadata)
	if _, exists := meta["create"]; !exists {
		t.Error("mutationMetadata missing 'create'")
	}
	if _, exists := meta["update"]; exists {
		t.Error("mutationMetadata should not have 'update' (registered without metadata)")
	}
}

func TestIntrospect_NoMutations_BackwardsCompat(t *testing.T) {
	s := newTestSchema()
	s.Operation("list", func(ctx OperationContext[*testItem]) (any, error) { return nil, nil })

	result, err := s.Query("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, exists := m["mutations"]; exists {
		t.Error("mutations key should be absent when no mutations registered")
	}
	if _, exists := m["mutationMetadata"]; exists {
		t.Error("mutationMetadata key should be absent when no mutations registered")
	}
}

func TestIntrospect_MutationMetadata_JSONSerialization(t *testing.T) {
	s := newMutationSchema()

	data, err := s.QueryJSON("schema()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("invalid JSON: %s", string(data))
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	metaRaw, exists := m["mutationMetadata"]
	if !exists {
		t.Fatal("mutationMetadata missing from JSON output")
	}
	metaMap := metaRaw.(map[string]any)
	createMeta := metaMap["create"].(map[string]any)

	if createMeta["description"] != "Create a new task" {
		t.Errorf("description = %v, want 'Create a new task'", createMeta["description"])
	}
	if createMeta["destructive"] != false {
		t.Errorf("destructive = %v, want false", createMeta["destructive"])
	}

	params := createMeta["parameters"].([]any)
	if len(params) != 2 {
		t.Fatalf("parameters length = %d, want 2", len(params))
	}

	// Check title param is required
	p0 := params[0].(map[string]any)
	if p0["name"] != "title" {
		t.Errorf("params[0].name = %v, want title", p0["name"])
	}
	if p0["required"] != true {
		t.Errorf("params[0].required = %v, want true", p0["required"])
	}

	// Check status param has enum
	p1 := params[1].(map[string]any)
	if p1["name"] != "status" {
		t.Errorf("params[1].name = %v, want status", p1["name"])
	}
	enumRaw := p1["enum"].([]any)
	if len(enumRaw) != 3 {
		t.Fatalf("enum length = %d, want 3", len(enumRaw))
	}
}

// --- Parse error still works ---

func TestMutation_ParseErrorStillReturned(t *testing.T) {
	s := newMutationSchema()

	_, err := s.Query("")
	if err == nil {
		t.Fatal("expected parse error for empty query")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
}

// --- MutationContext convenience methods ---

func TestMutationContext_PositionalArg(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var got string
	s.Mutation("pos", func(ctx MutationContext[*testItem]) (any, error) {
		got = ctx.PositionalArg()
		return "ok", nil
	})

	_, err := s.Query(`pos(T1, status=done)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "T1" {
		t.Errorf("PositionalArg() = %q, want T1", got)
	}
}

func TestMutationContext_PositionalArg_Empty(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var got string
	s.Mutation("pos", func(ctx MutationContext[*testItem]) (any, error) {
		got = ctx.PositionalArg()
		return "ok", nil
	})

	_, err := s.Query(`pos(status=done)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("PositionalArg() = %q, want empty", got)
	}
}

func TestMutationContext_RequireArg_Named(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var gotStatus string
	var gotErr error
	s.Mutation("req", func(ctx MutationContext[*testItem]) (any, error) {
		gotStatus, gotErr = ctx.RequireArg("status")
		return "ok", nil
	})

	_, _ = s.Query(`req(status=done)`)
	if gotErr != nil {
		t.Fatalf("RequireArg(status) error: %v", gotErr)
	}
	if gotStatus != "done" {
		t.Errorf("RequireArg(status) = %q, want done", gotStatus)
	}
}

func TestMutationContext_RequireArg_Missing(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var gotErr error
	s.Mutation("req", func(ctx MutationContext[*testItem]) (any, error) {
		_, gotErr = ctx.RequireArg("status")
		return "ok", nil
	})

	_, _ = s.Query(`req()`)
	if gotErr == nil {
		t.Fatal("RequireArg(status) should return error when missing")
	}
	if !strings.Contains(gotErr.Error(), "status") {
		t.Errorf("error should mention 'status', got: %v", gotErr)
	}
}

func TestMutationContext_RequireArg_PositionalNotChecked(t *testing.T) {
	// RequireArg only checks ArgMap, not positional args.
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var gotErr error
	s.Mutation("req", func(ctx MutationContext[*testItem]) (any, error) {
		_, gotErr = ctx.RequireArg("id")
		return "ok", nil
	})

	// T1 is positional, not key=value. RequireArg should NOT find it.
	_, _ = s.Query(`req(T1)`)
	if gotErr == nil {
		t.Fatal("RequireArg(id) should return error for positional-only arg")
	}
}

func TestMutationContext_ArgDefault(t *testing.T) {
	s := newTestSchema()
	s.SetLoader(func() ([]*testItem, error) { return testData(), nil })

	var gotStatus, gotPriority string
	s.Mutation("def", func(ctx MutationContext[*testItem]) (any, error) {
		gotStatus = ctx.ArgDefault("status", "todo")
		gotPriority = ctx.ArgDefault("priority", "medium")
		return "ok", nil
	})

	_, _ = s.Query(`def(status=done)`)
	if gotStatus != "done" {
		t.Errorf("ArgDefault(status) = %q, want done", gotStatus)
	}
	if gotPriority != "medium" {
		t.Errorf("ArgDefault(priority) = %q, want medium (default)", gotPriority)
	}
}

// --- validateMutationArgs unit tests ---

func TestValidateMutationArgs_RequiredPresent(t *testing.T) {
	argMap := map[string]string{"title": "hello"}
	args := []Arg{{Key: "title", Value: "hello"}}
	params := []ParameterDef{{Name: "title", Required: true}}

	errs := validateMutationArgs(argMap, args, params)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateMutationArgs_RequiredMissing(t *testing.T) {
	argMap := map[string]string{}
	args := []Arg{}
	params := []ParameterDef{{Name: "title", Required: true}}

	errs := validateMutationArgs(argMap, args, params)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Code != ErrRequired {
		t.Errorf("code = %q, want REQUIRED", errs[0].Code)
	}
	if errs[0].Field != "title" {
		t.Errorf("field = %q, want title", errs[0].Field)
	}
}

func TestValidateMutationArgs_RequiredSatisfiedByPositional(t *testing.T) {
	// A positional arg should satisfy a required param.
	argMap := map[string]string{}
	args := []Arg{{Key: "", Value: "task-1"}}
	params := []ParameterDef{{Name: "id", Required: true}}

	errs := validateMutationArgs(argMap, args, params)
	if len(errs) != 0 {
		t.Errorf("expected no errors (positional satisfies required), got: %v", errs)
	}
}

func TestValidateMutationArgs_EnumValid(t *testing.T) {
	argMap := map[string]string{"status": "done"}
	args := []Arg{{Key: "status", Value: "done"}}
	params := []ParameterDef{{Name: "status", Enum: []string{"todo", "in-progress", "done"}}}

	errs := validateMutationArgs(argMap, args, params)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateMutationArgs_EnumInvalid(t *testing.T) {
	argMap := map[string]string{"status": "bogus"}
	args := []Arg{{Key: "status", Value: "bogus"}}
	params := []ParameterDef{{Name: "status", Enum: []string{"todo", "in-progress", "done"}}}

	errs := validateMutationArgs(argMap, args, params)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Code != ErrInvalidValue {
		t.Errorf("code = %q, want INVALID_VALUE", errs[0].Code)
	}
}

func TestValidateMutationArgs_MultipleErrors(t *testing.T) {
	argMap := map[string]string{"status": "bogus"}
	args := []Arg{{Key: "status", Value: "bogus"}}
	params := []ParameterDef{
		{Name: "title", Required: true},
		{Name: "status", Enum: []string{"todo", "done"}},
	}

	errs := validateMutationArgs(argMap, args, params)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}

	codes := map[string]bool{}
	for _, e := range errs {
		codes[e.Code] = true
	}
	if !codes[ErrRequired] {
		t.Error("expected REQUIRED error")
	}
	if !codes[ErrInvalidValue] {
		t.Error("expected INVALID_VALUE error")
	}
}
