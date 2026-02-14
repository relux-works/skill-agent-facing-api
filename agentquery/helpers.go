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

// Distinct returns unique values from items, extracted by keyFn, in first-seen order.
// Preserving first-seen order ensures deterministic output regardless of map iteration.
func Distinct[T any](items []T, keyFn func(T) string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		key := keyFn(item)
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}
	return result
}

// DistinctCount returns the count per unique value, extracted by keyFn.
// More efficient than GroupBy + len when only counts are needed
// (avoids allocating intermediate slices).
func DistinctCount[T any](items []T, keyFn func(T) string) map[string]int {
	counts := make(map[string]int)
	for _, item := range items {
		counts[keyFn(item)]++
	}
	return counts
}

// GroupBy groups items by a key function.
// Returns a map of key to items. Each group preserves the original item order.
func GroupBy[T any](items []T, keyFn func(T) string) map[string][]T {
	groups := make(map[string][]T)
	for _, item := range items {
		key := keyFn(item)
		groups[key] = append(groups[key], item)
	}
	return groups
}
