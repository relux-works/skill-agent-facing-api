package agentquery

import (
	"fmt"
	"sort"
	"strings"
)

// stringEscapes maps the character after a backslash inside a quoted string
// to the byte it decodes to. The tokenizer reads through it, Render writes
// through its inverse and DSLGrammar publishes it, so the three cannot drift.
var stringEscapes = map[byte]byte{
	'"':  '"',
	'\\': '\\',
	'n':  '\n',
	't':  '\t',
}

// stringUnescapes is the inverse of stringEscapes: decoded byte to the
// character Render writes after the backslash.
var stringUnescapes = func() map[byte]byte {
	inverse := make(map[byte]byte, len(stringEscapes))
	for encoded, decoded := range stringEscapes {
		inverse[decoded] = encoded
	}
	return inverse
}()

// Grammar is the statement grammar of the query DSL as the parser accepts it.
// It is surfaced by the built-in schema() operation so an agent can discover
// the syntax through the facing API itself instead of a document outside it.
type Grammar struct {
	// Statement is the shape of one statement.
	Statement string `json:"statement"`
	// Batch describes how several statements are combined in one input.
	Batch string `json:"batch"`
	// Arguments describes the argument list between the parentheses.
	Arguments string `json:"arguments"`
	// Values describes the value forms an argument accepts.
	Values string `json:"values"`
	// Escapes lists the backslash escapes accepted inside a quoted string.
	Escapes []string `json:"escapes"`
	// Projection describes the optional field projection block.
	Projection string `json:"projection"`
	// Tokens lists every token the tokenizer produces, in tokenizer order.
	Tokens []string `json:"tokens"`
	// Examples are statement forms the parser accepts, one per form.
	Examples []string `json:"examples"`
}

// grammarExamples enumerates one accepted input per statement form. The drift
// test parses every one of them, so a form the parser stops accepting fails
// the build instead of staying advertised.
var grammarExamples = []string{
	`op()`,
	`op(key=value)`,
	`op(key=value, other=42, flag=true)`,
	`op(key="quoted value with \" \\ \n \t")`,
	`op(positional)`,
	`op() { field preset }`,
	`op(key=value) { field }; other()`,
}

// DSLGrammar returns the statement grammar derived from the parser: the token
// set comes from the tokenizer's token table and the escape list from the
// table the tokenizer decodes through.
func DSLGrammar() Grammar {
	tokens := make([]string, 0, int(tokenEOF)+1)
	for typ := tokenIdent; typ <= tokenEOF; typ++ {
		tokens = append(tokens, tokenTypeName(typ))
	}

	escapes := make([]string, 0, len(stringEscapes))
	for encoded := range stringEscapes {
		escapes = append(escapes, `\`+string(encoded))
	}
	sort.Strings(escapes)

	examples := make([]string, len(grammarExamples))
	copy(examples, grammarExamples)

	return Grammar{
		Statement:  "OPERATION(ARGUMENTS) { PROJECTION }; the projection block is optional",
		Batch:      "STATEMENT; STATEMENT — statements separated by ';', results keep source order",
		Arguments:  "comma-separated arguments: the first bare positional value is the element/identifier argument; remaining arguments use key=value; no operators or lists (not key in [a,b], not key!=value)",
		Values:     "a bare identifier (letters, digits, '_', '-', '.', '/'), an int, a bool (true|false), or a \"double-quoted string\"; quote any value containing spaces, commas or parentheses",
		Escapes:    escapes,
		Projection: "field projection: { field preset ... } — space-separated field names and preset names inside braces; omitted means the default fields",
		Tokens:     tokens,
		Examples:   examples,
	}
}

// GrammarSyntax is the one-line form of the grammar, meant for a scoped
// schema(operation=NAME) answer where the full block would drown the signature.
func GrammarSyntax() string {
	g := DSLGrammar()
	return fmt.Sprintf("OPERATION(element-or-identifier, key=value, key=\"quoted string\") { field preset }; statements separated by ';'; the first bare positional value is the element/identifier argument; remaining arguments use key=value (no operators or lists); string escapes: %s",
		strings.Join(g.Escapes, " "))
}
