package cobraext

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
	"github.com/spf13/cobra"
)

type testItem struct {
	ID   string
	Name string
}

func newTestSchema(t *testing.T) *agentquery.Schema[*testItem] {
	t.Helper()
	s := agentquery.NewSchema[*testItem]()
	s.Field("id", func(item *testItem) any { return item.ID })
	s.Field("name", func(item *testItem) any { return item.Name })
	s.DefaultFields("id", "name")

	items := []*testItem{
		{ID: "T1", Name: "alpha"},
		{ID: "T2", Name: "beta"},
	}
	s.SetLoader(func() ([]*testItem, error) {
		return items, nil
	})

	s.Operation("list", func(ctx agentquery.OperationContext[*testItem]) (any, error) {
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

	s.Operation("get", func(ctx agentquery.OperationContext[*testItem]) (any, error) {
		if len(ctx.Statement.Args) == 0 {
			return nil, &agentquery.Error{Code: agentquery.ErrValidation, Message: "get requires an ID argument"}
		}
		data, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		for _, item := range data {
			if item.ID == ctx.Statement.Args[0].Value {
				return ctx.Selector.Apply(item), nil
			}
		}
		return nil, &agentquery.Error{Code: agentquery.ErrNotFound, Message: "not found"}
	})

	return s
}

func TestQueryCommand_JSON(t *testing.T) {
	s := newTestSchema(t)
	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"get(T1) { id name }", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %s", buf.String())
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if m["id"] != "T1" {
		t.Errorf("id = %v, want T1", m["id"])
	}
	if m["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", m["name"])
	}
}

func TestQueryCommand_Compact(t *testing.T) {
	s := newTestSchema(t)
	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"list() { id }", "--format", "compact"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	// Compact list output: header + rows
	if json.Valid(buf.Bytes()) && strings.HasPrefix(out, "[") {
		t.Error("compact output should not be a JSON array")
	}
	if !strings.Contains(out, "id") {
		t.Error("expected header with 'id'")
	}
	if !strings.Contains(out, "T1") {
		t.Error("expected row with T1")
	}
}

func TestQueryCommand_MissingFormatFlag(t *testing.T) {
	s := newTestSchema(t)
	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"get(T1)"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --format is missing, got nil")
	}
}

func TestSearchCommand_JSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Hello World\nFoo bar"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
	)
	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Hello", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %s", buf.String())
	}

	var results []agentquery.SearchResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "Hello World" {
		t.Errorf("content = %q, want %q", results[0].Content, "Hello World")
	}
}

func TestSearchCommand_WithFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Line one\nHello World\nLine three"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
	)
	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"hello", "-i", "-C", "1", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []agentquery.SearchResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results (with context), got %d", len(results))
	}
}

func TestSearchCommand_Compact(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Hello World\nFoo bar"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
	)
	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Hello", "--format", "compact"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "[") {
		t.Error("compact format should not produce JSON array")
	}
	if !strings.Contains(out, "test.md") {
		t.Errorf("expected file header 'test.md' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "1: Hello World") {
		t.Errorf("expected match line '1: Hello World' in output, got:\n%s", out)
	}
}

func TestSearchCommand_LLMAlias(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
	)
	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Hello", "--format", "llm"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "[") {
		t.Error("--format=llm should produce compact (non-JSON) output")
	}
	if !strings.Contains(out, "test.md") {
		t.Errorf("expected file header, got:\n%s", out)
	}
}

func TestSearchCommand_MissingFormatFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
	)
	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"Hello"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --format is missing, got nil")
	}
}

func TestAddCommands(t *testing.T) {
	s := newTestSchema(t)

	root := &cobra.Command{Use: "root"}
	AddCommands(root, s)

	names := make(map[string]bool)
	for _, c := range root.Commands() {
		names[c.Use] = true
	}

	if !names["q <query>"] {
		t.Error("expected 'q' command to be added")
	}
	if !names["grep <pattern>"] {
		t.Error("expected 'grep' command to be added")
	}
}

// newMutationTestSchema builds a schema with both read operations and mutations
// for testing the MutateCommand.
func newMutationTestSchema(t *testing.T) *agentquery.Schema[*testItem] {
	t.Helper()
	s := newTestSchema(t)

	// Non-destructive mutation.
	s.MutationWithMetadata("create", func(ctx agentquery.MutationContext[*testItem]) (any, error) {
		title := ctx.ArgMap["title"]
		result := map[string]any{"id": "new-1", "title": title}
		if ctx.DryRun {
			result["dry_run"] = true
		}
		return result, nil
	}, agentquery.MutationMetadata{
		Description: "Create a new item",
		Parameters: []agentquery.ParameterDef{
			{Name: "title", Type: "string", Required: true},
		},
		Destructive: false,
	})

	// Destructive mutation.
	s.MutationWithMetadata("delete", func(ctx agentquery.MutationContext[*testItem]) (any, error) {
		id := ""
		for _, a := range ctx.Args {
			if a.Key == "" {
				id = a.Value
				break
			}
		}
		result := map[string]any{"deleted": id}
		if ctx.DryRun {
			result["dry_run"] = true
		}
		return result, nil
	}, agentquery.MutationMetadata{
		Description: "Delete an item",
		Parameters: []agentquery.ParameterDef{
			{Name: "id", Type: "string", Required: true},
		},
		Destructive: true,
	})

	return s
}

// --- MutateCommand tests ---

func TestMutateCommand_JSON(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{`create(title="hello")`, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %s", buf.String())
	}

	var mr agentquery.MutationResult
	if err := json.Unmarshal(buf.Bytes(), &mr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !mr.Ok {
		t.Fatalf("expected ok=true, got errors: %v", mr.Errors)
	}
}

func TestMutateCommand_Compact(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{`create(title="hello")`, "--format", "compact"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	// Compact output should contain ok and result fields.
	if !strings.Contains(out, "ok") {
		t.Errorf("compact output missing 'ok', got:\n%s", out)
	}
}

func TestMutateCommand_DryRun(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{`create(title="test")`, "--format", "json", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var mr agentquery.MutationResult
	if err := json.Unmarshal(buf.Bytes(), &mr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !mr.Ok {
		t.Fatalf("expected ok=true, got errors: %v", mr.Errors)
	}

	// The handler should have received dry_run=true.
	result, ok := mr.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", mr.Result)
	}
	if result["dry_run"] != true {
		t.Errorf("expected dry_run=true in result, got: %v", result)
	}
}

func TestMutateCommand_DestructiveWithoutConfirm(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{`delete(T1)`, "--format", "json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for destructive mutation without --confirm")
	}
	if !strings.Contains(err.Error(), "destructive mutation requires --confirm") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMutateCommand_DestructiveWithConfirm(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{`delete(T1)`, "--format", "json", "--confirm"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var mr agentquery.MutationResult
	if err := json.Unmarshal(buf.Bytes(), &mr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !mr.Ok {
		t.Fatalf("expected ok=true, got errors: %v", mr.Errors)
	}
}

func TestMutateCommand_DestructiveWithDryRun(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	// --dry-run skips the --confirm requirement.
	cmd.SetArgs([]string{`delete(T1)`, "--format", "json", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var mr agentquery.MutationResult
	if err := json.Unmarshal(buf.Bytes(), &mr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !mr.Ok {
		t.Fatalf("expected ok=true, errors: %v", mr.Errors)
	}

	result := mr.Result.(map[string]any)
	if result["dry_run"] != true {
		t.Errorf("expected dry_run=true in result, got: %v", result)
	}
}

func TestMutateCommand_NonDestructiveNoConfirmNeeded(t *testing.T) {
	s := newMutationTestSchema(t)
	cmd := MutateCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	// "create" is not destructive — no --confirm needed.
	cmd.SetArgs([]string{`create(title="test")`, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var mr agentquery.MutationResult
	if err := json.Unmarshal(buf.Bytes(), &mr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !mr.Ok {
		t.Fatalf("expected ok=true, errors: %v", mr.Errors)
	}
}

func TestAddCommands_WithMutations(t *testing.T) {
	s := newMutationTestSchema(t)

	root := &cobra.Command{Use: "root"}
	AddCommands(root, s)

	names := make(map[string]bool)
	for _, c := range root.Commands() {
		names[c.Use] = true
	}

	if !names["m <mutation>"] {
		t.Error("expected 'm' command to be added when mutations exist")
	}
	if !names["q <query>"] {
		t.Error("expected 'q' command to be added")
	}
	if !names["grep <pattern>"] {
		t.Error("expected 'grep' command to be added")
	}
}

func TestAddCommands_WithoutMutations(t *testing.T) {
	s := newTestSchema(t) // no mutations

	root := &cobra.Command{Use: "root"}
	AddCommands(root, s)

	for _, c := range root.Commands() {
		if c.Use == "m <mutation>" {
			t.Error("'m' command should NOT be added when no mutations exist")
		}
	}
}

// --- injectDryRun unit tests ---

func TestInjectDryRun_EmptyArgs(t *testing.T) {
	got := injectDryRun("create()")
	want := "create(dry_run=true)"
	if got != want {
		t.Errorf("injectDryRun(\"create()\") = %q, want %q", got, want)
	}
}

func TestInjectDryRun_WithArgs(t *testing.T) {
	got := injectDryRun(`create(title="X")`)
	want := `create(title="X", dry_run=true)`
	if got != want {
		t.Errorf("injectDryRun = %q, want %q", got, want)
	}
}

func TestInjectDryRun_PositionalArg(t *testing.T) {
	got := injectDryRun("delete(task-1)")
	want := "delete(task-1, dry_run=true)"
	if got != want {
		t.Errorf("injectDryRun = %q, want %q", got, want)
	}
}

func TestInjectDryRun_Batch(t *testing.T) {
	got := injectDryRun(`create(title="A"); delete(task-1)`)
	want := `create(title="A", dry_run=true); delete(task-1, dry_run=true)`
	if got != want {
		t.Errorf("injectDryRun batch = %q, want %q", got, want)
	}
}

// --- needsConfirm unit tests ---

func TestNeedsConfirm_Destructive(t *testing.T) {
	s := newMutationTestSchema(t)
	if !needsConfirm(s, "delete(T1)") {
		t.Error("needsConfirm should return true for destructive mutation")
	}
}

func TestNeedsConfirm_NonDestructive(t *testing.T) {
	s := newMutationTestSchema(t)
	if needsConfirm(s, `create(title="X")`) {
		t.Error("needsConfirm should return false for non-destructive mutation")
	}
}

func TestNeedsConfirm_BatchWithDestructive(t *testing.T) {
	s := newMutationTestSchema(t)
	if !needsConfirm(s, `create(title="X"); delete(T1)`) {
		t.Error("needsConfirm should return true if ANY mutation in batch is destructive")
	}
}

func TestParseOutputMode(t *testing.T) {
	tests := []struct {
		input   string
		want    agentquery.OutputMode
		wantErr bool
	}{
		{"compact", agentquery.LLMReadable, false},
		{"COMPACT", agentquery.LLMReadable, false},
		{"Compact", agentquery.LLMReadable, false},
		{"llm", agentquery.LLMReadable, false},
		{"LLM", agentquery.LLMReadable, false},
		{"json", agentquery.HumanReadable, false},
		{"JSON", agentquery.HumanReadable, false},
		{"", agentquery.HumanReadable, true},
		{"anything", agentquery.HumanReadable, true},
	}

	for _, tt := range tests {
		got, err := parseOutputMode(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseOutputMode(%q): expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseOutputMode(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseOutputMode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
