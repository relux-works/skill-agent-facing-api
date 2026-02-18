package agentquery

import "fmt"

// Error code constants for categorizing errors.
const (
	ErrParse      = "PARSE_ERROR"
	ErrNotFound   = "NOT_FOUND"
	ErrValidation = "VALIDATION_ERROR"
	ErrInternal   = "INTERNAL_ERROR"

	// Mutation-specific error codes.
	ErrConflict     = "CONFLICT"           // duplicate key, unique constraint violation
	ErrForbidden    = "FORBIDDEN"          // authorization failure
	ErrPrecondition = "PRECONDITION_FAILED" // optimistic concurrency check failed
	ErrRequired     = "REQUIRED"           // required parameter missing
	ErrInvalidValue = "INVALID_VALUE"      // enum/type mismatch
)

// ParseError represents a syntax or semantic error found during parsing.
type ParseError struct {
	Message  string `json:"message"`
	Pos      Pos    `json:"pos"`
	Got      string `json:"got,omitempty"`
	Expected string `json:"expected,omitempty"`
}

// Error implements the error interface for ParseError.
func (e *ParseError) Error() string {
	if e.Got != "" && e.Expected != "" {
		return fmt.Sprintf("parse error at %d:%d: %s (got %q, expected %s)",
			e.Pos.Line, e.Pos.Column, e.Message, e.Got, e.Expected)
	}
	if e.Got != "" {
		return fmt.Sprintf("parse error at %d:%d: %s (got %q)",
			e.Pos.Line, e.Pos.Column, e.Message, e.Got)
	}
	return fmt.Sprintf("parse error at %d:%d: %s", e.Pos.Line, e.Pos.Column, e.Message)
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
