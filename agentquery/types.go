package agentquery

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

// OperationHandler is the function signature for operation implementations.
// It receives context with the parsed statement, field selector, and lazy item loader,
// and returns a JSON-serializable result or an error.
type OperationHandler[T any] func(ctx OperationContext[T]) (any, error)

// OperationContext provides data to operation handlers during query execution.
// The Items function is lazy — it's only called if the operation needs the full dataset.
type OperationContext[T any] struct {
	Statement Statement
	Selector  *FieldSelector[T]
	Items     func() ([]T, error)
}
