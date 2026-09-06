package agentquery

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGrammarExamplesParse pins the grammar to the parser: every advertised
// statement form must parse, and every token the tokenizer produces must be
// named by the grammar. A form the parser drops, or a token added to the
// tokenizer without a grammar entry, fails here.
func TestGrammarExamplesParse(t *testing.T) {
	g := DSLGrammar()
	if len(g.Examples) == 0 {
		t.Fatal("grammar advertises no examples")
	}
	for _, example := range g.Examples {
		if _, err := Parse(example, nil); err != nil {
			t.Errorf("grammar example %q does not parse: %v", example, err)
		}
	}

	// Every token type the tokenizer knows is named exactly once.
	for typ := tokenIdent; typ <= tokenEOF; typ++ {
		name := tokenTypeName(typ)
		if name == "unknown" {
			t.Fatalf("token type %d has no name", typ)
		}
		count := 0
		for _, listed := range g.Tokens {
			if listed == name {
				count++
			}
		}
		if count != 1 {
			t.Errorf("token %s listed %d times in grammar, want 1", name, count)
		}
	}
	if len(g.Tokens) != int(tokenEOF)+1 {
		t.Errorf("grammar lists %d tokens, tokenizer has %d", len(g.Tokens), int(tokenEOF)+1)
	}

	// Every punctuation token the grammar names is one the tokenizer emits.
	for _, name := range g.Tokens {
		if !strings.HasPrefix(name, "'") {
			continue
		}
		literal := strings.Trim(name, "'")
		tokens, err := newTokenizer(literal).tokenize()
		if err != nil || len(tokens) != 2 || tokens[0].val != literal {
			t.Errorf("grammar token %s is not tokenized as a single token: tokens=%v err=%v", name, tokens, err)
		}
	}
}

// TestGrammarCoversParserAcceptedStatementForms derives the statement forms
// from parser fixtures rather than grammarExamples. That keeps the completeness
// check independent: removing a form from both the prose and advertised
// examples cannot also remove it from the test's source of truth.
func TestGrammarCoversParserAcceptedStatementForms(t *testing.T) {
	fixtures := []struct {
		name  string
		input string
	}{
		{name: "positional", input: `op(element-id)`},
		{name: "key=value", input: `op(key=value)`},
		{name: "quoted", input: `op(key="quoted value")`},
		{name: "batch", input: `op(); other()`},
		{name: "projection", input: `op() { field }`},
	}

	acceptedForms := make(map[string]bool, len(fixtures))
	for _, fixture := range fixtures {
		q, err := Parse(fixture.input, nil)
		if err != nil {
			t.Fatalf("parser contract fixture %s no longer parses: %v", fixture.name, err)
		}
		forms := statementForms(q)
		if !forms[fixture.name] {
			t.Fatalf("parser contract fixture %q produced forms %v, want %q", fixture.input, forms, fixture.name)
		}
		for form := range forms {
			acceptedForms[form] = true
		}
	}

	g := DSLGrammar()
	prose := map[string]string{
		"positional": g.Arguments,
		"key=value":  g.Arguments,
		"quoted":     g.Values,
		"batch":      g.Batch,
		"projection": g.Projection,
	}
	markers := map[string]string{
		"positional": "bare positional",
		"key=value":  "key=value",
		"quoted":     "double-quoted string",
		"batch":      "statements separated",
		"projection": "field projection",
	}

	exampleForms := make(map[string]bool, len(acceptedForms))
	for _, example := range g.Examples {
		q, err := Parse(example, nil)
		if err != nil {
			t.Errorf("grammar example %q does not parse: %v", example, err)
			continue
		}
		for form := range statementForms(q) {
			exampleForms[form] = true
		}
	}

	if len(acceptedForms) != len(fixtures) {
		t.Fatalf("parser fixtures derived %d accepted forms, want %d: %v", len(acceptedForms), len(fixtures), acceptedForms)
	}
	for form := range acceptedForms {
		t.Run(form, func(t *testing.T) {
			if !strings.Contains(prose[form], markers[form]) {
				t.Errorf("grammar prose for parser-accepted %s form = %q, want marker %q", form, prose[form], markers[form])
			}
			if !exampleForms[form] {
				t.Errorf("grammar has no parsing example of parser-accepted %s form", form)
			}
		})
	}
}

func statementForms(q *Query) map[string]bool {
	forms := make(map[string]bool)
	if len(q.Statements) > 1 {
		forms["batch"] = true
	}
	for _, statement := range q.Statements {
		if len(statement.Fields) > 0 {
			forms["projection"] = true
		}
		for _, arg := range statement.Args {
			if arg.Key == "" {
				forms["positional"] = true
			} else {
				forms["key=value"] = true
			}
			if strings.ContainsAny(arg.Value, " \t\n,()") {
				forms["quoted"] = true
			}
		}
	}
	return forms
}

// TestGrammarEscapesRoundTrip pins the escape list to the tokenizer and the
// renderer: each advertised escape decodes through the tokenizer, and a
// value containing the decoded byte renders back through the same escape.
func TestGrammarEscapesRoundTrip(t *testing.T) {
	g := DSLGrammar()
	if len(g.Escapes) != len(stringEscapes) {
		t.Fatalf("grammar lists %d escapes, tokenizer accepts %d", len(g.Escapes), len(stringEscapes))
	}
	for _, escape := range g.Escapes {
		if len(escape) != 2 || escape[0] != '\\' {
			t.Fatalf("escape %q is not a backslash sequence", escape)
		}
		decoded, ok := stringEscapes[escape[1]]
		if !ok {
			t.Errorf("grammar escape %q is not one the tokenizer decodes", escape)
			continue
		}
		q, err := Parse(`op(v="a`+escape+`b")`, nil)
		if err != nil {
			t.Errorf("escape %q does not parse: %v", escape, err)
			continue
		}
		got := q.Statements[0].Args[0].Value
		if got != "a"+string(decoded)+"b" {
			t.Errorf("escape %q decoded to %q", escape, got)
		}
		if rendered := Render(q); !strings.Contains(rendered, escape) {
			t.Errorf("Render(%q) = %q, want the %q escape", got, rendered, escape)
		}
	}
	// An unlisted escape is preserved verbatim, so listing it would be a lie.
	q, err := Parse(`op(v="a\qb")`, nil)
	if err != nil || q.Statements[0].Args[0].Value != `a\qb` {
		t.Errorf("unlisted escape should be kept verbatim, got %v %v", q, err)
	}
}

// TestSchemaIntrospectionCarriesGrammar is the negative test for the
// schema() contract: the grammar block must be present and serializable, and
// the syntax line must name the batch separator and the escapes.
func TestSchemaIntrospectionCarriesGrammar(t *testing.T) {
	s := NewSchema[string]()
	s.Field("id", func(v string) any { return v })
	raw, err := s.Query("schema()")
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(map[string]any)
	grammar, ok := result["grammar"].(Grammar)
	if !ok {
		t.Fatalf("schema() grammar = %T, want Grammar", result["grammar"])
	}
	if grammar.Statement == "" || grammar.Batch == "" || grammar.Arguments == "" || grammar.Values == "" || grammar.Projection == "" {
		t.Fatalf("grammar has empty sections: %+v", grammar)
	}
	if !strings.Contains(grammar.Arguments, "key=value") || !strings.Contains(grammar.Batch, "';'") {
		t.Fatalf("grammar does not state argument syntax and batch separator: %+v", grammar)
	}
	for _, want := range []string{"first bare positional", "element/identifier", "remaining arguments use key=value"} {
		if !strings.Contains(grammar.Arguments, want) {
			t.Errorf("Grammar.Arguments = %q, lacks %q", grammar.Arguments, want)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"grammar"`, `"statement"`, `"escapes"`, `"tokens"`, `"examples"`} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("schema() JSON lacks %s", key)
		}
	}

	syntax := GrammarSyntax()
	for _, want := range []string{"first bare positional", "element/identifier", "remaining arguments use key=value", "';'", `\"`, `\\`, `\n`, `\t`, "{ field preset }"} {
		if !strings.Contains(syntax, want) {
			t.Errorf("GrammarSyntax() = %q, lacks %q", syntax, want)
		}
	}
}

// TestUnknownOperationWinsOverArgumentPosition is the negative test for the
// recovery path: an unknown operation must be reported as unknown even when
// its arguments do not parse, and must carry the known operations and the
// schema() pointer both as data and in the rendered message.
func TestUnknownOperationWinsOverArgumentPosition(t *testing.T) {
	s := NewSchema[string]()
	s.Operation("list", func(ctx OperationContext[string]) (any, error) { return nil, nil })
	s.Operation("get", func(ctx OperationContext[string]) (any, error) { return nil, nil })

	for _, input := range []string{
		"items(status in [a,b]) { id }",
		"items(status in a) { id }",
		"items(status=x) { id }",
		"list(); items(status in [a,b])",
	} {
		_, err := s.Parse(input)
		pe, ok := err.(*ParseError)
		if !ok {
			t.Fatalf("%q: error = %T (%v), want *ParseError", input, err, err)
		}
		if !pe.IsUnknownOperation() || pe.Message != `unknown operation "items"` {
			t.Fatalf("%q: verdict = %#v, want unknown operation", input, pe)
		}
		wantColumn := strings.Index(input, "items") + 1
		if pe.Pos.Column != wantColumn || pe.Operation != "items" || pe.OperationPos == nil || pe.OperationPos.Column != wantColumn {
			t.Fatalf("%q: position = %+v, want the operation at column %d", input, pe, wantColumn)
		}
		if strings.Join(pe.KnownOperations, ",") != "get,list,schema" {
			t.Fatalf("%q: known operations = %v", input, pe.KnownOperations)
		}
		msg := pe.Error()
		if !strings.Contains(msg, "known operations: get, list, schema") || !strings.Contains(msg, "schema(operation=NAME)") {
			t.Fatalf("%q: message lacks recovery path: %s", input, msg)
		}
		if strings.Contains(msg, "expected ')'") || strings.Contains(msg, "unexpected character") {
			t.Fatalf("%q: regressed to positional error: %s", input, msg)
		}
	}

	// ValidateAST reaches the same verdict for a permissively parsed AST.
	ast, err := Parse("items(status=x) { id }", nil)
	if err != nil {
		t.Fatal(err)
	}
	pe, ok := s.ValidateAST(ast).(*ParseError)
	if !ok || !pe.IsUnknownOperation() || strings.Join(pe.KnownOperations, ",") != "get,list,schema" || pe.Hint == "" {
		t.Fatalf("ValidateAST verdict = %#v", pe)
	}
}

// TestPermissiveParseAttributesErrorsToOperation pins the field a permissive
// host relies on: an argument error names the statement it happened in.
func TestPermissiveParseAttributesErrorsToOperation(t *testing.T) {
	_, err := Parse("list(ok=1); items(status in [a]) { id }", nil)
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error = %T, want *ParseError", err)
	}
	if pe.Operation != "items" || pe.OperationPos == nil || pe.OperationPos.Column != 13 {
		t.Fatalf("attribution = %q at %+v, want items at column 13", pe.Operation, pe.OperationPos)
	}
	if pe.KnownOperations != nil || pe.IsUnknownOperation() {
		t.Fatalf("permissive parse must not claim an unknown operation: %#v", pe)
	}
	// A known operation with the same broken arguments keeps its positional
	// error text, at both the tokenizer and the parser stage.
	s := NewSchema[string]()
	s.Operation("list", func(ctx OperationContext[string]) (any, error) { return nil, nil })
	_, err = s.Parse("list(status in [a]) { id }")
	if err == nil || !strings.HasPrefix(err.Error(), "parse error at 1:16: unexpected character \"[\" (got \"[\")") {
		t.Fatalf("known operation tokenizer error changed: %v", err)
	}
	_, err = s.Parse("list(status in a) { id }")
	if err == nil || !strings.HasPrefix(err.Error(), "parse error at 1:13: expected ')' (got \"in\", expected ')')") {
		t.Fatalf("known operation positional error changed: %v", err)
	}
	if pe, ok := err.(*ParseError); !ok || pe.Operation != "list" {
		t.Fatalf("known operation error is not attributed: %#v", err)
	}
}
