package cobraext

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivalx1s/skill-agent-facing-api/agentquery"
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

func TestQueryCommand(t *testing.T) {
	s := newTestSchema(t)

	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"get(T1) { id name }"})

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

func TestQueryCommand_List(t *testing.T) {
	s := newTestSchema(t)

	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"list() { id }"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %s", buf.String())
	}

	var items []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestSearchCommand(t *testing.T) {
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
	cmd.SetArgs([]string{"Hello"})

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
	cmd.SetArgs([]string{"hello", "-i", "-C", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []agentquery.SearchResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Match on line 2 with 1 context line should give 3 lines
	if len(results) != 3 {
		t.Fatalf("expected 3 results (with context), got %d", len(results))
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

// --- --format flag tests ---

func TestQueryCommand_FormatFlagAccepted(t *testing.T) {
	s := newTestSchema(t)
	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"get(T1) { id name }", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With --format=json, output should be valid JSON
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("expected valid JSON output, got: %s", buf.String())
	}
}

func TestQueryCommand_DefaultNoFlagUsesSchemaMode(t *testing.T) {
	s := newTestSchema(t)
	cmd := QueryCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"get(T1) { id name }"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default schema mode is HumanReadable, so output should be valid JSON
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("expected valid JSON output with default mode, got: %s", buf.String())
	}
}

func TestSearchCommand_FormatCompactProducesNonJSON(t *testing.T) {
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

	// Compact output should NOT be a JSON array
	if strings.HasPrefix(out, "[") {
		t.Error("compact format should not produce JSON array")
	}

	// Should contain file header and match line
	if !strings.Contains(out, "test.md") {
		t.Errorf("expected file header 'test.md' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "1: Hello World") {
		t.Errorf("expected match line '1: Hello World' in output, got:\n%s", out)
	}
}

func TestSearchCommand_FormatLLMAlias(t *testing.T) {
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
	// "llm" is an alias for "compact" — should produce the same non-JSON output
	if strings.HasPrefix(out, "[") {
		t.Error("--format=llm should produce compact (non-JSON) output")
	}
	if !strings.Contains(out, "test.md") {
		t.Errorf("expected file header, got:\n%s", out)
	}
}

func TestSearchCommand_DefaultNoFlagUsesSchemaMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Schema set to LLMReadable — command without --format should use schema default
	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
		agentquery.WithOutputMode(agentquery.LLMReadable),
	)

	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Hello"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	// Should be compact because schema defaults to LLMReadable
	if strings.HasPrefix(out, "[") {
		t.Error("expected compact output from LLMReadable schema default")
	}
	if !strings.Contains(out, "test.md") {
		t.Errorf("expected file header, got:\n%s", out)
	}
}

func TestSearchCommand_FormatJsonOverridesLLMSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte("Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Schema defaults to LLMReadable, but --format=json should override
	s := agentquery.NewSchema[*testItem](
		agentquery.WithDataDir(dir),
		agentquery.WithExtensions(".md"),
		agentquery.WithOutputMode(agentquery.LLMReadable),
	)

	cmd := SearchCommand(s)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Hello", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("--format=json should produce valid JSON, got: %s", buf.String())
	}
}

func TestParseOutputMode(t *testing.T) {
	tests := []struct {
		input string
		want  agentquery.OutputMode
	}{
		{"compact", agentquery.LLMReadable},
		{"COMPACT", agentquery.LLMReadable},
		{"Compact", agentquery.LLMReadable},
		{"llm", agentquery.LLMReadable},
		{"LLM", agentquery.LLMReadable},
		{"json", agentquery.HumanReadable},
		{"JSON", agentquery.HumanReadable},
		{"", agentquery.HumanReadable},
		{"anything", agentquery.HumanReadable},
	}

	for _, tt := range tests {
		got := parseOutputMode(tt.input)
		if got != tt.want {
			t.Errorf("parseOutputMode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
