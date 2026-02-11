// Reference implementation: field selection and projection.
//
// The Selector controls which fields are included in JSON responses.
// Shared between DSL and any future MCP server — guarantees identical output.
//
// Adapt ValidFields, Presets, and Apply() to your domain model.

package fields

import "fmt"

// ValidFields — all recognized field names for your domain.
// ADAPT THIS to your element model.
var ValidFields = map[string]bool{
	"id":          true,
	"name":        true,
	"status":      true,
	"assignee":    true,
	"description": true,
	"parent":      true,
	"created":     true,
	"updated":     true,
	"tags":        true,
	"blockedBy":   true,
	"blocks":      true,
}

// Presets — named bundles of fields for common access patterns.
// ADAPT THIS to your use-cases.
var Presets = map[string][]string{
	"minimal":  {"id", "status"},
	"default":  {"id", "name", "status"},
	"overview": {"id", "name", "status", "assignee", "parent"},
	"full":     {"id", "name", "status", "assignee", "description", "parent", "created", "updated", "tags", "blockedBy", "blocks"},
}

// Selector controls which fields appear in the response.
type Selector struct {
	fields map[string]bool
	all    bool // optimization: skip per-field checks when "full" preset is used
}

// NewSelector creates a Selector from requested field names.
// Empty input defaults to {"id", "name", "status"}.
// Presets are expanded inline. Unknown fields return an error.
func NewSelector(requested []string) (*Selector, error) {
	if len(requested) == 0 {
		return &Selector{
			fields: map[string]bool{"id": true, "name": true, "status": true},
		}, nil
	}

	s := &Selector{fields: make(map[string]bool)}

	for _, f := range requested {
		if expanded, ok := Presets[f]; ok {
			if f == "full" {
				s.all = true
			}
			for _, ef := range expanded {
				s.fields[ef] = true
			}
			continue
		}
		if !ValidFields[f] {
			return nil, fmt.Errorf("unknown field: %s", f)
		}
		s.fields[f] = true
	}

	return s, nil
}

// Include returns true if the field should be in the response.
func (s *Selector) Include(field string) bool {
	if s.all {
		return true
	}
	return s.fields[field]
}

// Apply builds a response map for an element, including only selected fields.
// ADAPT THIS to your domain model.
//
// Pattern: one `if s.Include("field")` block per field.
// Lazy evaluation — expensive fields (children, notes, etc.) are only
// computed when requested.
//
//   func (s *Selector) Apply(elem *YourElement) map[string]interface{} {
//       result := make(map[string]interface{})
//       if s.Include("id") {
//           result["id"] = elem.ID
//       }
//       if s.Include("name") {
//           result["name"] = elem.Name
//       }
//       if s.Include("status") {
//           result["status"] = elem.Status
//       }
//       // ... more fields
//       if s.Include("children") {
//           // expensive: only computed when requested
//           result["children"] = loadChildren(elem)
//       }
//       return result
//   }
