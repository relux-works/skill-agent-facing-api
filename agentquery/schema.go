package agentquery

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Schema is the central type for defining a query-able domain.
// It is generic on T, the domain item type (e.g. *board.Element).
// All fields, presets, operations, and defaults are registered on the Schema
// before the first query.
type Schema[T any] struct {
	fields         map[string]FieldAccessor[T]    // registered field accessors
	fieldOrder     []string                       // field names in registration order
	presets        map[string][]string            // named field bundles
	defaultFields  []string                       // fields used when no projection specified
	operations     map[string]OperationHandler[T] // registered operation handlers
	loader         func() ([]T, error)            // lazy data loader
	searchProvider SearchProvider                 // pluggable search backend
	outputMode     OutputMode                     // HumanReadable (default) or LLMReadable
}

// schemaConfig holds configuration set via functional options.
type schemaConfig struct {
	dataDir        string
	extensions     []string
	searchProvider SearchProvider
	outputMode     OutputMode
}

// Option configures a Schema during construction.
type Option func(*schemaConfig)

// WithDataDir sets the root data directory used by Search.
// If no SearchProvider is explicitly set, a FileSystemSearchProvider is created
// using this directory and the configured extensions.
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

// WithSearchProvider sets an explicit SearchProvider, overriding any
// dataDir/extensions configuration. Use this to plug in non-filesystem
// search backends.
func WithSearchProvider(sp SearchProvider) Option {
	return func(c *schemaConfig) {
		c.searchProvider = sp
	}
}

// WithOutputMode sets the serialization mode for query and search results.
// HumanReadable (default) produces standard JSON.
// LLMReadable produces compact tabular output optimized for LLM token efficiency.
func WithOutputMode(mode OutputMode) Option {
	return func(c *schemaConfig) {
		c.outputMode = mode
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

	// Resolve search provider: explicit provider wins, otherwise auto-create
	// a FileSystemSearchProvider from dataDir/extensions if dataDir is set.
	sp := cfg.searchProvider
	if sp == nil && cfg.dataDir != "" {
		sp = &FileSystemSearchProvider{
			DataDir:    cfg.dataDir,
			Extensions: cfg.extensions,
		}
	}

	s := &Schema[T]{
		fields:         make(map[string]FieldAccessor[T]),
		presets:        make(map[string][]string),
		operations:     make(map[string]OperationHandler[T]),
		searchProvider: sp,
		outputMode:     cfg.outputMode,
	}

	// Register built-in "schema" introspection operation.
	// The handler is a closure over s — it reads operations/fields/presets/defaults
	// at execution time, so all user-registered entries are visible.
	s.operations["schema"] = func(ctx OperationContext[T]) (any, error) {
		return s.introspect(), nil
	}

	return s
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

// OutputMode returns the schema's configured output mode.
func (s *Schema[T]) OutputMode() OutputMode {
	return s.outputMode
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

// Search performs a full-text regex search using the schema's SearchProvider.
// Returns an error if no search provider is configured.
func (s *Schema[T]) Search(pattern string, opts SearchOptions) ([]SearchResult, error) {
	if s.searchProvider == nil {
		return nil, &Error{Code: ErrInternal, Message: "no search provider configured"}
	}
	return s.searchProvider.Search(pattern, opts)
}

// SearchJSON performs a search and returns the results serialized according to
// the schema's configured output mode. HumanReadable produces indented JSON;
// LLMReadable produces compact grouped-by-file text.
func (s *Schema[T]) SearchJSON(pattern string, opts SearchOptions) ([]byte, error) {
	return s.SearchJSONWithMode(pattern, opts, s.outputMode)
}

// SearchJSONWithMode performs a search and serializes with the specified output mode,
// overriding the schema's default mode.
func (s *Schema[T]) SearchJSONWithMode(pattern string, opts SearchOptions, mode OutputMode) ([]byte, error) {
	results, err := s.Search(pattern, opts)
	if err != nil {
		return nil, err
	}
	if mode == LLMReadable {
		return FormatSearchCompact(results), nil
	}
	return json.MarshalIndent(results, "", "  ")
}

// QueryJSONWithMode executes a query and serializes with the specified output mode,
// overriding the schema's default mode.
func (s *Schema[T]) QueryJSONWithMode(input string, mode OutputMode) ([]byte, error) {
	result, err := s.Query(input)
	if err != nil {
		return nil, err
	}
	if mode == LLMReadable {
		return s.formatLLMReadable(input, result)
	}
	return json.Marshal(result)
}

// introspect returns the full schema contract as a JSON-serializable map.
// It lists all operations (sorted), fields (in registration order),
// presets (with expanded field lists), and default fields.
func (s *Schema[T]) introspect() map[string]any {
	// Collect operation names and sort them for deterministic output.
	ops := make([]string, 0, len(s.operations))
	for name := range s.operations {
		ops = append(ops, name)
	}
	sort.Strings(ops)

	// Fields in registration order.
	fields := make([]string, len(s.fieldOrder))
	copy(fields, s.fieldOrder)

	// Presets: map name -> expanded field list (copy each slice).
	presets := make(map[string][]string, len(s.presets))
	for name, pf := range s.presets {
		cp := make([]string, len(pf))
		copy(cp, pf)
		presets[name] = cp
	}

	// Default fields (copy).
	defaults := make([]string, len(s.defaultFields))
	copy(defaults, s.defaultFields)

	return map[string]any{
		"operations":    ops,
		"fields":        fields,
		"presets":       presets,
		"defaultFields": defaults,
	}
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
