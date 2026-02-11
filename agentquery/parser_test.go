package agentquery

import (
	"errors"
	"strings"
	"testing"
)

// mapResolver is a test helper that implements FieldResolver.
type mapResolver struct {
	fields  map[string]bool
	presets map[string][]string
}

func (r *mapResolver) ResolveField(name string) ([]string, error) {
	if expanded, ok := r.presets[name]; ok {
		return expanded, nil
	}
	if r.fields[name] {
		return []string{name}, nil
	}
	return nil, errors.New("unknown field: " + name)
}

func configWithOps(ops ...string) *ParserConfig {
	m := make(map[string]bool)
	for _, op := range ops {
		m[op] = true
	}
	return &ParserConfig{Operations: m}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		config  *ParserConfig
		want    *Query
		wantErr string
	}{
		// --- Basic valid queries ---
		{
			name:  "simple get with positional arg",
			input: "get(T1)",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "list with key-value args",
			input: "list(type=task, status=dev)",
			want: &Query{Statements: []Statement{{
				Operation: "list",
				Args: []Arg{
					{Key: "type", Value: "task", Pos: Pos{Offset: 5, Line: 1, Column: 6}},
					{Key: "status", Value: "dev", Pos: Pos{Offset: 16, Line: 1, Column: 17}},
				},
				Pos: Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "query with field projection",
			input: "get(T1) { status name }",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Fields:    []string{"status", "name"},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "batch queries with semicolons",
			input: "get(T1); list(status=dev)",
			want: &Query{Statements: []Statement{
				{
					Operation: "get",
					Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
					Pos:       Pos{Offset: 0, Line: 1, Column: 1},
				},
				{
					Operation: "list",
					Args:      []Arg{{Key: "status", Value: "dev", Pos: Pos{Offset: 14, Line: 1, Column: 15}}},
					Pos:       Pos{Offset: 9, Line: 1, Column: 10},
				},
			}},
		},
		{
			name:  "empty args",
			input: "summary()",
			want: &Query{Statements: []Statement{{
				Operation: "summary",
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "empty fields",
			input: "get(T1) {}",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "string literal as arg value",
			input: `get("hello world")`,
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "hello world", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "string literal as key-value arg value",
			input: `list(filter="status in (dev, review)")`,
			want: &Query{Statements: []Statement{{
				Operation: "list",
				Args:      []Arg{{Key: "filter", Value: "status in (dev, review)", Pos: Pos{Offset: 5, Line: 1, Column: 6}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "positional args",
			input: "get(T1, T2)",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args: []Arg{
					{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}},
					{Value: "T2", Pos: Pos{Offset: 8, Line: 1, Column: 9}},
				},
				Pos: Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "mixed positional and key-value args",
			input: "list(task, status=dev)",
			want: &Query{Statements: []Statement{{
				Operation: "list",
				Args: []Arg{
					{Value: "task", Pos: Pos{Offset: 5, Line: 1, Column: 6}},
					{Key: "status", Value: "dev", Pos: Pos{Offset: 11, Line: 1, Column: 12}},
				},
				Pos: Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "identifier with hyphens and digits",
			input: "get(TASK-260211-abc)",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "TASK-260211-abc", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "trailing semicolons ignored",
			input: "get(T1);",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "multiple semicolons between queries",
			input: "get(T1);;;get(T2)",
			want: &Query{Statements: []Statement{
				{
					Operation: "get",
					Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
					Pos:       Pos{Offset: 0, Line: 1, Column: 1},
				},
				{
					Operation: "get",
					Args:      []Arg{{Value: "T2", Pos: Pos{Offset: 14, Line: 1, Column: 15}}},
					Pos:       Pos{Offset: 10, Line: 1, Column: 11},
				},
			}},
		},
		{
			name:  "string with escape sequences",
			input: `get("hello \"world\"")`,
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: `hello "world"`, Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "string with backslash-n",
			input: `get("line1\nline2")`,
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "line1\nline2", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "multiline query",
			input: "get(T1)\n{ status name }",
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Fields:    []string{"status", "name"},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "batch on multiple lines",
			input: "get(T1);\nlist(status=dev)",
			want: &Query{Statements: []Statement{
				{
					Operation: "get",
					Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
					Pos:       Pos{Offset: 0, Line: 1, Column: 1},
				},
				{
					Operation: "list",
					Args:      []Arg{{Key: "status", Value: "dev", Pos: Pos{Offset: 14, Line: 2, Column: 6}}},
					Pos:       Pos{Offset: 9, Line: 2, Column: 1},
				},
			}},
		},

		// --- Config: operation validation ---
		{
			name:    "unknown op rejected with config",
			input:   "foo()",
			config:  configWithOps("get", "list"),
			wantErr: "unknown operation",
		},
		{
			name:   "known op accepted with config",
			input:  "get(T1)",
			config: configWithOps("get", "list"),
			want: &Query{Statements: []Statement{{
				Operation: "get",
				Args:      []Arg{{Value: "T1", Pos: Pos{Offset: 4, Line: 1, Column: 5}}},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "unknown op accepted without config",
			input: "foo()",
			want: &Query{Statements: []Statement{{
				Operation: "foo",
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "unknown op accepted with nil operations map",
			input: "anything()",
			config: &ParserConfig{
				Operations: nil,
			},
			want: &Query{Statements: []Statement{{
				Operation: "anything",
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},

		// --- Config: field resolver ---
		{
			name:  "field resolver expands preset",
			input: "list() { overview }",
			config: &ParserConfig{
				FieldResolver: &mapResolver{
					fields:  map[string]bool{"id": true, "name": true, "status": true},
					presets: map[string][]string{"overview": {"id", "name", "status"}},
				},
			},
			want: &Query{Statements: []Statement{{
				Operation: "list",
				Fields:    []string{"id", "name", "status"},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "field resolver accepts known field",
			input: "list() { status }",
			config: &ParserConfig{
				FieldResolver: &mapResolver{
					fields: map[string]bool{"status": true},
				},
			},
			want: &Query{Statements: []Statement{{
				Operation: "list",
				Fields:    []string{"status"},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},
		{
			name:  "field resolver rejects unknown field",
			input: "list() { bogus }",
			config: &ParserConfig{
				FieldResolver: &mapResolver{
					fields: map[string]bool{"status": true},
				},
			},
			wantErr: "unknown field",
		},
		{
			name:  "field resolver deduplicates expanded fields",
			input: "list() { overview id }",
			config: &ParserConfig{
				FieldResolver: &mapResolver{
					fields:  map[string]bool{"id": true, "name": true, "status": true},
					presets: map[string][]string{"overview": {"id", "name", "status"}},
				},
			},
			want: &Query{Statements: []Statement{{
				Operation: "list",
				Fields:    []string{"id", "name", "status"},
				Pos:       Pos{Offset: 0, Line: 1, Column: 1},
			}}},
		},

		// --- Error cases ---
		{
			name:    "empty query",
			input:   "",
			wantErr: "empty query",
		},
		{
			name:    "only semicolons",
			input:   ";;;",
			wantErr: "empty query",
		},
		{
			name:    "only whitespace",
			input:   "   \t\n  ",
			wantErr: "empty query",
		},
		{
			name:    "unclosed paren",
			input:   "get(T1",
			wantErr: "expected ')'",
		},
		{
			name:    "unclosed brace",
			input:   "get(T1) { status",
			wantErr: "expected '}'",
		},
		{
			name:    "unexpected token instead of operation",
			input:   "(T1)",
			wantErr: "expected operation name",
		},
		{
			name:    "missing lparen after operation",
			input:   "get T1",
			wantErr: "expected '('",
		},
		{
			name:    "unexpected character",
			input:   "get(@T1)",
			wantErr: "unexpected character",
		},
		{
			name:    "unterminated string",
			input:   `get("hello)`,
			wantErr: "unterminated string",
		},
		{
			name:    "unexpected token after query",
			input:   "get(T1) list(T2)",
			wantErr: "expected ';' or end of input",
		},
		{
			name:    "expected value after equals",
			input:   "get(key=)",
			wantErr: "expected value after '='",
		},
		{
			name:    "non-ident in field projection",
			input:   "get(T1) { = }",
			wantErr: "expected field name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input, tt.config)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				// Verify error is a *ParseError
				var pe *ParseError
				if !errors.As(err, &pe) {
					t.Errorf("expected error to be *ParseError, got %T", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Compare statements count
			if len(got.Statements) != len(tt.want.Statements) {
				t.Fatalf("got %d statements, want %d", len(got.Statements), len(tt.want.Statements))
			}
			for i, wantStmt := range tt.want.Statements {
				gotStmt := got.Statements[i]
				if gotStmt.Operation != wantStmt.Operation {
					t.Errorf("stmt[%d].Operation = %q, want %q", i, gotStmt.Operation, wantStmt.Operation)
				}
				if gotStmt.Pos != wantStmt.Pos {
					t.Errorf("stmt[%d].Pos = %+v, want %+v", i, gotStmt.Pos, wantStmt.Pos)
				}
				// Compare args
				if len(gotStmt.Args) != len(wantStmt.Args) {
					t.Fatalf("stmt[%d]: got %d args, want %d", i, len(gotStmt.Args), len(wantStmt.Args))
				}
				for j, wantArg := range wantStmt.Args {
					gotArg := gotStmt.Args[j]
					if gotArg.Key != wantArg.Key {
						t.Errorf("stmt[%d].Args[%d].Key = %q, want %q", i, j, gotArg.Key, wantArg.Key)
					}
					if gotArg.Value != wantArg.Value {
						t.Errorf("stmt[%d].Args[%d].Value = %q, want %q", i, j, gotArg.Value, wantArg.Value)
					}
					if gotArg.Pos != wantArg.Pos {
						t.Errorf("stmt[%d].Args[%d].Pos = %+v, want %+v", i, j, gotArg.Pos, wantArg.Pos)
					}
				}
				// Compare fields
				if len(gotStmt.Fields) != len(wantStmt.Fields) {
					t.Fatalf("stmt[%d]: got %d fields, want %d\ngot: %v\nwant: %v", i, len(gotStmt.Fields), len(wantStmt.Fields), gotStmt.Fields, wantStmt.Fields)
				}
				for j := range wantStmt.Fields {
					if gotStmt.Fields[j] != wantStmt.Fields[j] {
						t.Errorf("stmt[%d].Fields[%d] = %q, want %q", i, j, gotStmt.Fields[j], wantStmt.Fields[j])
					}
				}
			}
		})
	}
}

func TestParseError_StructuredFields(t *testing.T) {
	// Verify ParseError has correct Got/Expected on specific error types
	_, err := Parse("get(T1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Expected == "" {
		t.Error("expected ParseError.Expected to be non-empty")
	}
	if pe.Pos.Line != 1 {
		t.Errorf("expected Line 1, got %d", pe.Pos.Line)
	}
}

func TestParseError_PositionMultiline(t *testing.T) {
	// Error on second line should have correct line/column
	_, err := Parse("get(T1);\n@invalid", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Pos.Line != 2 {
		t.Errorf("expected error on line 2, got line %d", pe.Pos.Line)
	}
	if pe.Pos.Column != 1 {
		t.Errorf("expected error at column 1, got column %d", pe.Pos.Column)
	}
}

func TestParse_NilConfig(t *testing.T) {
	// nil config should behave like permissive defaults
	q, err := Parse("anything(x, y=z) { field1 field2 }", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(q.Statements))
	}
	s := q.Statements[0]
	if s.Operation != "anything" {
		t.Errorf("Operation = %q, want %q", s.Operation, "anything")
	}
	if len(s.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(s.Args))
	}
	if s.Args[0].Key != "" || s.Args[0].Value != "x" {
		t.Errorf("Args[0] = {%q, %q}, want positional x", s.Args[0].Key, s.Args[0].Value)
	}
	if s.Args[1].Key != "y" || s.Args[1].Value != "z" {
		t.Errorf("Args[1] = {%q, %q}, want y=z", s.Args[1].Key, s.Args[1].Value)
	}
	if len(s.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(s.Fields))
	}
	if s.Fields[0] != "field1" || s.Fields[1] != "field2" {
		t.Errorf("Fields = %v, want [field1 field2]", s.Fields)
	}
}

func TestTokenizer_LineTracking(t *testing.T) {
	input := "get(T1);\nlist(T2)"
	q, err := Parse(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(q.Statements))
	}
	// Second statement should be on line 2
	if q.Statements[1].Pos.Line != 2 {
		t.Errorf("second statement line = %d, want 2", q.Statements[1].Pos.Line)
	}
	if q.Statements[1].Pos.Column != 1 {
		t.Errorf("second statement column = %d, want 1", q.Statements[1].Pos.Column)
	}
}

func FuzzParse(f *testing.F) {
	// Seed corpus from real queries
	f.Add("get(T1) { status }")
	f.Add("list(type=task, status=dev) { id name status }")
	f.Add("summary()")
	f.Add("get(T1) { status }; get(T2) { status }")
	f.Add("")
	f.Add(";;;")
	f.Add(`get("hello world")`)
	f.Add(`get("escape \"test\"")`)
	f.Add("get(TASK-260211-abc)")
	f.Add("list(a, b=c, d)")
	f.Add("get(T1) {}")
	f.Add("a()\nb()\nc()")
	f.Add("  get ( T1 )  { status  name }  ")
	f.Add(`get("line1\nline2")`)
	f.Add("get(T1);\n\nlist(T2)")

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic — this is the primary invariant
		q, err := Parse(input, nil)
		if err != nil {
			// Errors are fine, panics are not
			// Verify error implements error interface
			_ = err.Error()
			return
		}
		// If parsing succeeds, the AST must be non-nil with >0 statements
		if q == nil {
			t.Error("Parse returned nil query without error")
		}
		if q != nil && len(q.Statements) == 0 {
			t.Error("Parse returned empty query without error")
		}
		// Every statement must have a non-empty operation
		for i, s := range q.Statements {
			if s.Operation == "" {
				t.Errorf("statement[%d] has empty operation", i)
			}
		}
	})
}
