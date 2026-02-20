package agentquery

import "fmt"

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
	Name        string   `json:"name"`
	Type        string   `json:"type"`                  // "string", "int", "bool"
	Optional    bool     `json:"optional"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`         // allowed values (validated by framework for mutations)
	Required    bool     `json:"required,omitempty"`     // explicit required flag (for mutation parameters)
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

// MutationHandler is the function signature for mutation implementations.
// It receives a MutationContext and returns a result object and/or error.
// The result should be a JSON-serializable representation of the affected entity
// (or a confirmation map for destructive operations like delete).
type MutationHandler[T any] func(ctx MutationContext[T]) (any, error)

// MutationContext provides data to mutation handlers during execution.
// Unlike OperationContext, it does not include Selector (no field projection)
// but adds ArgMap for convenient key-value access and DryRun flag.
type MutationContext[T any] struct {
	Mutation   string            // mutation operation name
	Statement  Statement         // full parsed statement (for positional args)
	Args       []Arg             // parsed arguments
	ArgMap     map[string]string // key=value args as map (convenience)
	Items      func() ([]T, error) // lazy item loader (for lookups/validation)
	DryRun     bool              // true when dry_run=true was passed
}

// PositionalArg returns the value of the first positional (keyless) argument,
// or empty string if none exists. Use for mutations where the first arg is an ID.
func (ctx MutationContext[T]) PositionalArg() string {
	for _, arg := range ctx.Args {
		if arg.Key == "" {
			return arg.Value
		}
	}
	return ""
}

// RequireArg returns the named argument value from ArgMap, or an error if missing/empty.
// For positional args, use PositionalArg() instead.
func (ctx MutationContext[T]) RequireArg(name string) (string, error) {
	if v, ok := ctx.ArgMap[name]; ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("required parameter %q is missing", name)
}

// ArgDefault returns the named argument value, or defaultValue if missing/empty.
func (ctx MutationContext[T]) ArgDefault(name, defaultValue string) string {
	if v, ok := ctx.ArgMap[name]; ok && v != "" {
		return v
	}
	return defaultValue
}

// MutationResult wraps a mutation's outcome for consistent response shape.
// Handlers return (any, error) — the framework wraps into MutationResult
// before serialization.
type MutationResult struct {
	Ok     bool            `json:"ok"`
	Result any             `json:"result,omitempty"`
	Errors []MutationError `json:"errors,omitempty"`
}

// MutationError describes a validation or domain error from a mutation.
// Field pinpoints which input argument caused the error (empty for general errors).
type MutationError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// MutationMetadata extends OperationMetadata with mutation-specific annotations.
// These are surfaced by schema() introspection and used by agents for
// safety decisions (confirm before destructive, skip confirm for idempotent).
type MutationMetadata struct {
	Description string         `json:"description,omitempty"`
	Parameters  []ParameterDef `json:"parameters,omitempty"`
	Examples    []string       `json:"examples,omitempty"`
	Destructive bool           `json:"destructive"`
	Idempotent  bool           `json:"idempotent"`
}
