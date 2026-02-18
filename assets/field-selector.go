// Reference implementation: field selection and projection.
//
// The Selector controls which fields are included in JSON responses.
// Shared between DSL and any future MCP server — guarantees identical output.
//
// Adapt ValidFields, Presets, and Apply() to your domain model.
//
// When using the agentquery library, register operations with metadata
// for schema introspection:
//
//   // ADAPT THIS: register operations with OperationWithMetadata
//   schema.OperationWithMetadata("list", opList, agentquery.OperationMetadata{
//       Description: "List items with optional filters and pagination",
//       Parameters: []agentquery.ParameterDef{
//           {Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
//           {Name: "skip", Type: "int", Optional: true, Default: 0, Description: "Skip first N items"},
//           {Name: "take", Type: "int", Optional: true, Description: "Return at most N items"},
//       },
//       Examples: []string{"list() { overview }", "list(status=done, skip=0, take=5) { minimal }"},
//   })
//
//   schema.OperationWithMetadata("count", opCount, agentquery.OperationMetadata{
//       Description: "Count items matching optional filters",
//       Parameters: []agentquery.ParameterDef{
//           {Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
//       },
//       Examples: []string{"count()", "count(status=done)"},
//   })

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

// --- Mutation registration patterns using agentquery ---
//
// ADAPT THIS: register mutations with MutationWithMetadata
//
//   schema.MutationWithMetadata("create", mutCreate, agentquery.MutationMetadata{
//       Description: "Create a new item",
//       Parameters: []agentquery.ParameterDef{
//           {Name: "title", Type: "string", Required: true, Description: "Item title"},
//           {Name: "status", Type: "string", Enum: []string{"todo", "in-progress", "done"}, Default: "todo"},
//       },
//       Destructive: false,
//       Idempotent:  false,
//       Examples:    []string{`create(title="Fix bug")`},
//   })
//
//   schema.MutationWithMetadata("delete", mutDelete, agentquery.MutationMetadata{
//       Description: "Delete an item by ID",
//       Parameters: []agentquery.ParameterDef{
//           {Name: "id", Type: "string", Required: true, Description: "Item ID (positional)"},
//       },
//       Destructive: true,
//       Idempotent:  true,
//       Examples:    []string{`delete(ITEM-42)`},
//   })
//
// ADAPT THIS: mutation handler pattern
//
//   func mutCreate(ctx agentquery.MutationContext[Item]) (any, error) {
//       title := ctx.ArgMap["title"]
//       if title == "" {
//           return nil, &agentquery.Error{Code: agentquery.ErrValidation, Message: "title is required"}
//       }
//       if ctx.DryRun {
//           return map[string]any{"dry_run": true, "would_create": map[string]any{"title": title}}, nil
//       }
//       item := createItem(title, ctx.ArgMap["status"])
//       return map[string]any{"id": item.ID, "title": item.Name}, nil
//   }

// --- Operation handler patterns using agentquery helpers ---
//
// ADAPT THIS: list operation with filtering + skip/take pagination
//
//   func opList(ctx agentquery.OperationContext[Item]) (any, error) {
//       items, err := ctx.Items()
//       if err != nil {
//           return nil, err
//       }
//       filtered := agentquery.FilterItems(items, filterFromArgs(ctx.Statement.Args))
//       page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
//       if err != nil {
//           return nil, err
//       }
//       results := make([]map[string]any, 0, len(page))
//       for _, item := range page {
//           results = append(results, ctx.Selector.Apply(item))
//       }
//       return results, nil
//   }
//
// ADAPT THIS: count operation with filtering
//
//   func opCount(ctx agentquery.OperationContext[Item]) (any, error) {
//       items, err := ctx.Items()
//       if err != nil {
//           return nil, err
//       }
//       n := agentquery.CountItems(items, filterFromArgs(ctx.Statement.Args))
//       return map[string]any{"count": n}, nil
//   }
//
// ADAPT THIS: shared filter builder from keyword args
//
//   func filterFromArgs(args []agentquery.Arg) func(Item) bool {
//       var filterStatus string
//       for _, arg := range args {
//           switch arg.Key {
//           case "status":
//               filterStatus = arg.Value
//           }
//       }
//       if filterStatus == "" {
//           return agentquery.MatchAll[Item]()
//       }
//       return func(item Item) bool {
//           return strings.EqualFold(item.Status, filterStatus)
//       }
//   }
