package agentquery

import (
	"fmt"
	"sort"
	"strings"
)

// Error code constants for categorizing errors.
const (
	ErrParse      = "PARSE_ERROR"
	ErrNotFound   = "NOT_FOUND"
	ErrValidation = "VALIDATION_ERROR"
	ErrInternal   = "INTERNAL_ERROR"

	// Mutation-specific error codes.
	ErrConflict           = "CONFLICT"            // duplicate key, unique constraint violation
	ErrForbidden          = "FORBIDDEN"           // authorization failure
	ErrPrecondition       = "PRECONDITION_FAILED" // optimistic concurrency check failed
	ErrRequired           = "REQUIRED"            // required parameter missing
	ErrInvalidValue       = "INVALID_VALUE"       // enum/type mismatch
	ErrUnknownArgument    = "UNKNOWN_ARGUMENT"    // named argument is absent from mutation metadata
	ErrUnexpectedArgument = "UNEXPECTED_ARGUMENT" // positional argument cannot map to mutation metadata
)

// ParseError represents a syntax or semantic error found during parsing.
type ParseError struct {
	Message  string `json:"message"`
	Pos      Pos    `json:"pos"`
	Got      string `json:"got,omitempty"`
	Expected string `json:"expected,omitempty"`

	// Operation names the statement being parsed when the error arose, and
	// OperationPos where that statement starts. They are set for every error
	// raised inside a statement body so a host that parsed permissively can
	// still attribute an argument or projection failure to its operation.
	Operation    string `json:"operation,omitempty"`
	OperationPos *Pos   `json:"operationPos,omitempty"`

	// KnownOperations lists the operation names the schema accepts, sorted.
	// Set on unknown-operation errors so a host can render the recovery path
	// as data, not only as prose.
	KnownOperations []string `json:"knownOperations,omitempty"`

	// Hint is the recovery pointer rendered after the message, for example
	// the schema() introspection call that lists the accepted contract.
	Hint string `json:"hint,omitempty"`
}

// UnknownOperationHint is the recovery pointer attached to every
// unknown-operation error: the introspection calls that expose the contract.
const UnknownOperationHint = "see schema() for the contract and schema(operation=NAME) for one signature"

// NewUnknownOperationError builds the canonical unknown-operation error: the
// name is reported as unknown, the sorted known names are carried as data and
// the schema() pointer is attached. Hosts that parse permissively and only
// later select a schema use it so their verdict reads the same as the parser's.
func NewUnknownOperationError(name string, pos Pos, known []string) *ParseError {
	sorted := make([]string, len(known))
	copy(sorted, known)
	sort.Strings(sorted)
	return &ParseError{
		Message:         fmt.Sprintf("unknown operation %q", name),
		Pos:             pos,
		Got:             name,
		Operation:       name,
		OperationPos:    &Pos{Offset: pos.Offset, Line: pos.Line, Column: pos.Column},
		KnownOperations: sorted,
		Hint:            UnknownOperationHint,
	}
}

// IsUnknownOperation reports whether the error is an unknown-operation verdict.
func (e *ParseError) IsUnknownOperation() bool {
	return e != nil && e.KnownOperations != nil && strings.HasPrefix(e.Message, "unknown operation ")
}

// Error implements the error interface for ParseError.
func (e *ParseError) Error() string {
	var base string
	switch {
	case e.Got != "" && e.Expected != "":
		base = fmt.Sprintf("parse error at %d:%d: %s (got %q, expected %s)",
			e.Pos.Line, e.Pos.Column, e.Message, e.Got, e.Expected)
	case e.Got != "":
		base = fmt.Sprintf("parse error at %d:%d: %s (got %q)",
			e.Pos.Line, e.Pos.Column, e.Message, e.Got)
	default:
		base = fmt.Sprintf("parse error at %d:%d: %s", e.Pos.Line, e.Pos.Column, e.Message)
	}
	if len(e.KnownOperations) > 0 {
		base += "; known operations: " + strings.Join(e.KnownOperations, ", ")
	}
	if e.Hint != "" {
		base += "; " + e.Hint
	}
	return base
}

// Error represents a structured error with a code, message, and optional details.
// It is JSON-serializable for use in API responses.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}
