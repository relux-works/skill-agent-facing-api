package agentquery

// Query is the top-level AST node containing one or more statements (batch support).
type Query struct {
	Statements []Statement `json:"statements"`
}

// Statement is a single operation call with arguments and optional field projection.
type Statement struct {
	Operation string   `json:"operation"`
	Args      []Arg    `json:"args,omitempty"`
	Fields    []string `json:"fields,omitempty"` // raw identifiers; preset expansion happens at resolver
	Pos       Pos      `json:"pos"`
}

// Arg is a positional value or key=value pair within a statement's argument list.
type Arg struct {
	Key   string `json:"key,omitempty"` // empty for positional args
	Value string `json:"value"`
	Pos   Pos    `json:"pos"`
}
