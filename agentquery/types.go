package agentquery

// OutputMode controls how query and search results are serialized.
// HumanReadable (default) produces standard JSON.
// LLMReadable produces a compact tabular format optimized for LLM token efficiency.
type OutputMode int

const (
	// HumanReadable outputs standard JSON (default).
	HumanReadable OutputMode = iota
	// LLMReadable outputs compact tabular format: schema-once header + CSV-style rows.
	LLMReadable
)

// SearchProvider abstracts full-text search over a data source.
// Implementations may search the filesystem, a database, or a remote API.
type SearchProvider interface {
	Search(pattern string, opts SearchOptions) ([]SearchResult, error)
}

// FieldAccessor extracts a field value from a domain item.
// The accessor returns any because field values are heterogeneous
// (strings, ints, slices, etc.) and ultimately serialized to JSON.
type FieldAccessor[T any] func(item T) any

// SearchResult represents a single line from a scoped grep search.
type SearchResult struct {
	Source  Source `json:"source"`
	Content string `json:"content"`
	IsMatch bool   `json:"isMatch"`
}

// Source identifies where a search match was found.
type Source struct {
	Path string `json:"path"` // relative to data directory
	Line int    `json:"line"` // 1-indexed
}

// SearchOptions configures a search query.
type SearchOptions struct {
	FileGlob        string `json:"fileGlob,omitempty"`
	CaseInsensitive bool   `json:"caseInsensitive,omitempty"`
	ContextLines    int    `json:"contextLines,omitempty"`
}

// Pos represents a position in the input string.
type Pos struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

// ParameterDef describes a single parameter accepted by an operation.
// Used in OperationMetadata for schema introspection, so agents can discover
// valid parameters without external docs.
type ParameterDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`                  // "string", "int", "bool"
	Optional    bool   `json:"optional"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// OperationMetadata provides human/agent-readable documentation for an operation.
// Registered via Schema.OperationWithMetadata and surfaced by the built-in schema() operation.
type OperationMetadata struct {
	Description string         `json:"description,omitempty"`
	Parameters  []ParameterDef `json:"parameters,omitempty"`
	Examples    []string       `json:"examples,omitempty"`
}

// SortComparator defines ordering between two items for a specific field.
// Returns negative if a sorts before b, zero if equal, positive if a sorts after b.
// Defines the "natural" ascending order — descending is handled by negating the result.
type SortComparator[T any] func(a, b T) int

// SortSpec represents a parsed sort directive: field name + direction.
type SortSpec struct {
	Field     string
	Direction SortDirection
}

// SortDirection indicates ascending or descending order.
type SortDirection int

const (
	Asc  SortDirection = iota // default
	Desc
)

// OperationHandler is the function signature for operation implementations.
// It receives context with the parsed statement, field selector, and lazy item loader,
// and returns a JSON-serializable result or an error.
type OperationHandler[T any] func(ctx OperationContext[T]) (any, error)

// OperationContext provides data to operation handlers during query execution.
// The Items function is lazy — it's only called if the operation needs the full dataset.
// Predicate is auto-built from registered filterable fields and query args; defaults
// to MatchAll when no registered filters match the args.
type OperationContext[T any] struct {
	Statement Statement
	Selector  *FieldSelector[T]
	Items     func() ([]T, error)
	Predicate func(T) bool
}
