package agentquery

// FilterItems returns only items for which the predicate returns true.
// This is a convenience for operation handlers that need to apply filters
// (e.g. list, count) without duplicating the loop boilerplate.
func FilterItems[T any](items []T, pred func(T) bool) []T {
	var result []T
	for _, item := range items {
		if pred(item) {
			result = append(result, item)
		}
	}
	return result
}

// CountItems returns the number of items for which the predicate returns true.
// More efficient than len(FilterItems(...)) when you only need the count.
func CountItems[T any](items []T, pred func(T) bool) int {
	n := 0
	for _, item := range items {
		if pred(item) {
			n++
		}
	}
	return n
}

// MatchAll returns a predicate that always returns true.
// Useful as a default when no filters are specified.
func MatchAll[T any]() func(T) bool {
	return func(T) bool { return true }
}
