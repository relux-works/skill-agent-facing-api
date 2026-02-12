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
