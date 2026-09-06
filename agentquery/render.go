package agentquery

import "strings"

// Render serializes a parsed Query AST back to canonical DSL text.
// Argument values are always quoted so separators and whitespace remain data,
// never syntax. The escaping rules intentionally mirror tokenizer.readString.
func Render(q *Query) string {
	if q == nil {
		return ""
	}
	statements := make([]string, 0, len(q.Statements))
	for _, statement := range q.Statements {
		statements = append(statements, renderStatement(statement))
	}
	return strings.Join(statements, "; ")
}

func renderStatement(statement Statement) string {
	var b strings.Builder
	b.WriteString(statement.Operation)
	b.WriteByte('(')
	for i, arg := range statement.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		if arg.Key != "" {
			b.WriteString(arg.Key)
			b.WriteByte('=')
		}
		writeQuotedValue(&b, arg.Value)
	}
	b.WriteByte(')')
	if len(statement.Fields) > 0 {
		b.WriteString(" { ")
		b.WriteString(strings.Join(statement.Fields, " "))
		b.WriteString(" }")
	}
	return b.String()
}

func writeQuotedValue(b *strings.Builder, value string) {
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		if encoded, ok := stringUnescapes[value[i]]; ok {
			b.WriteByte('\\')
			b.WriteByte(encoded)
			continue
		}
		b.WriteByte(value[i])
	}
	b.WriteByte('"')
}
