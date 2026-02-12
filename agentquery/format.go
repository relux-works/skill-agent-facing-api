package agentquery

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatCompact formats a query result in compact tabular format.
// fieldOrder provides column ordering for tabular output.
//
// For list results ([]map[string]any or []any containing maps):
// header row with comma-separated field names, then one row per item.
//
// For single object results (map[string]any with no "error" key):
// key:value pairs, one per line.
//
// For other results (summary, schema introspection, errors, non-map types):
// falls back to JSON marshaling.
func FormatCompact(result any, fieldOrder []string) ([]byte, error) {
	switch v := result.(type) {
	case []map[string]any:
		return formatList(v, fieldOrder)
	case []any:
		return formatAnyList(v, fieldOrder)
	case map[string]any:
		// Error maps fall through to JSON.
		if _, hasError := v["error"]; hasError {
			return json.Marshal(v)
		}
		return formatSingle(v, fieldOrder)
	default:
		return json.Marshal(result)
	}
}

// formatList formats a slice of maps as a CSV-style table.
func formatList(items []map[string]any, fieldOrder []string) ([]byte, error) {
	if len(fieldOrder) == 0 && len(items) > 0 {
		// No explicit field order — derive from first item's keys.
		// This is a fallback; normally fieldOrder is provided by the query.
		fieldOrder = mapKeys(items[0])
	}

	var b strings.Builder

	// Header row
	b.WriteString(strings.Join(fieldOrder, ","))
	b.WriteByte('\n')

	// Data rows
	for _, item := range items {
		for i, field := range fieldOrder {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(escapeCSV(item[field]))
		}
		b.WriteByte('\n')
	}

	return []byte(b.String()), nil
}

// formatAnyList tries to interpret []any as a list of maps for tabular output.
// If any element is not a map, falls back to JSON.
func formatAnyList(items []any, fieldOrder []string) ([]byte, error) {
	maps := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			// Mixed types — fall back to JSON.
			return json.Marshal(items)
		}
		maps = append(maps, m)
	}
	return formatList(maps, fieldOrder)
}

// formatSingle formats a single map as key:value pairs, one per line.
func formatSingle(m map[string]any, fieldOrder []string) ([]byte, error) {
	if len(fieldOrder) == 0 || !hasOverlap(fieldOrder, m) {
		fieldOrder = mapKeys(m)
	}

	var b strings.Builder
	for _, field := range fieldOrder {
		val := m[field]
		b.WriteString(field)
		b.WriteByte(':')
		b.WriteString(escapeKV(val))
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// escapeCSV converts a value to a string suitable for a CSV cell.
// Nil values become empty string. Values containing commas, quotes,
// or newlines are wrapped in double quotes with internal quotes doubled.
func escapeCSV(val any) string {
	if val == nil {
		return ""
	}
	s := formatValue(val)
	if strings.ContainsAny(s, ",\"\n\r") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// escapeKV converts a value to a string suitable for the value part of key:value format.
// Nil values become empty string. Values containing newlines are escaped.
func escapeKV(val any) string {
	if val == nil {
		return ""
	}
	s := formatValue(val)
	// Escape embedded newlines so each key:value stays on one logical line.
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// formatValue converts any value to its string representation.
// Slices and maps are JSON-encoded for readability. Scalars use fmt.Sprintf.
func formatValue(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		// For slices, maps, and other complex types, JSON is more readable
		// than fmt.Sprintf's Go-syntax output (e.g. [go rust] vs ["go","rust"]).
		if isComplex(val) {
			b, err := json.Marshal(v)
			if err == nil {
				return string(b)
			}
		}
		return fmt.Sprintf("%v", val)
	}
}

// isComplex returns true for types that benefit from JSON encoding
// rather than fmt.Sprintf (slices, arrays, maps).
func isComplex(val any) bool {
	switch val.(type) {
	case []string, []int, []float64, []any, []map[string]any:
		return true
	case map[string]any, map[string]string:
		return true
	}
	// For other slice/map types, check with a broader approach.
	s := fmt.Sprintf("%T", val)
	return strings.HasPrefix(s, "[]") || strings.HasPrefix(s, "map[")
}

// hasOverlap returns true if at least one field in fieldOrder exists as a key in the map.
// Used to detect when the provided field order doesn't match the result's actual structure.
func hasOverlap(fieldOrder []string, m map[string]any) bool {
	for _, f := range fieldOrder {
		if _, ok := m[f]; ok {
			return true
		}
	}
	return false
}

// mapKeys returns the keys of a map in sorted order for deterministic output.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sort for deterministic output when no explicit order is given.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
