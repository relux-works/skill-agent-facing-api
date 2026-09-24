package layerladder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
)

// LayerID identifies one representation layer of the same scenario response.
type LayerID string

const (
	// L0 — all eight fields, pretty JSON. REST-style "give me the record" baseline.
	L0JSONPrettyFull LayerID = "L0"
	// L1 — all eight fields, minified JSON. What agentquery's own "json" mode emits.
	L1JSONMinFull LayerID = "L1"
	// L2 — scenario fields only, pretty JSON.
	L2JSONPrettyProjected LayerID = "L2"
	// L3 — scenario fields only, minified JSON. Production `--format json`.
	L3JSONMinProjected LayerID = "L3"
	// L4 — scenario fields only, header+value compact. Production `--format compact`.
	L4CompactProjected LayerID = "L4"
	// L5 — L4 with one-character header aliases. Research overlay, not production.
	L5CompactAliasProjected LayerID = "L5"
)

// AliasHeader is the research-only one-character header used by L5. agentquery
// does not implement aliases; the 2026-02-12 study recommended against them.
var AliasHeader = map[string]string{
	"id":       "i",
	"status":   "s",
	"assignee": "a",
	"priority": "p",
}

// Layer describes one rung of the ladder.
type Layer struct {
	ID             LayerID `json:"id"`
	Name           string  `json:"name"`
	Fields         string  `json:"fields"`
	ProductionPath string  `json:"production_path"`
	EvidenceClass  string  `json:"evidence_class"`
}

// Layers is the ordered ladder contract. EvidenceClass distinguishes output a
// shipped agentquery code path actually produces from output this measurement
// constructs for comparison.
var Layers = []Layer{
	{L0JSONPrettyFull, "json-pretty-full", "full", "encoding/json.MarshalIndent over Schema.QueryAST result", "measurement-constructed baseline"},
	{L1JSONMinFull, "json-min-full", "full", "Schema.QueryJSONWithMode(HumanReadable)", "production output"},
	{L2JSONPrettyProjected, "json-pretty-projected", "scenario", "encoding/json.MarshalIndent over Schema.QueryAST result", "measurement-constructed baseline"},
	{L3JSONMinProjected, "json-min-projected", "scenario", "Schema.QueryJSONWithMode(HumanReadable)", "production output"},
	{L4CompactProjected, "compact-projected", "scenario", "Schema.QueryJSONWithMode(LLMReadable) -> FormatCompact", "production output"},
	{L5CompactAliasProjected, "compact-alias-projected", "scenario", "ApplyAliasHeader over L4 (no production equivalent)", "research overlay"},
}

// LayerByID returns the ladder entry for id.
func LayerByID(id LayerID) (Layer, bool) {
	for _, l := range Layers {
		if l.ID == id {
			return l, true
		}
	}
	return Layer{}, false
}

// Render produces every layer for one scale from one schema instance.
func Render(schema *agentquery.Schema[Task]) (map[LayerID]string, error) {
	out := make(map[LayerID]string, len(Layers))

	prettyFull, err := renderPretty(schema, FullQuery)
	if err != nil {
		return nil, err
	}
	out[L0JSONPrettyFull] = prettyFull

	minFull, err := schema.QueryJSONWithMode(FullQuery, agentquery.HumanReadable)
	if err != nil {
		return nil, fmt.Errorf("render L1: %w", err)
	}
	out[L1JSONMinFull] = string(minFull)

	prettyProjected, err := renderPretty(schema, ScenarioQuery)
	if err != nil {
		return nil, err
	}
	out[L2JSONPrettyProjected] = prettyProjected

	minProjected, err := schema.QueryJSONWithMode(ScenarioQuery, agentquery.HumanReadable)
	if err != nil {
		return nil, fmt.Errorf("render L3: %w", err)
	}
	out[L3JSONMinProjected] = string(minProjected)

	compact, err := schema.QueryJSONWithMode(ScenarioQuery, agentquery.LLMReadable)
	if err != nil {
		return nil, fmt.Errorf("render L4: %w", err)
	}
	out[L4CompactProjected] = string(compact)

	alias, err := ApplyAliasHeader(string(compact), AliasHeader)
	if err != nil {
		return nil, fmt.Errorf("render L5: %w", err)
	}
	out[L5CompactAliasProjected] = alias

	return out, nil
}

func renderPretty(schema *agentquery.Schema[Task], query string) (string, error) {
	result, err := schema.Query(query)
	if err != nil {
		return "", fmt.Errorf("execute %q: %w", query, err)
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal %q: %w", query, err)
	}
	return string(raw), nil
}

// ErrAlias reports a rejected alias header.
var ErrAlias = errors.New("alias header rejected")

// ApplyAliasHeader replaces a compact payload's header row with aliases.
//
// This is a gate, not a formatter convenience: an alias header that loses a
// column, maps two columns onto the same alias, or is longer than the field it
// replaces destroys the reader's ability to recover the record, so it is
// refused rather than emitted.
func ApplyAliasHeader(compact string, aliases map[string]string) (string, error) {
	if compact == "" {
		return "", fmt.Errorf("%w: empty payload", ErrAlias)
	}
	newline := strings.IndexByte(compact, '\n')
	if newline < 0 {
		return "", fmt.Errorf("%w: payload has no header row", ErrAlias)
	}
	header := compact[:newline]
	rest := compact[newline:]

	columns := strings.Split(header, ",")
	seen := make(map[string]string, len(columns))
	renamed := make([]string, 0, len(columns))
	for _, col := range columns {
		alias, ok := aliases[col]
		if !ok || alias == "" {
			return "", fmt.Errorf("%w: no alias registered for column %q", ErrAlias, col)
		}
		if prior, clash := seen[alias]; clash {
			return "", fmt.Errorf("%w: alias %q maps both %q and %q", ErrAlias, alias, prior, col)
		}
		if len(alias) >= len(col) {
			return "", fmt.Errorf("%w: alias %q does not shorten column %q", ErrAlias, alias, col)
		}
		seen[alias] = col
		renamed = append(renamed, alias)
	}
	return strings.Join(renamed, ",") + rest, nil
}

// ErrPreservation reports a layer whose decoded records differ from the
// canonical scenario records.
var ErrPreservation = errors.New("layer does not preserve scenario data")

// DecodeScenarioFields recovers the scenario fields from a rendered layer.
func DecodeScenarioFields(id LayerID, payload string) ([]map[string]string, error) {
	switch id {
	case L0JSONPrettyFull, L1JSONMinFull, L2JSONPrettyProjected, L3JSONMinProjected:
		return decodeJSONLayer(payload)
	case L4CompactProjected:
		return decodeCompactLayer(payload, nil)
	case L5CompactAliasProjected:
		return decodeCompactLayer(payload, AliasHeader)
	default:
		return nil, fmt.Errorf("unknown layer %q", id)
	}
}

func decodeJSONLayer(payload string) ([]map[string]string, error) {
	var rows []map[string]any
	if err := json.Unmarshal([]byte(payload), &rows); err != nil {
		return nil, fmt.Errorf("decode json layer: %w", err)
	}
	out := make([]map[string]string, 0, len(rows))
	for i, row := range rows {
		rec := make(map[string]string, len(ScenarioFields))
		for _, f := range ScenarioFields {
			v, ok := row[f]
			if !ok {
				return nil, fmt.Errorf("decode json layer: row %d is missing scenario field %q", i, f)
			}
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("decode json layer: row %d field %q is %T, want string", i, f, v)
			}
			rec[f] = s
		}
		out = append(out, rec)
	}
	return out, nil
}

// decodeCompactLayer parses header+value output. When aliases is non-nil the
// header is expected to carry aliases and is mapped back to field names.
func decodeCompactLayer(payload string, aliases map[string]string) ([]map[string]string, error) {
	trimmed := strings.TrimRight(payload, "\n")
	if trimmed == "" {
		return nil, errors.New("decode compact layer: empty payload")
	}
	lines := strings.Split(trimmed, "\n")
	columns := splitCSVRow(lines[0])

	if aliases != nil {
		reverse := make(map[string]string, len(aliases))
		for field, alias := range aliases {
			reverse[alias] = field
		}
		for i, col := range columns {
			field, ok := reverse[col]
			if !ok {
				return nil, fmt.Errorf("decode compact layer: header alias %q is not registered", col)
			}
			columns[i] = field
		}
	}

	out := make([]map[string]string, 0, len(lines)-1)
	for i, line := range lines[1:] {
		values := splitCSVRow(line)
		if len(values) != len(columns) {
			return nil, fmt.Errorf("decode compact layer: row %d has %d values, header has %d columns", i, len(values), len(columns))
		}
		rec := make(map[string]string, len(columns))
		for j, col := range columns {
			rec[col] = values[j]
		}
		for _, f := range ScenarioFields {
			if _, ok := rec[f]; !ok {
				return nil, fmt.Errorf("decode compact layer: row %d is missing scenario field %q", i, f)
			}
		}
		out = append(out, rec)
	}
	return out, nil
}

// splitCSVRow mirrors agentquery's escapeCSV quoting rules.
func splitCSVRow(line string) []string {
	var (
		fields  []string
		current strings.Builder
		quoted  bool
	)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted && c == '"':
			if i+1 < len(line) && line[i+1] == '"' {
				current.WriteByte('"')
				i++
				continue
			}
			quoted = false
		case !quoted && c == '"' && current.Len() == 0:
			quoted = true
		case !quoted && c == ',':
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteByte(c)
		}
	}
	fields = append(fields, current.String())
	return fields
}

// VerifyPreservation is the data-preservation gate. It refuses any layer whose
// decoded scenario records differ from the canonical records in count, order,
// field set or value. Token savings measured across layers mean nothing unless
// every layer still answers the same scenario question.
func VerifyPreservation(id LayerID, payload string, canonical []map[string]string) error {
	decoded, err := DecodeScenarioFields(id, payload)
	if err != nil {
		return fmt.Errorf("%w: layer %s: %v", ErrPreservation, id, err)
	}
	if len(decoded) != len(canonical) {
		return fmt.Errorf("%w: layer %s: %d records, want %d", ErrPreservation, id, len(decoded), len(canonical))
	}
	comparedRecords := canonical
	comparedFields := ScenarioFields
	for i := range comparedRecords {
		for _, f := range comparedFields {
			got, want := decoded[i][f], canonical[i][f]
			if got != want {
				return fmt.Errorf("%w: layer %s: record %d field %q is %q, want %q", ErrPreservation, id, i, f, got, want)
			}
		}
	}
	return nil
}

// ScenarioDigest is a stable digest of the scenario records a layer carries.
// The Python measurement refuses to compare layers whose digests disagree.
func ScenarioDigest(records []map[string]string) string {
	sum := sha256.Sum256(RecordsDigestInput(records))
	return hex.EncodeToString(sum[:])
}
