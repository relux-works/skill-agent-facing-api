package agentquery

import "fmt"

// Schema is the central type for defining a query-able domain.
// It is generic on T, the domain item type (e.g. *board.Element).
// All fields, presets, operations, and defaults are registered on the Schema
// before the first query.
type Schema[T any] struct {
	fields        map[string]FieldAccessor[T]    // registered field accessors
	fieldOrder    []string                       // field names in registration order
	presets       map[string][]string            // named field bundles
	defaultFields []string                       // fields used when no projection specified
	operations    map[string]OperationHandler[T] // registered operation handlers
	loader        func() ([]T, error)            // lazy data loader
	dataDir       string                         // root data directory for search
	extensions    []string                       // file extensions for search
}

// schemaConfig holds configuration set via functional options.
type schemaConfig struct {
	dataDir    string
	extensions []string
}

// Option configures a Schema during construction.
type Option func(*schemaConfig)

// WithDataDir sets the root data directory used by Search.
func WithDataDir(dir string) Option {
	return func(c *schemaConfig) {
		c.dataDir = dir
	}
}

// WithExtensions sets the file extensions used by Search.
// Each extension should include the leading dot (e.g. ".md").
func WithExtensions(exts ...string) Option {
	return func(c *schemaConfig) {
		c.extensions = exts
	}
}

// NewSchema creates a new empty Schema for the given domain type.
// Options configure search-related settings (data directory, file extensions).
// Default extensions: [".md"].
func NewSchema[T any](opts ...Option) *Schema[T] {
	cfg := &schemaConfig{
		extensions: []string{".md"},
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &Schema[T]{
		fields:     make(map[string]FieldAccessor[T]),
		presets:    make(map[string][]string),
		operations: make(map[string]OperationHandler[T]),
		dataDir:    cfg.dataDir,
		extensions: cfg.extensions,
	}
}

// Field registers a named field with its accessor function.
// The accessor extracts the field value from a domain item.
func (s *Schema[T]) Field(name string, accessor FieldAccessor[T]) {
	if _, exists := s.fields[name]; !exists {
		s.fieldOrder = append(s.fieldOrder, name)
	}
	s.fields[name] = accessor
}

// Preset registers a named bundle of fields.
// When a preset name appears in a field projection, it expands to the listed fields.
func (s *Schema[T]) Preset(name string, fields ...string) {
	s.presets[name] = fields
}

// DefaultFields sets the field set used when no projection is specified in a query.
func (s *Schema[T]) DefaultFields(fields ...string) {
	s.defaultFields = fields
}

// Operation registers a named operation with its handler function.
// The handler is called when the operation name appears in a query.
func (s *Schema[T]) Operation(name string, handler OperationHandler[T]) {
	s.operations[name] = handler
}

// SetLoader sets the function used to load domain items for query execution.
// The loader is called lazily — only when an operation handler accesses ctx.Items().
func (s *Schema[T]) SetLoader(fn func() ([]T, error)) {
	s.loader = fn
}

// ResolveField implements the FieldResolver interface.
// If name matches a preset, it returns the expanded field list.
// If name matches a registered field, it returns []string{name}.
// Otherwise it returns an error for unknown fields.
func (s *Schema[T]) ResolveField(name string) ([]string, error) {
	if expanded, ok := s.presets[name]; ok {
		return expanded, nil
	}
	if _, ok := s.fields[name]; ok {
		return []string{name}, nil
	}
	return nil, fmt.Errorf("unknown field: %s", name)
}

// Search performs a recursive full-text regex search within the schema's data directory.
// It delegates to the package-level Search function, passing the schema's dataDir and extensions.
func (s *Schema[T]) Search(pattern string, opts SearchOptions) ([]SearchResult, error) {
	return Search(s.dataDir, pattern, s.extensions, opts)
}

// SearchJSON performs a search and returns the results as indented JSON bytes.
// It delegates to the package-level SearchJSON function.
func (s *Schema[T]) SearchJSON(pattern string, opts SearchOptions) ([]byte, error) {
	return SearchJSON(s.dataDir, pattern, s.extensions, opts)
}

// parserConfig builds a ParserConfig from the schema's registered operations and fields.
func (s *Schema[T]) parserConfig() *ParserConfig {
	ops := make(map[string]bool, len(s.operations))
	for name := range s.operations {
		ops[name] = true
	}
	return &ParserConfig{
		Operations:    ops,
		FieldResolver: s,
	}
}
