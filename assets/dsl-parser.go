// Reference implementation: mini-query DSL parser.
// Recursive descent parser for the query grammar:
//
//   batch     = query (";" query)*
//   query     = operation "(" args ")" [ "{" fields "}" ]
//   args      = arg ("," arg)*  |  ε
//   arg       = ident "=" value  |  value       (key=value or positional)
//   fields    = ident+                           (field names or preset name)
//   value     = ident | quoted_string
//
// Adapt the valid operations, fields, and presets to your domain.
//
// Standard operations: get (by ID), list (with filters + skip/take pagination),
// count (with filters), summary (aggregates). The grammar supports key=value
// params natively, so pagination (skip=10, take=5) and filters (status=done)
// use the same arg syntax.

package query

import "fmt"

// --- Token types ---

type tokenType int

const (
	tokenIdent     tokenType = iota // identifier: letters, digits, underscores, hyphens
	tokenLParen                     // (
	tokenRParen                     // )
	tokenLBrace                     // {
	tokenRBrace                     // }
	tokenEquals                     // =
	tokenComma                      // ,
	tokenSemicolon                  // ;
	tokenString                     // quoted "..."
	tokenEOF
)

type token struct {
	typ tokenType
	val string
	pos int
}

// --- AST ---

// Query is the top-level node containing one or more statements (batch support).
type Query struct {
	Statements []Statement
}

// Statement is a single operation call with args and optional field projection.
type Statement struct {
	Operation string   // e.g. "get", "list", "summary"
	Args      []Arg    // positional and key=value arguments
	Fields    []string // resolved field names (presets expanded); nil = use default
}

// Arg is a positional value or key=value pair.
type Arg struct {
	Key   string // empty for positional args
	Value string
}

// --- Tokenizer ---

type tokenizer struct {
	input  string
	pos    int
	tokens []token
}

func tokenize(input string) ([]token, error) {
	t := &tokenizer{input: input}
	if err := t.run(); err != nil {
		return nil, err
	}
	return t.tokens, nil
}

func (t *tokenizer) run() error {
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			t.pos++
			continue
		}
		switch ch {
		case '(':
			t.emit(tokenLParen, "(")
		case ')':
			t.emit(tokenRParen, ")")
		case '{':
			t.emit(tokenLBrace, "{")
		case '}':
			t.emit(tokenRBrace, "}")
		case '=':
			t.emit(tokenEquals, "=")
		case ',':
			t.emit(tokenComma, ",")
		case ';':
			t.emit(tokenSemicolon, ";")
		case '"':
			str, err := t.readString()
			if err != nil {
				return err
			}
			t.tokens = append(t.tokens, token{typ: tokenString, val: str, pos: t.pos})
		default:
			if isIdentStart(ch) {
				ident := t.readIdent()
				t.tokens = append(t.tokens, token{typ: tokenIdent, val: ident, pos: t.pos})
			} else {
				return fmt.Errorf("unexpected character '%c' at position %d", ch, t.pos)
			}
		}
	}
	t.tokens = append(t.tokens, token{typ: tokenEOF, pos: t.pos})
	return nil
}

func (t *tokenizer) emit(typ tokenType, val string) {
	t.tokens = append(t.tokens, token{typ: typ, val: val, pos: t.pos})
	t.pos++
}

func (t *tokenizer) readString() (string, error) {
	t.pos++ // skip opening quote
	start := t.pos
	for t.pos < len(t.input) {
		if t.input[t.pos] == '"' {
			result := t.input[start:t.pos]
			t.pos++
			return result, nil
		}
		t.pos++
	}
	return "", fmt.Errorf("unterminated string at position %d", start-1)
}

func (t *tokenizer) readIdent() string {
	start := t.pos
	for t.pos < len(t.input) && isIdentChar(t.input[t.pos]) {
		t.pos++
	}
	return t.input[start:t.pos]
}

// Adapt these to your ID format.
// Example: ITEM-260211-abc123 requires hyphens in identifiers.
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || ch == '-'
}

// --- Parser ---

type parser struct {
	tokens []token
	pos    int
}

// ParseQuery parses a full query string (with batch support) into an AST.
func ParseQuery(input string) (*Query, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	return p.parseQuery()
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{typ: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() token {
	tok := p.peek()
	if tok.typ != tokenEOF {
		p.pos++
	}
	return tok
}

func (p *parser) expect(typ tokenType) (token, error) {
	tok := p.advance()
	if tok.typ != typ {
		return tok, fmt.Errorf("expected %d at position %d, got %q", typ, tok.pos, tok.val)
	}
	return tok, nil
}

// --- ADAPT THIS: your valid operations ---
var validOperations = map[string]bool{
	"get":     true,
	"list":    true,
	"count":   true,
	"summary": true,
}

// --- ADAPT THIS: your valid fields and presets ---
var validFields = map[string]bool{
	"id": true, "name": true, "status": true, "assignee": true,
	"description": true, "parent": true, "created": true, "updated": true,
}

var presets = map[string][]string{
	"minimal":  {"id", "status"},
	"default":  {"id", "name", "status"},
	"overview": {"id", "name", "status", "assignee", "parent"},
	"full":     {"id", "name", "status", "assignee", "description", "parent", "created", "updated"},
}

func (p *parser) parseQuery() (*Query, error) {
	q := &Query{}
	for {
		for p.peek().typ == tokenSemicolon {
			p.advance()
		}
		if p.peek().typ == tokenEOF {
			break
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		q.Statements = append(q.Statements, *stmt)
		if p.peek().typ == tokenSemicolon {
			p.advance()
			continue
		}
		if p.peek().typ == tokenEOF {
			break
		}
		return nil, fmt.Errorf("expected ';' or end of input at position %d", p.peek().pos)
	}
	if len(q.Statements) == 0 {
		return nil, fmt.Errorf("empty query")
	}
	return q, nil
}

func (p *parser) parseStatement() (*Statement, error) {
	opTok, err := p.expect(tokenIdent)
	if err != nil {
		return nil, fmt.Errorf("expected operation name: %w", err)
	}
	if !validOperations[opTok.val] {
		return nil, fmt.Errorf("unknown operation %q at position %d", opTok.val, opTok.pos)
	}

	if _, err := p.expect(tokenLParen); err != nil {
		return nil, err
	}

	var args []Arg
	if p.peek().typ != tokenRParen {
		args, err = p.parseArgs()
		if err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(tokenRParen); err != nil {
		return nil, err
	}

	var fields []string
	if p.peek().typ == tokenLBrace {
		fields, err = p.parseProjection()
		if err != nil {
			return nil, err
		}
	}

	return &Statement{Operation: opTok.val, Args: args, Fields: fields}, nil
}

func (p *parser) parseArgs() ([]Arg, error) {
	var args []Arg
	for {
		arg, err := p.parseArg()
		if err != nil {
			return nil, err
		}
		args = append(args, *arg)
		if p.peek().typ != tokenComma {
			break
		}
		p.advance()
	}
	return args, nil
}

func (p *parser) parseArg() (*Arg, error) {
	tok := p.advance()
	if tok.typ != tokenIdent && tok.typ != tokenString {
		return nil, fmt.Errorf("expected argument at position %d, got %q", tok.pos, tok.val)
	}
	if p.peek().typ == tokenEquals {
		p.advance()
		valTok := p.advance()
		if valTok.typ != tokenIdent && valTok.typ != tokenString {
			return nil, fmt.Errorf("expected value after '=' at position %d", valTok.pos)
		}
		return &Arg{Key: tok.val, Value: valTok.val}, nil
	}
	return &Arg{Value: tok.val}, nil
}

func (p *parser) parseProjection() ([]string, error) {
	p.advance() // consume {

	var fieldNames []string
	for p.peek().typ != tokenRBrace && p.peek().typ != tokenEOF {
		tok := p.advance()
		if tok.typ != tokenIdent {
			return nil, fmt.Errorf("expected field name at position %d, got %q", tok.pos, tok.val)
		}
		// Expand presets
		if expanded, ok := presets[tok.val]; ok {
			fieldNames = append(fieldNames, expanded...)
			continue
		}
		if !validFields[tok.val] {
			return nil, fmt.Errorf("unknown field %q at position %d", tok.val, tok.pos)
		}
		fieldNames = append(fieldNames, tok.val)
	}

	if _, err := p.expect(tokenRBrace); err != nil {
		return nil, err
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, f := range fieldNames {
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}
	return unique, nil
}
