package agentquery

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type qmParser struct {
	input     string
	pos       int
	limits    QueryLimits
	nodes     uint64
	statement int
}

func ParseQueryModel(input string, limits QueryLimits) (*QueryModel, error) {
	l, err := normalizeQueryLimits(limits)
	if err != nil {
		return nil, err
	}
	if !utf8.ValidString(input) {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	if uint64(len(input)) > l.MaxSourceBytes {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	p := &qmParser{input: input, limits: l}
	m := &QueryModel{limitDigest: queryLimitsDigest(l)}
	p.ws()
	for p.take(';') {
		p.ws()
	}
	for !p.end() {
		if uint64(len(m.Statements)) >= l.MaxStatements {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		p.statement = len(m.Statements) + 1
		p.nodes = 0
		s, e := p.parseStatement()
		if e != nil {
			return nil, e
		}
		m.Statements = append(m.Statements, s)
		p.ws()
		if p.end() {
			break
		}
		if !p.take(';') {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		for p.take(';') {
			p.ws()
		}
	}
	if len(m.Statements) == 0 {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	m.parsedDigest = queryModelParsedDigest(m)
	return m, nil
}

func queryModelParsedDigest(m *QueryModel) [32]byte {
	if m == nil {
		return [32]byte{}
	}
	return queryModelParsedDigestStatements(m.Statements)
}

func queryModelParsedDigestStatements(statements []QueryStatement) [32]byte {
	b, err := json.Marshal(statements)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(b)
}

func (p *qmParser) parseStatement() (QueryStatement, error) {
	start := p.position()
	op, err := p.ident(true)
	if err != nil {
		return QueryStatement{}, err
	}
	if !p.take('(') {
		return QueryStatement{}, qmError("predicate_syntax", "invalid query syntax")
	}
	var args []Arg
	p.ws()
	if !p.take(')') {
		for {
			ap := p.position()
			first, quoted, e := p.legacyValue()
			if e != nil {
				return QueryStatement{}, e
			}
			p.ws()
			if p.take('=') {
				if quoted {
					return QueryStatement{}, qmError("predicate_syntax", "invalid query syntax")
				}
				value, _, e := p.legacyValue()
				if e != nil {
					return QueryStatement{}, e
				}
				args = append(args, Arg{Key: first, Value: value, Pos: ap})
			} else {
				args = append(args, Arg{Value: first, Pos: ap})
			}
			p.ws()
			if p.take(')') {
				break
			}
			if !p.take(',') {
				return QueryStatement{}, qmError("predicate_syntax", "invalid query syntax")
			}
		}
	}
	s := QueryStatement{Call: Statement{Operation: op, Args: args, Pos: start}}
	p.ws()
	if p.keyword("where") {
		p.nodes = 0
		s.Where, err = p.expression(1)
		if err != nil {
			return QueryStatement{}, err
		}
	}
	p.ws()
	if p.keyword("sortOrder") {
		if !p.take('(') {
			return s, qmError("predicate_syntax", "invalid query syntax")
		}
		for {
			pos := p.position()
			f, e := p.ident(false)
			if e != nil {
				return s, e
			}
			d, e := p.ident(false)
			if e != nil {
				return s, e
			}
			dir := QuerySortDirection(d)
			if dir != QuerySortAscending && dir != QuerySortDescending {
				return s, qmError("query_modifier", "invalid query modifier")
			}
			s.SortOrder = append(s.SortOrder, SortCriterion{Field: f, Direction: dir, Pos: pos})
			if uint64(len(s.SortOrder)) > p.limits.MaxSortCriteria {
				return s, qmError("predicate_limit", "query limit exceeded")
			}
			if p.take(')') {
				break
			}
			if !p.take(',') {
				return s, qmError("predicate_syntax", "invalid query syntax")
			}
		}
	}
	p.ws()
	if p.keyword("groupBy") {
		if !p.take('(') {
			return s, qmError("predicate_syntax", "invalid query syntax")
		}
		pos := p.position()
		f, e := p.ident(false)
		if e != nil {
			return s, e
		}
		if !p.take(')') {
			return s, qmError("predicate_syntax", "invalid query syntax")
		}
		s.GroupBy = &GroupCriterion{Field: f, Pos: pos}
	}
	p.ws()
	if p.keyword("skip") {
		v, e := p.unsigned()
		if e != nil {
			return s, e
		}
		s.Skip = &v
	}
	p.ws()
	if p.keyword("take") {
		v, e := p.unsigned()
		if e != nil {
			return s, e
		}
		s.Take = &v
	}
	p.ws()
	if p.take('{') {
		for {
			p.ws()
			if p.take('}') {
				break
			}
			f, e := p.ident(false)
			if e != nil {
				return s, e
			}
			s.Call.Fields = append(s.Call.Fields, f)
		}
	}
	return s, nil
}

func (p *qmParser) expression(depth uint64) (*Expression, error) {
	if depth > p.limits.MaxExpressionDepth {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	p.nodes++
	if p.nodes > p.limits.MaxExpressionNodes {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	pos := p.position()
	name, e := p.ident(false)
	if e != nil {
		return nil, e
	}
	if !p.take('(') {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	switch name {
	case "not":
		c, e := p.expression(depth + 1)
		if e != nil {
			return nil, e
		}
		if !p.take(')') {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		return &Expression{Kind: ExpressionNot, Child: c, Pos: pos}, nil
	case "satisfiesAll", "satisfiesAny":
		var cs []*Expression
		if p.take(')') {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		for {
			c, e := p.expression(depth + 1)
			if e != nil {
				return nil, e
			}
			cs = append(cs, c)
			if p.take(')') {
				break
			}
			if !p.take(',') {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
		}
		k := ExpressionSatisfiesAll
		if name == "satisfiesAny" {
			k = ExpressionSatisfiesAny
		}
		return &Expression{Kind: k, Children: cs, Pos: pos}, nil
	}
	op := PredicateOperator(name)
	if !knownOperator(op) {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	field, e := p.ident(false)
	if e != nil {
		return nil, e
	}
	pred := &Predicate{Field: field, Operator: op}
	if op == OperatorIsNull || op == OperatorIsMissing {
		if !p.take(')') {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
	} else {
		if !p.take(',') {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		switch op {
		case OperatorInRange:
			a, e := p.literal()
			if e != nil {
				return nil, e
			}
			if !p.take(',') {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
			b, e := p.literal()
			if e != nil {
				return nil, e
			}
			pred.Lower = &a
			pred.Upper = &b
		case OperatorInSet:
			if !p.take('[') {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
			if p.take(']') {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
			for {
				v, e := p.literal()
				if e != nil {
					return nil, e
				}
				pred.Values = append(pred.Values, v)
				if uint64(len(pred.Values)) > p.limits.MaxSetEntries {
					return nil, qmError("predicate_limit", "query limit exceeded")
				}
				if p.take(']') {
					break
				}
				if !p.take(',') {
					return nil, qmError("predicate_syntax", "invalid query syntax")
				}
			}
		default:
			v, e := p.literal()
			if e != nil {
				return nil, e
			}
			pred.Value = &v
		}
		if !p.take(')') {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
	}
	return &Expression{Kind: ExpressionPredicate, Predicate: pred, Pos: pos}, nil
}

func knownOperator(o PredicateOperator) bool {
	switch o {
	case OperatorEquals, OperatorNotEquals, OperatorContains, OperatorMatchesRegex, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorHasElement, OperatorIsNull, OperatorIsMissing:
		return true
	}
	return false
}
func (p *qmParser) literal() (Literal, error) {
	p.ws()
	if p.peek() == '"' {
		s, e := p.stringValue()
		if e != nil {
			return Literal{}, e
		}
		return StringValue(s), nil
	}
	if p.keyword("true") {
		return BooleanValue(true), nil
	}
	if p.keyword("false") {
		return BooleanValue(false), nil
	}
	if p.keyword("timestamp") {
		if !p.take('(') {
			return Literal{}, qmError("predicate_syntax", "invalid query syntax")
		}
		s, e := p.stringValue()
		if e != nil {
			return Literal{}, e
		}
		if !p.take(')') {
			return Literal{}, qmError("predicate_syntax", "invalid query syntax")
		}
		if !validTimestamp(s) {
			return Literal{}, qmError("predicate_type", "invalid predicate value")
		}
		return Literal{Kind: LiteralTimestamp, Text: s}, nil
	}
	start := p.pos
	if p.peek() == '+' {
		return Literal{}, qmError("predicate_syntax", "invalid query syntax")
	}
	if p.peek() == '-' {
		p.pos++
	}
	digits := p.pos
	for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
	}
	if digits == p.pos {
		return Literal{}, qmError("predicate_syntax", "invalid query syntax")
	}
	raw := p.input[start:p.pos]
	mag := p.input[digits:p.pos]
	if (len(mag) > 1 && mag[0] == '0') || raw == "-0" {
		return Literal{}, qmError("predicate_syntax", "invalid query syntax")
	}
	v, e := strconv.ParseInt(raw, 10, 64)
	if e != nil {
		return Literal{}, qmError("predicate_type", "invalid predicate value")
	}
	return Int64Value(v), nil
}
func validTimestamp(s string) bool {
	if len(s) < 20 || len(s) > 35 ||
		!decimalDigits(s, 0, 4) || s[:4] == "0000" ||
		s[4] != '-' || !decimalDigits(s, 5, 7) ||
		s[7] != '-' || !decimalDigits(s, 8, 10) ||
		s[10] != 'T' || !decimalDigits(s, 11, 13) ||
		s[13] != ':' || !decimalDigits(s, 14, 16) ||
		s[16] != ':' || !decimalDigits(s, 17, 19) ||
		decimalValue(s, 11, 13) > 23 || decimalValue(s, 14, 16) > 59 || decimalValue(s, 17, 19) > 59 {
		return false
	}
	pos := 19
	if pos < len(s) && s[pos] == '.' {
		fractionStart := pos + 1
		pos = fractionStart
		for pos < len(s) && s[pos] >= '0' && s[pos] <= '9' {
			pos++
		}
		if digits := pos - fractionStart; digits < 1 || digits > 9 {
			return false
		}
	}
	if pos == len(s)-1 && s[pos] == 'Z' {
		// UTC is the only single-byte zone designator in the frozen grammar.
	} else {
		if len(s)-pos != 6 || (s[pos] != '+' && s[pos] != '-') ||
			!decimalDigits(s, pos+1, pos+3) || s[pos+3] != ':' ||
			!decimalDigits(s, pos+4, pos+6) ||
			decimalValue(s, pos+1, pos+3) > 23 || decimalValue(s, pos+4, pos+6) > 59 ||
			s[pos:] == "-00:00" {
			return false
		}
	}
	_, e := time.Parse(time.RFC3339Nano, s)
	return e == nil
}

func decimalDigits(s string, start, end int) bool {
	if start < 0 || end > len(s) || start >= end {
		return false
	}
	for i := start; i < end; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func decimalValue(s string, start, end int) int {
	value := 0
	for i := start; i < end; i++ {
		value = value*10 + int(s[i]-'0')
	}
	return value
}
func (p *qmParser) unsigned() (uint64, error) {
	p.ws()
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
	}
	if start == p.pos {
		return 0, qmError("predicate_syntax", "invalid query syntax")
	}
	raw := p.input[start:p.pos]
	if len(raw) > 1 && raw[0] == '0' {
		return 0, qmError("predicate_syntax", "invalid query syntax")
	}
	v, e := strconv.ParseUint(raw, 10, 64)
	if e != nil {
		return 0, qmError("predicate_limit", "query limit exceeded")
	}
	return v, nil
}
func (p *qmParser) legacyValue() (string, bool, error) {
	p.ws()
	if p.peek() == '"' {
		s, e := p.stringValue()
		return s, true, e
	}
	s, e := p.ident(true)
	return s, false, e
}
func (p *qmParser) stringValue() (string, error) {
	p.ws()
	if p.peek() != '"' {
		return "", qmError("predicate_syntax", "invalid query syntax")
	}
	p.pos++
	var b strings.Builder
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '"' {
			p.pos++
			if uint64(b.Len()) > p.limits.MaxLiteralBytes {
				return "", qmError("predicate_limit", "query limit exceeded")
			}
			return b.String(), nil
		}
		if c < 0x20 {
			return "", qmError("predicate_syntax", "invalid query syntax")
		}
		if c == '\\' {
			if p.pos+1 >= len(p.input) {
				break
			}
			n := p.input[p.pos+1]
			if n < 0x20 {
				return "", qmError("predicate_syntax", "invalid query syntax")
			}
			switch n {
			case '"', '\\':
				b.WriteByte(n)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte('\\')
				b.WriteByte(n)
			}
			p.pos += 2
			continue
		}
		b.WriteByte(c)
		p.pos++
	}
	return "", qmError("predicate_syntax", "invalid query syntax")
}
func (p *qmParser) ident(legacy bool) (string, error) {
	p.ws()
	start := p.pos
	if start >= len(p.input) || !((p.input[start] >= 'a' && p.input[start] <= 'z') || (p.input[start] >= 'A' && p.input[start] <= 'Z') || (p.input[start] >= '0' && p.input[start] <= '9') || p.input[start] == '_') {
		return "", qmError("predicate_syntax", "invalid query syntax")
	}
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || (legacy && (c == '-' || c == '.' || c == '/'))
		if !ok {
			break
		}
		p.pos++
	}
	if start == p.pos || (!legacy && p.input[start] >= '0' && p.input[start] <= '9') {
		return "", qmError("predicate_syntax", "invalid query syntax")
	}
	return p.input[start:p.pos], nil
}
func (p *qmParser) keyword(s string) bool {
	p.ws()
	if !strings.HasPrefix(p.input[p.pos:], s) {
		return false
	}
	end := p.pos + len(s)
	if end < len(p.input) {
		c := p.input[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			return false
		}
	}
	p.pos = end
	p.ws()
	return true
}
func (p *qmParser) take(c byte) bool {
	p.ws()
	if p.pos < len(p.input) && p.input[p.pos] == c {
		p.pos++
		p.ws()
		return true
	}
	return false
}
func (p *qmParser) peek() byte {
	p.ws()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}
func (p *qmParser) ws() {
	for p.pos < len(p.input) {
		switch p.input[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}
func (p *qmParser) end() bool { p.ws(); return p.pos >= len(p.input) }
func (p *qmParser) position() Pos {
	line, col := 1, 1
	for i := 0; i < p.pos; i++ {
		if p.input[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return Pos{Offset: p.pos, Line: line, Column: col}
}

func RenderQueryModel(model *QueryModel) (string, error) {
	if model == nil || len(model.Statements) == 0 {
		return "", qmError("predicate_syntax", "invalid query syntax")
	}
	parts := make([]string, len(model.Statements))
	for i, s := range model.Statements {
		v, e := renderQueryStatement(s)
		if e != nil {
			return "", e
		}
		parts[i] = v
	}
	return strings.Join(parts, "; "), nil
}
func renderQueryStatement(s QueryStatement) (string, error) {
	if !validLegacyIdentifier(s.Call.Operation) {
		return "", qmError("predicate_syntax", "invalid query syntax")
	}
	for _, a := range s.Call.Args {
		if (a.Key != "" && !validLegacyIdentifier(a.Key)) || !validRenderableString(a.Value) {
			return "", qmError("predicate_syntax", "invalid query syntax")
		}
	}
	for _, field := range s.Call.Fields {
		if !validLegacyIdentifier(field) {
			return "", qmError("predicate_syntax", "invalid query syntax")
		}
	}
	for _, criterion := range s.SortOrder {
		if !validFieldIdentifier(criterion.Field) || (criterion.Direction != QuerySortAscending && criterion.Direction != QuerySortDescending) {
			return "", qmError("predicate_syntax", "invalid query syntax")
		}
	}
	if s.GroupBy != nil && !validFieldIdentifier(s.GroupBy.Field) {
		return "", qmError("predicate_syntax", "invalid query syntax")
	}
	var b strings.Builder
	b.WriteString(s.Call.Operation)
	b.WriteByte('(')
	for i, a := range s.Call.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		if a.Key != "" {
			b.WriteString(a.Key)
			b.WriteByte('=')
		}
		writeQuotedValue(&b, a.Value)
	}
	b.WriteByte(')')
	if s.Where != nil {
		b.WriteString(" where ")
		if e := renderExpression(&b, s.Where, map[*Expression]bool{}); e != nil {
			return "", e
		}
	}
	if len(s.SortOrder) > 0 {
		b.WriteString(" sortOrder(")
		for i, c := range s.SortOrder {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(c.Field)
			b.WriteByte(' ')
			b.WriteString(string(c.Direction))
		}
		b.WriteByte(')')
	}
	if s.GroupBy != nil {
		fmt.Fprintf(&b, " groupBy(%s)", s.GroupBy.Field)
	}
	if s.Skip != nil {
		fmt.Fprintf(&b, " skip %d", *s.Skip)
	}
	if s.Take != nil {
		fmt.Fprintf(&b, " take %d", *s.Take)
	}
	if len(s.Call.Fields) > 0 {
		b.WriteString(" { ")
		b.WriteString(strings.Join(s.Call.Fields, " "))
		b.WriteString(" }")
	}
	return b.String(), nil
}
func renderExpression(b *strings.Builder, e *Expression, path map[*Expression]bool) error {
	if e == nil || path[e] {
		return qmError("predicate_syntax", "invalid query syntax")
	}
	path[e] = true
	defer delete(path, e)
	switch e.Kind {
	case ExpressionNot:
		if e.Child == nil || e.Predicate != nil || len(e.Children) > 0 {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		b.WriteString("not(")
		if err := renderExpression(b, e.Child, path); err != nil {
			return err
		}
		b.WriteByte(')')
	case ExpressionSatisfiesAll, ExpressionSatisfiesAny:
		if len(e.Children) == 0 || e.Child != nil || e.Predicate != nil {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		b.WriteString(string(e.Kind))
		b.WriteByte('(')
		for i, c := range e.Children {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := renderExpression(b, c, path); err != nil {
				return err
			}
		}
		b.WriteByte(')')
	case ExpressionPredicate:
		if e.Predicate == nil || e.Child != nil || len(e.Children) > 0 {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		p := e.Predicate
		if !validFieldIdentifier(p.Field) {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		b.WriteString(string(p.Operator))
		b.WriteByte('(')
		b.WriteString(p.Field)
		switch p.Operator {
		case OperatorIsNull, OperatorIsMissing:
			if p.Value != nil || p.Lower != nil || p.Upper != nil || len(p.Values) > 0 {
				return qmError("predicate_syntax", "invalid query syntax")
			}
		case OperatorInRange:
			if p.Lower == nil || p.Upper == nil || p.Value != nil || len(p.Values) > 0 {
				return qmError("predicate_syntax", "invalid query syntax")
			}
			if !validLiteral(*p.Lower) || !validLiteral(*p.Upper) {
				return qmError("predicate_syntax", "invalid query syntax")
			}
			b.WriteString(", ")
			renderLiteral(b, *p.Lower)
			b.WriteString(", ")
			renderLiteral(b, *p.Upper)
		case OperatorInSet:
			if len(p.Values) == 0 || p.Value != nil || p.Lower != nil || p.Upper != nil {
				return qmError("predicate_syntax", "invalid query syntax")
			}
			b.WriteString(", [")
			for i, v := range p.Values {
				if !validLiteral(v) {
					return qmError("predicate_syntax", "invalid query syntax")
				}
				if i > 0 {
					b.WriteString(", ")
				}
				renderLiteral(b, v)
			}
			b.WriteByte(']')
		default:
			if !knownOperator(p.Operator) || p.Lower != nil || p.Upper != nil || len(p.Values) > 0 {
				return qmError("predicate_syntax", "invalid query syntax")
			}
			b.WriteString(", ")
			if p.Value == nil {
				return qmError("predicate_syntax", "invalid query syntax")
			}
			if !validLiteral(*p.Value) {
				return qmError("predicate_syntax", "invalid query syntax")
			}
			renderLiteral(b, *p.Value)
		}
		b.WriteByte(')')
	default:
		return qmError("predicate_syntax", "invalid query syntax")
	}
	return nil
}
func validLiteral(l Literal) bool {
	switch l.Kind {
	case LiteralString:
		return validRenderableString(l.Text)
	case LiteralInt64, LiteralBoolean:
		return true
	case LiteralTimestamp:
		return validRenderableString(l.Text) && validTimestamp(l.Text)
	}
	return false
}

func validLegacyIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || (i > 0 && (c == '-' || c == '.' || c == '/'))) {
			return false
		}
	}
	return true
}

func validFieldIdentifier(value string) bool {
	if value == "" || (value[0] >= '0' && value[0] <= '9') {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func validRenderableString(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 && value[i] != '\n' && value[i] != '\t' {
			return false
		}
	}
	return true
}
func renderLiteral(b *strings.Builder, l Literal) {
	switch l.Kind {
	case LiteralString:
		writeQuotedValue(b, l.Text)
	case LiteralInt64:
		fmt.Fprintf(b, "%d", l.Int64)
	case LiteralBoolean:
		fmt.Fprintf(b, "%t", l.Boolean)
	case LiteralTimestamp:
		b.WriteString("timestamp(")
		writeQuotedValue(b, l.Text)
		b.WriteByte(')')
	}
}
