package agentquery

import (
	"errors"
	"fmt"
	"sort"
)

// FieldResolver validates and optionally expands field names in projections.
// If a name is a valid field, return ([]string{name}, nil).
// If a name is a preset, return the expanded field list.
// If a name is unknown, return (nil, error).
type FieldResolver interface {
	ResolveField(name string) ([]string, error)
}

// ParserConfig controls parser behavior.
// If nil is passed to Parse, permissive defaults are used.
type ParserConfig struct {
	// Operations restricts valid operation names. Nil = accept any.
	Operations map[string]bool

	// FieldResolver validates/expands field names in projections.
	// Nil = accept any identifier as a field name.
	FieldResolver FieldResolver
}

// Parse parses the input string into a Query AST.
// If config is nil, uses permissive defaults (any operation, no field validation).
func Parse(input string, config *ParserConfig) (*Query, error) {
	tok := newTokenizer(input)
	tokens, err := tok.tokenize()
	if err != nil {
		return nil, attributeTokenizeError(err, tok, config)
	}
	p := &parser{
		tokens: tokens,
		config: config,
		tzer:   tok,
	}
	return p.parseQuery()
}

// attributeTokenizeError names the statement a tokenizer error happened in,
// using the tokens emitted before the failure. When the parser is configured
// with Operations and that statement names an unknown one, the unknown-
// operation verdict wins: an agent that wrote `items(status in [a])` needs to
// hear that `items` does not exist before it hears that `[` is not a token.
func attributeTokenizeError(err error, tok *tokenizer, config *ParserConfig) error {
	pe, ok := err.(*ParseError)
	if !ok {
		return err
	}
	var (
		name    string
		namePos Pos
		found   bool
		atStart = true
	)
	for _, t := range tok.tokens {
		if t.pos > pe.Pos.Offset {
			break
		}
		switch {
		case t.typ == tokenSemicolon:
			atStart = true
		case atStart && t.typ == tokenIdent:
			name, namePos, found = t.val, tok.posAt(t.pos), true
			atStart = false
		default:
			atStart = false
		}
	}
	if !found {
		return pe
	}
	if config != nil && config.Operations != nil && !config.Operations[name] {
		known := make([]string, 0, len(config.Operations))
		for op, accepted := range config.Operations {
			if accepted {
				known = append(known, op)
			}
		}
		return NewUnknownOperationError(name, namePos, known)
	}
	pe.Operation = name
	pe.OperationPos = &Pos{Offset: namePos.Offset, Line: namePos.Line, Column: namePos.Column}
	return pe
}

// --- Token types ---

type tokenType int

const (
	tokenIdent     tokenType = iota // identifier: letters, digits, underscores, hyphens
	tokenString                     // quoted "..."
	tokenLParen                     // (
	tokenRParen                     // )
	tokenLBrace                     // {
	tokenRBrace                     // }
	tokenEquals                     // =
	tokenComma                      // ,
	tokenSemicolon                  // ;
	tokenEOF
)

func tokenTypeName(t tokenType) string {
	switch t {
	case tokenIdent:
		return "identifier"
	case tokenString:
		return "string"
	case tokenLParen:
		return "'('"
	case tokenRParen:
		return "')'"
	case tokenLBrace:
		return "'{'"
	case tokenRBrace:
		return "'}'"
	case tokenEquals:
		return "'='"
	case tokenComma:
		return "','"
	case tokenSemicolon:
		return "';'"
	case tokenEOF:
		return "end of input"
	default:
		return "unknown"
	}
}

type token struct {
	typ tokenType
	val string
	pos int // byte offset in input
}

// --- Tokenizer ---

type tokenizer struct {
	input      string
	pos        int
	tokens     []token
	lineStarts []int // byte offsets where each line starts
}

func newTokenizer(input string) *tokenizer {
	t := &tokenizer{
		input:      input,
		lineStarts: []int{0}, // line 1 starts at offset 0
	}
	// Pre-scan for newlines to build lineStarts table
	for i := 0; i < len(input); i++ {
		if input[i] == '\n' && i+1 <= len(input) {
			t.lineStarts = append(t.lineStarts, i+1)
		}
	}
	return t
}

// posAt converts a byte offset into a Pos with line and column.
func (t *tokenizer) posAt(offset int) Pos {
	// Binary search for the line containing this offset
	line := sort.Search(len(t.lineStarts), func(i int) bool {
		return t.lineStarts[i] > offset
	})
	// line is now the index of the first lineStart > offset, so the actual line is line (1-based)
	col := offset - t.lineStarts[line-1] + 1
	return Pos{Offset: offset, Line: line, Column: col}
}

func (t *tokenizer) tokenize() ([]token, error) {
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
			if err := t.readString(); err != nil {
				return nil, err
			}
		default:
			if isIdentStart(ch) {
				t.readIdent()
			} else {
				pos := t.posAt(t.pos)
				return nil, &ParseError{
					Message: fmt.Sprintf("unexpected character %q", string(ch)),
					Pos:     pos,
					Got:     string(ch),
				}
			}
		}
	}
	t.tokens = append(t.tokens, token{typ: tokenEOF, pos: t.pos})
	return t.tokens, nil
}

func (t *tokenizer) emit(typ tokenType, val string) {
	t.tokens = append(t.tokens, token{typ: typ, val: val, pos: t.pos})
	t.pos++
}

func (t *tokenizer) readString() error {
	startPos := t.pos
	t.pos++ // skip opening quote
	var result []byte
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if ch == '\\' && t.pos+1 < len(t.input) {
			// Backslash escaping
			next := t.input[t.pos+1]
			if decoded, ok := stringEscapes[next]; ok {
				result = append(result, decoded)
			} else {
				result = append(result, '\\', next)
			}
			t.pos += 2
			continue
		}
		if ch == '"' {
			t.tokens = append(t.tokens, token{typ: tokenString, val: string(result), pos: startPos})
			t.pos++
			return nil
		}
		result = append(result, ch)
		t.pos++
	}
	pos := t.posAt(startPos)
	return &ParseError{
		Message: "unterminated string literal",
		Pos:     pos,
		Got:     t.input[startPos:],
	}
}

func (t *tokenizer) readIdent() {
	start := t.pos
	for t.pos < len(t.input) && isIdentChar(t.input[t.pos]) {
		t.pos++
	}
	t.tokens = append(t.tokens, token{typ: tokenIdent, val: t.input[start:t.pos], pos: start})
}

// isIdentStart checks if a byte can start an identifier.
// Permissive: letters, digits, underscore.
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

// isIdentChar checks if a byte can appear in an identifier (after the first character).
// Permissive: letters, digits, underscore, hyphen, dot, slash.
func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || ch == '-' || ch == '.' || ch == '/'
}

// --- Recursive Descent Parser ---

type parser struct {
	tokens []token
	pos    int
	config *ParserConfig
	tzer   *tokenizer
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{typ: tokenEOF, pos: len(p.tzer.input)}
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
		pos := p.tzer.posAt(tok.pos)
		got := tok.val
		if tok.typ == tokenEOF {
			got = "end of input"
		}
		return tok, &ParseError{
			Message:  fmt.Sprintf("expected %s", tokenTypeName(typ)),
			Pos:      pos,
			Got:      got,
			Expected: tokenTypeName(typ),
		}
	}
	return tok, nil
}

// parseQuery parses the top-level batch: query (';' query)*
func (p *parser) parseQuery() (*Query, error) {
	q := &Query{}

	// Skip leading semicolons
	for p.peek().typ == tokenSemicolon {
		p.advance()
	}

	if p.peek().typ == tokenEOF {
		pos := p.tzer.posAt(p.peek().pos)
		return nil, &ParseError{
			Message: "empty query",
			Pos:     pos,
			Got:     "end of input",
		}
	}

	for {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		q.Statements = append(q.Statements, *stmt)

		// Skip semicolons between statements
		if p.peek().typ == tokenSemicolon {
			for p.peek().typ == tokenSemicolon {
				p.advance()
			}
			if p.peek().typ == tokenEOF {
				break
			}
			continue
		}

		if p.peek().typ == tokenEOF {
			break
		}

		// Neither semicolon nor EOF — unexpected token
		tok := p.peek()
		pos := p.tzer.posAt(tok.pos)
		return nil, &ParseError{
			Message:  "expected ';' or end of input",
			Pos:      pos,
			Got:      tok.val,
			Expected: "';' or end of input",
		}
	}

	return q, nil
}

// parseStatement parses: operation '(' args ')' ['{' fields '}']
func (p *parser) parseStatement() (*Statement, error) {
	// Parse operation name
	opTok := p.advance()
	if opTok.typ != tokenIdent {
		pos := p.tzer.posAt(opTok.pos)
		got := opTok.val
		if opTok.typ == tokenEOF {
			got = "end of input"
		}
		return nil, &ParseError{
			Message:  "expected operation name",
			Pos:      pos,
			Got:      got,
			Expected: "identifier",
		}
	}

	// Validate operation name if config restricts them
	if p.config != nil && p.config.Operations != nil {
		if !p.config.Operations[opTok.val] {
			known := make([]string, 0, len(p.config.Operations))
			for name, accepted := range p.config.Operations {
				if accepted {
					known = append(known, name)
				}
			}
			return nil, NewUnknownOperationError(opTok.val, p.tzer.posAt(opTok.pos), known)
		}
	}

	stmtPos := p.tzer.posAt(opTok.pos)
	body, err := p.parseStatementBody(opTok.val, stmtPos)
	if err != nil {
		// Every error inside the statement body is attributed to its operation
		// so a permissive host can decide whether the operation itself was the
		// problem before it reports an argument position.
		var parseErr *ParseError
		if errors.As(err, &parseErr) && parseErr.Operation == "" {
			parseErr.Operation = opTok.val
			parseErr.OperationPos = &Pos{Offset: stmtPos.Offset, Line: stmtPos.Line, Column: stmtPos.Column}
		}
		return nil, err
	}
	return body, nil
}

// parseStatementBody parses everything after the operation name:
// '(' args ')' ['{' fields '}'].
func (p *parser) parseStatementBody(operation string, stmtPos Pos) (*Statement, error) {

	// Expect '('
	if _, err := p.expect(tokenLParen); err != nil {
		return nil, err
	}

	// Parse args (possibly empty)
	var args []Arg
	var err error
	if p.peek().typ != tokenRParen {
		args, err = p.parseArgs()
		if err != nil {
			return nil, err
		}
	}

	// Expect ')'
	if _, err := p.expect(tokenRParen); err != nil {
		return nil, err
	}

	// Optional field projection: '{' fields '}'
	var fields []string
	if p.peek().typ == tokenLBrace {
		fields, err = p.parseProjection()
		if err != nil {
			return nil, err
		}
	}

	return &Statement{
		Operation: operation,
		Args:      args,
		Fields:    fields,
		Pos:       stmtPos,
	}, nil
}

// parseArgs parses: arg (',' arg)*
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
		p.advance() // consume comma
	}
	return args, nil
}

// parseArg parses: ident '=' value | value
// where value = ident | quoted_string
func (p *parser) parseArg() (*Arg, error) {
	tok := p.advance()
	if tok.typ != tokenIdent && tok.typ != tokenString {
		pos := p.tzer.posAt(tok.pos)
		got := tok.val
		if tok.typ == tokenEOF {
			got = "end of input"
		}
		return nil, &ParseError{
			Message:  "expected argument",
			Pos:      pos,
			Got:      got,
			Expected: "identifier or string",
		}
	}

	argPos := p.tzer.posAt(tok.pos)

	// Check for key=value form
	if p.peek().typ == tokenEquals {
		p.advance() // consume '='
		valTok := p.advance()
		if valTok.typ != tokenIdent && valTok.typ != tokenString {
			pos := p.tzer.posAt(valTok.pos)
			got := valTok.val
			if valTok.typ == tokenEOF {
				got = "end of input"
			}
			return nil, &ParseError{
				Message:  "expected value after '='",
				Pos:      pos,
				Got:      got,
				Expected: "identifier or string",
			}
		}
		return &Arg{Key: tok.val, Value: valTok.val, Pos: argPos}, nil
	}

	// Positional argument
	return &Arg{Value: tok.val, Pos: argPos}, nil
}

// parseProjection parses: '{' ident+ '}'
// If a FieldResolver is configured, each ident is resolved (may expand presets).
func (p *parser) parseProjection() ([]string, error) {
	p.advance() // consume '{'

	var fields []string
	for p.peek().typ != tokenRBrace && p.peek().typ != tokenEOF {
		tok := p.advance()
		if tok.typ != tokenIdent {
			pos := p.tzer.posAt(tok.pos)
			return nil, &ParseError{
				Message:  "expected field name",
				Pos:      pos,
				Got:      tok.val,
				Expected: "identifier",
			}
		}

		// Resolve field through FieldResolver if configured
		if p.config != nil && p.config.FieldResolver != nil {
			resolved, err := p.config.FieldResolver.ResolveField(tok.val)
			if err != nil {
				pos := p.tzer.posAt(tok.pos)
				return nil, &ParseError{
					Message: fmt.Sprintf("unknown field %q", tok.val),
					Pos:     pos,
					Got:     tok.val,
				}
			}
			fields = append(fields, resolved...)
		} else {
			fields = append(fields, tok.val)
		}
	}

	if _, err := p.expect(tokenRBrace); err != nil {
		return nil, err
	}

	// Deduplicate while preserving order
	seen := make(map[string]bool)
	var unique []string
	for _, f := range fields {
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}

	return unique, nil
}
