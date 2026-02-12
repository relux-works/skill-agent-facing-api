package agentquery

import (
	"fmt"
	"strconv"
)

// PaginateSlice extracts skip/take integer params from the statement args
// and applies offset/limit slicing to the items slice.
//
// Conventions:
//   - skip defaults to 0 (no offset)
//   - take defaults to 0 (meaning: no limit, return all remaining items)
//   - skip must be >= 0
//   - take must be > 0 (when specified)
//   - If skip >= len(items), returns an empty slice
//   - If skip+take > len(items), returns items[skip:]
//
// Usage in operation handlers:
//
//	items, err := ctx.Items()
//	// ... filter items ...
//	items, err = agentquery.PaginateSlice(items, ctx.Statement.Args)
func PaginateSlice[T any](items []T, args []Arg) ([]T, error) {
	skip, take, err := ParseSkipTake(args)
	if err != nil {
		return nil, err
	}
	return applySkipTake(items, skip, take), nil
}

// ParseSkipTake extracts skip and take values from a list of Args.
// Returns (skip, take, error). skip defaults to 0, take defaults to 0 (no limit).
// Validates: skip >= 0, take > 0 (when specified).
func ParseSkipTake(args []Arg) (skip, take int, err error) {
	for _, arg := range args {
		switch arg.Key {
		case "skip":
			skip, err = strconv.Atoi(arg.Value)
			if err != nil {
				return 0, 0, &Error{
					Code:    ErrValidation,
					Message: fmt.Sprintf("skip must be an integer, got %q", arg.Value),
					Details: map[string]any{"param": "skip", "value": arg.Value},
				}
			}
			if skip < 0 {
				return 0, 0, &Error{
					Code:    ErrValidation,
					Message: fmt.Sprintf("skip must be >= 0, got %d", skip),
					Details: map[string]any{"param": "skip", "value": skip},
				}
			}
		case "take":
			take, err = strconv.Atoi(arg.Value)
			if err != nil {
				return 0, 0, &Error{
					Code:    ErrValidation,
					Message: fmt.Sprintf("take must be an integer, got %q", arg.Value),
					Details: map[string]any{"param": "take", "value": arg.Value},
				}
			}
			if take <= 0 {
				return 0, 0, &Error{
					Code:    ErrValidation,
					Message: fmt.Sprintf("take must be > 0, got %d", take),
					Details: map[string]any{"param": "take", "value": take},
				}
			}
		}
	}
	return skip, take, nil
}

// applySkipTake performs the actual slicing. skip and take are assumed valid.
// take=0 means no limit.
func applySkipTake[T any](items []T, skip, take int) []T {
	if skip >= len(items) {
		return []T{}
	}
	items = items[skip:]
	if take > 0 && take < len(items) {
		items = items[:take]
	}
	return items
}
