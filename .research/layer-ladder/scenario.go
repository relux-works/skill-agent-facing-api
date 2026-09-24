// Package layerladder builds one task scenario's response in several
// representation layers so that the token cost of each layer can be measured
// against identical underlying data.
//
// The input data is the existing checked-in synthetic fixture set in
// ../synthetic-payloads (json-5, json-20, json-100, json-500). Reusing it keeps
// this measurement comparable with the 2026-02-12 alias study and avoids
// introducing a second generator seed.
package layerladder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
)

// Task mirrors the eight fields carried by the checked-in synthetic fixtures.
type Task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Assignee    string `json:"assignee"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Created     string `json:"created"`
	Updated     string `json:"updated"`
}

// FullFields is the complete record contract, in fixture order.
var FullFields = []string{"id", "name", "status", "assignee", "description", "priority", "created", "updated"}

// ScenarioFields is what the measured scenario actually needs.
//
// Scenario: an agent assembles a sprint stand-up roll-call. For every task in
// the sprint page it needs the identifier, the current status, who holds it and
// how urgent it is. It does not need the name, the free-text description or the
// two timestamps. The row set is identical in every layer; only the per-record
// field set and the serialization change.
var ScenarioFields = []string{"id", "status", "assignee", "priority"}

// ScenarioQuery is the DSL text executed through the production query path.
const ScenarioQuery = "list() { id status assignee priority }"

// FullQuery is the same row set with no projection applied.
const FullQuery = "list() { full }"

// Scales are the fixture sizes reused from the synthetic-payloads study.
var Scales = []int{5, 20, 100, 500}

// LoadTasks reads one checked-in synthetic fixture and decodes it into records.
func LoadTasks(payloadDir string, scale int) ([]Task, error) {
	path := filepath.Join(payloadDir, fmt.Sprintf("json-%d.txt", scale))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load fixture %s: %w", path, err)
	}
	var tasks []Task
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	if len(tasks) != scale {
		return nil, fmt.Errorf("fixture %s: expected %d records, got %d", path, scale, len(tasks))
	}
	return tasks, nil
}

// NewSchema registers the same field/preset/operation contract the example CLI
// uses, so every layer below is produced by the real agentquery entry points
// rather than by a measurement-only re-implementation.
func NewSchema(tasks []Task) *agentquery.Schema[Task] {
	schema := agentquery.NewSchema[Task]()

	schema.Field("id", func(t Task) any { return t.ID })
	schema.Field("name", func(t Task) any { return t.Name })
	schema.Field("status", func(t Task) any { return t.Status })
	schema.Field("assignee", func(t Task) any { return t.Assignee })
	schema.Field("description", func(t Task) any { return t.Description })
	schema.Field("priority", func(t Task) any { return t.Priority })
	schema.Field("created", func(t Task) any { return t.Created })
	schema.Field("updated", func(t Task) any { return t.Updated })

	schema.Preset("minimal", "id", "status")
	schema.Preset("standup", "id", "status", "assignee", "priority")
	schema.Preset("full", FullFields...)
	schema.DefaultFields("minimal")

	agentquery.FilterableField(schema, "status", func(t Task) string { return t.Status })
	agentquery.FilterableField(schema, "assignee", func(t Task) string { return t.Assignee })
	agentquery.FilterableField(schema, "priority", func(t Task) string { return t.Priority })

	snapshot := make([]Task, len(tasks))
	copy(snapshot, tasks)
	schema.SetLoader(func() ([]Task, error) {
		out := make([]Task, len(snapshot))
		copy(out, snapshot)
		return out, nil
	})

	schema.OperationWithMetadata("list", func(ctx agentquery.OperationContext[Task]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		filtered := agentquery.FilterItems(items, ctx.Predicate)
		page, err := agentquery.PaginateSlice(filtered, ctx.Statement.Args)
		if err != nil {
			return nil, err
		}
		results := make([]map[string]any, 0, len(page))
		for _, task := range page {
			results = append(results, ctx.Selector.Apply(task))
		}
		return results, nil
	}, agentquery.OperationMetadata{
		Description: "List sprint tasks with optional filters and pagination",
		Parameters: []agentquery.ParameterDef{
			{Name: "status", Type: "string", Optional: true, Description: "Filter by status"},
			{Name: "assignee", Type: "string", Optional: true, Description: "Filter by assignee"},
			{Name: "priority", Type: "string", Optional: true, Description: "Filter by priority"},
			{Name: "skip", Type: "int", Optional: true, Default: 0, Description: "Skip first N items"},
			{Name: "take", Type: "int", Optional: true, Description: "Return at most N items"},
		},
		Examples: []string{ScenarioQuery, FullQuery, "list(status=blocked) { standup }"},
	})

	return schema
}

// CanonicalRecords projects every task down to the scenario fields. This is the
// reference the preservation gate compares every rendered layer against.
func CanonicalRecords(tasks []Task) []map[string]string {
	out := make([]map[string]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, map[string]string{
			"id":       t.ID,
			"status":   t.Status,
			"assignee": t.Assignee,
			"priority": t.Priority,
		})
	}
	return out
}

// RecordsDigestInput renders records in a canonical, order-stable form so a
// digest over it is stable across layers and runs.
func RecordsDigestInput(records []map[string]string) []byte {
	rows := make([]map[string]string, len(records))
	copy(rows, records)
	keys := make([]string, 0, len(ScenarioFields))
	keys = append(keys, ScenarioFields...)
	sort.Strings(keys)

	normalized := make([][]string, 0, len(rows))
	for _, r := range rows {
		row := make([]string, 0, len(keys))
		for _, k := range keys {
			row = append(row, r[k])
		}
		normalized = append(normalized, row)
	}
	raw, _ := json.Marshal(normalized)
	return raw
}
