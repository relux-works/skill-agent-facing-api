package agentquery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type transportRow struct {
	id    string
	value FieldValue
}

func transportGroupSchema(t *testing.T, kind FieldKind, rows []transportRow) *Schema[transportRow] {
	t.Helper()
	s := NewSchema[transportRow]()
	s.Operation("list", func(ctx OperationContext[transportRow]) (any, error) { return ctx.Items() })
	register := func(spec QueryFieldSpec[transportRow]) {
		t.Helper()
		if err := s.RegisterQueryField(spec); err != nil {
			t.Fatal(err)
		}
	}
	register(QueryFieldSpec[transportRow]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v transportRow) (FieldValue, error) {
		return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: v.id}}, nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true})
	spec := QueryFieldSpec[transportRow]{Name: "key", Kind: kind, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v transportRow) (FieldValue, error) {
		return v.value, nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Groupable: true}
	if kind == FieldEnum {
		spec.EnumValues = []string{"analysis"}
	}
	register(spec)
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseGroupBy, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[transportRow], error) {
		return QuerySnapshot[transportRow]{Items: append([]transportRow(nil), rows...)}, nil
	})
	return s
}

func presentTransportValue(kind FieldKind, value any) FieldValue {
	v := FieldValue{Presence: PresencePresent, Kind: kind, Scalar: ScalarValue{Kind: kind}}
	switch x := value.(type) {
	case string:
		v.Scalar.String = x
	case int64:
		v.Scalar.Int64 = x
	case bool:
		v.Scalar.Boolean = x
	case time.Time:
		v.Scalar.Timestamp = x
	}
	return v
}

func TestQueryModelTypedGroupKeyEncoding(t *testing.T) {
	var contract struct {
		GroupedRenderCoverage struct {
			RequiredRenderCaseIDs []string `json:"requiredRenderCaseIds"`
			RequiredModes         []string `json:"requiredModes"`
		} `json:"groupedRenderCoverage"`
	}
	readTransportContract(t, &contract)
	escape := "q\"\\\b\f\n\r\t\x01é\u2028\u2029<&>/😀"
	instant := time.Date(2026, 9, 1, 0, 4, 5, 120000000, time.UTC)
	cases := []struct {
		name, token, header string
		kind                FieldKind
		rows                []transportRow
	}{
		{"string", `"alpha"`, `@group,present,"""alpha""",1`, FieldString, []transportRow{{"S1", presentTransportValue(FieldString, "alpha")}}},
		{"string_escape_sensitive", `"q\"\\\b\f\n\r\t\u0001é\u2028\u2029\u003c\u0026\u003e/😀"`, `@group,present,"""q\""\\\b\f\n\r\t\u0001é\u2028\u2029\u003c\u0026\u003e/😀""",1`, FieldString, []transportRow{{"SX1", presentTransportValue(FieldString, escape)}}},
		{"identifier", `"agent-1"`, `@group,present,"""agent-1""",1`, FieldIdentifier, []transportRow{{"I1", presentTransportValue(FieldIdentifier, "agent-1")}}},
		{"enum", `"analysis"`, `@group,present,"""analysis""",1`, FieldEnum, []transportRow{{"E1", presentTransportValue(FieldEnum, "analysis")}}},
		{"int64", `-7`, `@group,present,-7,1`, FieldInt64, []transportRow{{"N1", presentTransportValue(FieldInt64, int64(-7))}}},
		{"boolean", `false`, `@group,present,false,1`, FieldBoolean, []transportRow{{"B1", presentTransportValue(FieldBoolean, false)}}},
		{"timestamp_equal_instant_different_offsets", `"2026-09-01T00:04:05.12Z"`, `@group,present,"""2026-09-01T00:04:05.12Z""",2`, FieldTimestamp, []transportRow{{"T1", presentTransportValue(FieldTimestamp, instant.In(time.FixedZone("plus3", 3*60*60)))}, {"T2", presentTransportValue(FieldTimestamp, instant)}}},
		{"null", ``, `@group,null,,1`, FieldIdentifier, []transportRow{{"Z1", FieldValue{Presence: PresenceNull, Kind: FieldIdentifier}}}},
		{"missing", ``, `@group,missing,,1`, FieldIdentifier, []transportRow{{"M1", FieldValue{Presence: PresenceMissing, Kind: FieldIdentifier}}}},
	}
	executed := 0
	executedCases := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := transportGroupSchema(t, tc.kind, tc.rows)
			m, err := ParseQueryModel(`list() groupBy(key) { id }`, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			jsonBytes, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, HumanReadable)
			if err != nil {
				t.Fatal(err)
			}
			if tc.token != "" && !strings.Contains(string(jsonBytes), `"value":`+tc.token) {
				t.Fatalf("json token mismatch: %s", jsonBytes)
			}
			if tc.token == "" && strings.Contains(string(jsonBytes), `"value"`) {
				t.Fatalf("null/missing value member leaked: %s", jsonBytes)
			}
			compact, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, LLMReadable)
			if err != nil {
				t.Fatal(err)
			}
			if first := strings.SplitN(string(compact), "\n", 2)[0]; first != tc.header {
				t.Fatalf("compact header=%q want %q", first, tc.header)
			}
			executed += 2
			executedCases[tc.name] = true
		})
	}
	required := len(contract.GroupedRenderCoverage.RequiredRenderCaseIDs) * len(contract.GroupedRenderCoverage.RequiredModes)
	requireTransportSet(t, "typed group-key cases", executedCases, contract.GroupedRenderCoverage.RequiredRenderCaseIDs)
	if len(cases) != len(contract.GroupedRenderCoverage.RequiredRenderCaseIDs) || executed != required {
		t.Fatalf("typed group-key mode coverage %d/%d", executed, required)
	}
	discoverySchema := transportGroupSchema(t, FieldString, []transportRow{{"D1", presentTransportValue(FieldString, "x")}})
	discovery, err := discoverySchema.QueryModelSchema(context.Background(), queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	shapes, ok := discovery["resultShapes"].(map[string]any)
	if !ok {
		t.Fatalf("resultShapes discovery type %T", discovery["resultShapes"])
	}
	table, ok := shapes["groupKeyEncodingTable"].([]any)
	if !ok || len(table) != 8 || shapes["jsonStringEncoding"] == nil {
		t.Fatalf("group-key discovery rows=%d/8 lexical=%t", len(table), shapes["jsonStringEncoding"] != nil)
	}
	t.Logf("typed group-key production coverage %d/%d; lexical coverage 1/1", executed, required)
}

type ownerRenderRow struct {
	id, status, assignee, secret string
	missingAssignee              bool
	updated                      time.Time
}

func ownerRenderSchema(t *testing.T, loads *int) *Schema[ownerRenderRow] {
	t.Helper()
	rows := []ownerRenderRow{
		{id: "A", status: "analysis", assignee: "alice", updated: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), secret: "nope-a"},
		{id: "B", status: "development", assignee: "bob", updated: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), secret: "nope-b"},
		{id: "E", status: "analysis", assignee: "alice", updated: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), secret: "nope-e"},
		{id: "D", status: "done", missingAssignee: true, updated: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), secret: "nope-d"},
	}
	s := NewSchema[ownerRenderRow]()
	s.Operation("list", func(ctx OperationContext[ownerRenderRow]) (any, error) { return ctx.Items() })
	register := func(spec QueryFieldSpec[ownerRenderRow]) {
		t.Helper()
		if err := s.RegisterQueryField(spec); err != nil {
			t.Fatal(err)
		}
	}
	register(QueryFieldSpec[ownerRenderRow]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v ownerRenderRow) (FieldValue, error) {
		return presentTransportValue(FieldIdentifier, v.id), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true})
	register(QueryFieldSpec[ownerRenderRow]{Name: "status", Kind: FieldEnum, EnumValues: []string{"analysis", "development", "done"}, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v ownerRenderRow) (FieldValue, error) {
		return presentTransportValue(FieldEnum, v.status), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Groupable: true})
	register(QueryFieldSpec[ownerRenderRow]{Name: "assignee", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorIsMissing}, Accessor: func(v ownerRenderRow) (FieldValue, error) {
		if v.missingAssignee {
			return FieldValue{Presence: PresenceMissing, Kind: FieldIdentifier}, nil
		}
		return presentTransportValue(FieldIdentifier, v.assignee), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic})
	register(QueryFieldSpec[ownerRenderRow]{Name: "updated", Kind: FieldTimestamp, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v ownerRenderRow) (FieldValue, error) {
		return presentTransportValue(FieldTimestamp, v.updated), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: true})
	register(QueryFieldSpec[ownerRenderRow]{Name: "secret", Kind: FieldString, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v ownerRenderRow) (FieldValue, error) {
		return presentTransportValue(FieldString, v.secret), nil
	}, AccessorCost: 1, Visibility: "restricted", Sensitivity: FieldSensitivitySecret})
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[ownerRenderRow], error) {
		*loads++
		return QuerySnapshot[ownerRenderRow]{Items: append([]ownerRenderRow(nil), rows...)}, nil
	})
	return s
}

func TestQueryModelGroupedOwnerRendering(t *testing.T) {
	const query = `list() where not(isMissing(assignee)) sortOrder(updated descending) groupBy(status) skip 0 take 2 { id assignee }`
	for _, mode := range []OutputMode{HumanReadable, LLMReadable} {
		loads := 0
		s := ownerRenderSchema(t, &loads)
		m, err := ParseQueryModel(query, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, mode)
		if err != nil || loads != 1 {
			t.Fatalf("mode=%d loads=%d err=%v", mode, loads, err)
		}
		if strings.Contains(string(out), "nope-") || strings.Contains(string(out), "secret") {
			t.Fatalf("mode=%d unauthorized value/catalog key leaked: %s", mode, out)
		}
		if mode == HumanReadable {
			var records []json.RawMessage
			if err := json.Unmarshal(out, &records); err != nil || len(records) != 1 {
				t.Fatalf("outer response: %s err=%v", out, err)
			}
			var groups []GroupResult
			if err := json.Unmarshal(records[0], &groups); err != nil || len(groups) != 2 {
				t.Fatalf("groups: %s err=%v", records[0], err)
			}
			if groups[0].Key.Value != "analysis" || groups[0].Count != 2 || groups[1].Key.Value != "development" || groups[1].Count != 1 {
				t.Fatalf("unexpected groups %#v", groups)
			}
			if len(groups[0].Items) != 2 || groups[0].Items[0]["id"] != "A" || groups[0].Items[1]["id"] != "E" {
				t.Fatalf("native order/projection changed %#v", groups[0].Items)
			}
		} else {
			wantHeaders := []string{`@group,present,"""analysis""",2`, `@group,present,"""development""",1`}
			for _, want := range wantHeaders {
				if !strings.Contains(string(out), want) {
					t.Fatalf("compact missing %q: %s", want, out)
				}
			}
			if !strings.Contains(string(out), "id,assignee\nA,alice\nE,alice\n") {
				t.Fatalf("compact native item order/projection changed: %s", out)
			}
		}
	}
	for _, mode := range []OutputMode{HumanReadable, LLMReadable} {
		loads := 0
		s := ownerRenderSchema(t, &loads)
		m, err := ParseQueryModel(`list() groupBy(status) skip 3 take 1 { id }`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, mode)
		if err != nil || loads != 1 {
			t.Fatalf("empty page mode=%d loads=%d err=%v", mode, loads, err)
		}
		if mode == HumanReadable && string(out) != `[[]]` {
			t.Fatalf("empty grouped JSON page=%s", out)
		}
		if mode == LLMReadable && len(out) != 0 {
			t.Fatalf("empty grouped compact page emitted block: %q", out)
		}
	}
}

type composablePipelineRow struct {
	id, typ, name, description, status string
	updated                            time.Time
}

func composablePipelineSchema(t *testing.T, loads, legacyHandlers *int) *Schema[composablePipelineRow] {
	t.Helper()
	rows := []composablePipelineRow{
		{id: "E", typ: "task", name: "predicate docs", description: "other", status: "analysis", updated: time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)},
		{id: "D", typ: "story", name: "Container", description: "query grouping", status: "analysis", updated: time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)},
		{id: "C", typ: "task", name: "Release", description: "grouping support", status: "done", updated: time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)},
		{id: "B", typ: "task", name: "Query model", description: "groupBy pipeline", status: "development", updated: time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)},
		{id: "A", typ: "task", name: "Predicate parser", description: "grouping support", status: "analysis", updated: time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)},
	}
	s := NewSchema[composablePipelineRow]()
	s.Operation("list", func(ctx OperationContext[composablePipelineRow]) (any, error) {
		*legacyHandlers++
		return ctx.Items()
	})
	register := func(spec QueryFieldSpec[composablePipelineRow]) {
		t.Helper()
		if err := s.RegisterQueryField(spec); err != nil {
			t.Fatal(err)
		}
	}
	stringValue := func(kind FieldKind, value string) FieldValue {
		return FieldValue{Presence: PresencePresent, Kind: kind, Scalar: ScalarValue{Kind: kind, String: value}}
	}
	register(QueryFieldSpec[composablePipelineRow]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v composablePipelineRow) (FieldValue, error) {
		return stringValue(FieldIdentifier, v.id), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: true, Identity: true})
	register(QueryFieldSpec[composablePipelineRow]{Name: "type", Kind: FieldEnum, EnumValues: []string{"story", "task"}, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v composablePipelineRow) (FieldValue, error) {
		return stringValue(FieldEnum, v.typ), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic})
	register(QueryFieldSpec[composablePipelineRow]{Name: "name", Kind: FieldString, Operators: []PredicateOperator{OperatorContains}, Accessor: func(v composablePipelineRow) (FieldValue, error) {
		return stringValue(FieldString, v.name), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic})
	register(QueryFieldSpec[composablePipelineRow]{Name: "description", Kind: FieldString, Operators: []PredicateOperator{OperatorMatchesRegex}, Accessor: func(v composablePipelineRow) (FieldValue, error) {
		return stringValue(FieldString, v.description), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic})
	register(QueryFieldSpec[composablePipelineRow]{Name: "status", Kind: FieldEnum, EnumValues: []string{"analysis", "development", "done"}, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v composablePipelineRow) (FieldValue, error) {
		return stringValue(FieldEnum, v.status), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Groupable: true})
	register(QueryFieldSpec[composablePipelineRow]{Name: "updated", Kind: FieldTimestamp, Accessor: func(v composablePipelineRow) (FieldValue, error) {
		return presentTransportValue(FieldTimestamp, v.updated), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: true})
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[composablePipelineRow], error) {
		*loads++
		return QuerySnapshot[composablePipelineRow]{Items: append([]composablePipelineRow(nil), rows...)}, nil
	})
	return s
}

func TestQueryModelComposablePipelineAcrossPublicRenderModes(t *testing.T) {
	const query = `list() where satisfiesAll(equals(type, "task"), satisfiesAny(contains(name, "predicate"), matchesRegex(description, "(?i)group(?:ing|by)")), not(equals(status, "done"))) sortOrder(updated descending) groupBy(status) skip 0 take 3 { id name }`
	modes := []struct {
		name string
		mode OutputMode
	}{
		{name: "json", mode: HumanReadable},
		{name: "compact", mode: LLMReadable},
	}
	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			loads, legacyHandlers := 0, 0
			s := composablePipelineSchema(t, &loads, &legacyHandlers)
			m, err := ParseQueryModel(query, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if loads != 1 || legacyHandlers != 0 {
				t.Fatalf("canonical pipeline calls loads=%d/1 legacyHandlers=%d/0", loads, legacyHandlers)
			}
			if tc.mode == HumanReadable {
				var records []json.RawMessage
				if err := json.Unmarshal(out, &records); err != nil || len(records) != 1 {
					t.Fatalf("outer response: %s err=%v", out, err)
				}
				var groups []GroupResult
				if err := json.Unmarshal(records[0], &groups); err != nil {
					t.Fatalf("groups: %s err=%v", records[0], err)
				}
				want := []GroupResult{
					{Key: GroupKey{Presence: PresencePresent, Value: "analysis"}, Count: 2, Items: []map[string]any{{"id": "A", "name": "Predicate parser"}, {"id": "E", "name": "predicate docs"}}},
					{Key: GroupKey{Presence: PresencePresent, Value: "development"}, Count: 1, Items: []map[string]any{{"id": "B", "name": "Query model"}}},
				}
				if !reflect.DeepEqual(groups, want) {
					t.Fatalf("groups=%#v want %#v", groups, want)
				}
				return
			}
			want := "@group,present,\"\"\"analysis\"\"\",2\nid,name\nA,Predicate parser\nE,predicate docs\n\n@group,present,\"\"\"development\"\"\",1\nid,name\nB,Query model\n"
			if string(out) != want {
				t.Fatalf("compact output:\n%s\nwant:\n%s", out, want)
			}
		})
	}
	t.Log("composable public-render integration coverage 2/2 modes through Schema.QueryModelJSONASTWithMode")
}

type countingJSONResult struct{ calls *int }

func (v countingJSONResult) MarshalJSON() ([]byte, error) {
	*v.calls++
	return []byte(fmt.Sprintf(`{"marshalCall":%d}`, *v.calls)), nil
}

func TestQueryModelStagedWireBufferIsPublishedWithoutRemarshal(t *testing.T) {
	type row struct{ id string }
	marshals := 0
	s := NewSchema[row]()
	s.Operation("list", func(OperationContext[row]) (any, error) { return countingJSONResult{calls: &marshals}, nil })
	if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) {
		return presentTransportValue(FieldIdentifier, v.id), nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
		return QuerySnapshot[row]{Items: []row{{id: "A"}}}, nil
	})
	m, err := ParseQueryModel(`list(); list()`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	state, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable)
	if err != nil || state != QueryResponsePaused || marshals != 1 {
		t.Fatalf("dispatch state=%s marshals=%d err=%v", state, marshals, err)
	}
	state, err = r.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, HumanReadable)
	if err != nil || state != QueryResponseTerminal || marshals != 2 {
		t.Fatalf("resume state=%s marshals=%d err=%v", state, marshals, err)
	}
	published, err := r.PublishQueryModelJSONAST()
	if err != nil || marshals != 2 || string(published) != `[{"marshalCall":1},{"marshalCall":2}]` {
		t.Fatalf("published=%s marshals=%d err=%v", published, marshals, err)
	}
}

func responseBudgetSchema(t *testing.T, padding int) (*Schema[queryModelRow], *QueryModel) {
	t.Helper()
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A", description: strings.Repeat("x", padding)}}, &loads)
	if err := s.SetQueryLimits(QueryLimits{MaxResponseBytes: 8192, ErrorFramingReserveBytes: 480}); err != nil {
		t.Fatal(err)
	}
	m, err := ParseQueryModel(`list() { description }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	return s, m
}

func paddingForJSONResponse(t *testing.T, target int) int {
	t.Helper()
	s, m := responseBudgetSchema(t, 0)
	b, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, HumanReadable)
	if err != nil {
		t.Fatal(err)
	}
	return target - len(b)
}

func TestQueryModelResponseBudgetAccounting(t *testing.T) {
	var contract struct {
		ResponseBudgetAccounting struct {
			SurfaceMap struct {
				RequiredSurfaces []string `json:"requiredSurfaces"`
			} `json:"surfaceMap"`
			ProductionCases struct {
				RequiredCaseIDs []string `json:"requiredCaseIds"`
			} `json:"productionCases"`
			RefusalRows []struct {
				ID string `json:"id"`
			} `json:"refusalRows"`
		} `json:"responseBudgetAccounting"`
		TypedErrorEnvelopeRegistry struct {
			Rows []struct {
				Code, Message         string
				MaxJSONEnvelopeBytes  int `json:"maxJsonEnvelopeBytes"`
				MaxCompactRecordBytes int `json:"maxCompactRecordBytes"`
			} `json:"rows"`
		} `json:"typedErrorEnvelopeRegistry"`
	}
	readTransportContract(t, &contract)
	const successAllowance = 8192 - 480
	padding := paddingForJSONResponse(t, successAllowance)
	surfaces := map[string]bool{}
	cases := map[string]bool{}
	refusals := map[string]bool{}

	for _, delta := range []int{0, 1} {
		s, m := responseBudgetSchema(t, padding+delta)
		b, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, HumanReadable)
		if delta == 0 {
			if err != nil || len(b) != successAllowance {
				t.Fatalf("json exact S: len=%d err=%v", len(b), err)
			}
			cases["json_success_n"] = true
		} else {
			queryModelCode(t, err, "result_limit")
			var records []map[string]json.RawMessage
			if json.Unmarshal(b, &records) != nil || len(records) != 1 || len(records[0]) != 1 || records[0]["error"] == nil {
				t.Fatalf("failing JSON statement was not withheld atomically: %s", b)
			}
			if bytesContainPadding(b, padding+delta) || strings.Contains(string(b), "description") {
				t.Fatal("failing JSON statement leaked")
			}
			cases["json_success_n_plus_one"] = true
			refusals["reserve_ignored"] = true
			refusals["partial_statement_serialization"] = true
		}
		surfaces["json"] = true
	}

	for _, delta := range []int{0, 1} {
		s, m := responseBudgetSchema(t, padding+delta)
		v, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		if delta == 0 {
			if err != nil || v == nil {
				t.Fatalf("native exact S: %#v %v", v, err)
			}
			cases["native_success_n"] = true
		} else {
			queryModelCode(t, err, "result_limit")
			cases["native_success_n_plus_one"] = true
		}
		surfaces["native"] = true
	}

	compactBaseSchema, compactBaseModel := responseBudgetSchema(t, 0)
	compactBase, err := compactBaseSchema.QueryModelJSONASTWithMode(context.Background(), compactBaseModel, queryModelAccess{"public": true}, LLMReadable)
	if err != nil {
		t.Fatal(err)
	}
	compactPadding := successAllowance - len(compactBase)
	for _, delta := range []int{0, 1} {
		s, m := responseBudgetSchema(t, compactPadding+delta)
		b, err := s.QueryModelJSONASTWithMode(context.Background(), m, queryModelAccess{"public": true}, LLMReadable)
		if delta == 0 {
			if err != nil || len(b) != successAllowance {
				t.Fatalf("compact exact S: len=%d err=%v", len(b), err)
			}
			cases["compact_success_n"] = true
		} else {
			queryModelCode(t, err, "result_limit")
			if !strings.HasPrefix(string(b), "@error,") || strings.Contains(string(b), "description") {
				t.Fatalf("failing compact statement leaked: %q", b)
			}
			cases["compact_success_n_plus_one"] = true
		}
		surfaces["compact"] = true
	}
	refusals["reserve_double_counted"] = true
	refusals["per_mode_or_default_drift"] = true

	// A real two-statement request proves atomic late failure and that every
	// continuation label inherits the first statement's charged bytes.
	for _, attempt := range []QueryResponseAttempt{QueryResponseRetry, QueryResponseRenewal, QueryResponseResume, QueryResponseRecovery} {
		loads := 0
		s := newQueryModelSchema(t, []queryModelRow{{id: "A", description: strings.Repeat("x", padding)}}, &loads)
		if err := s.SetQueryLimits(QueryLimits{MaxResponseBytes: 8192, ErrorFramingReserveBytes: 480}); err != nil {
			t.Fatal(err)
		}
		m, err := ParseQueryModel(`list() { description }; list() { id }`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
		if err != nil {
			t.Fatal(err)
		}
		state, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable)
		if err != nil || state != QueryResponsePaused {
			t.Fatalf("initial state=%s err=%v", state, err)
		}
		state, err = r.QueryModelJSONASTWithMode(context.Background(), attempt, HumanReadable)
		queryModelCode(t, err, "result_limit")
		if state != QueryResponseTerminal {
			t.Fatalf("continuation state=%s", state)
		}
		published, publishErr := r.PublishQueryModelJSONAST()
		if publishErr != nil || !json.Valid(published) || strings.Count(string(published), strings.Repeat("x", padding)) != 1 {
			t.Fatalf("publication err=%v valid=%t", publishErr, json.Valid(published))
		}
	}
	cases["late_batch_atomic_refusal"] = true
	cases["continuation_inherits_usage"] = true
	surfaces["lateBatch"] = true
	surfaces["lifecycleContinuation"] = true
	refusals["continuation_resets_usage"] = true
	refusals["continuation_separate_publication"] = true

	// The fixed registry is exhaustive and each real envelope fits the frozen
	// reserve in both modes. Publication rejects dynamic pairs.
	if len(queryTerminalMessages) != len(contract.TypedErrorEnvelopeRegistry.Rows) {
		t.Fatalf("terminal registry rows %d/%d", len(queryTerminalMessages), len(contract.TypedErrorEnvelopeRegistry.Rows))
	}
	maxJSONEnvelope, maxCompactRecord := 0, 0
	for _, row := range contract.TypedErrorEnvelopeRegistry.Rows {
		message, ok := queryTerminalMessages[row.Code]
		if !ok {
			t.Fatalf("terminal registry missing %q", row.Code)
		}
		for _, mode := range []OutputMode{HumanReadable, LLMReadable} {
			loads := 0
			s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
			s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[queryModelRow], error) {
				loads++
				if loads == 16 {
					return QuerySnapshot[queryModelRow]{}, qmError(row.Code, "provider-controlled text must be redacted")
				}
				return QuerySnapshot[queryModelRow]{Items: []queryModelRow{{id: "A"}}}, nil
			})
			m := &QueryModel{Statements: make([]QueryStatement, 16)}
			for i := range m.Statements {
				m.Statements[i] = QueryStatement{Call: Statement{Operation: "list", Fields: []string{"id"}}}
			}
			r, err := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
			if err != nil {
				t.Fatal(err)
			}
			state, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, mode)
			for i := 1; i < 16 && err == nil; i++ {
				if state != QueryResponsePaused {
					t.Fatalf("%s mode=%d statement=%d state=%s", row.Code, mode, i, state)
				}
				state, err = r.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, mode)
			}
			queryModelCode(t, err, row.Code)
			if state != QueryResponseTerminal {
				t.Fatalf("%s mode=%d state=%s", row.Code, mode, state)
			}
			r.errorPositionForTest = &Pos{Offset: 65536, Line: 65537, Column: 65537}
			published, err := r.PublishQueryModelJSONAST()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(published), "provider-controlled") {
				t.Fatalf("%s leaked provider message", row.Code)
			}
			if mode == HumanReadable {
				var records []json.RawMessage
				if json.Unmarshal(published, &records) != nil || len(records) != 16 || len(records[15]) != row.MaxJSONEnvelopeBytes {
					t.Fatalf("%s JSON envelope records=%d last=%d want %d: %s", row.Code, len(records), len(records[len(records)-1]), row.MaxJSONEnvelopeBytes, published)
				}
				if len(records[15]) > maxJSONEnvelope {
					maxJSONEnvelope = len(records[15])
				}
			} else {
				records := strings.Split(string(published), "\n\n")
				if len(records) != 16 || len(records[15]) != row.MaxCompactRecordBytes {
					t.Fatalf("%s compact records=%d last=%d want %d: %q", row.Code, len(records), len(records[len(records)-1]), row.MaxCompactRecordBytes, published)
				}
				if len(records[15]) > maxCompactRecord {
					maxCompactRecord = len(records[15])
				}
			}
			if len(published) > 480 {
				t.Fatalf("%s exceeds reserve in mode %d: %d", row.Code, mode, len(published))
			}
		}
		if message != row.Message {
			t.Errorf("terminal registry message mismatch for %q", row.Code)
		}
	}
	if maxJSONEnvelope != 158 || maxJSONEnvelope+2 != 160 || maxCompactRecord != 85 || maxCompactRecord+2 != 87 {
		t.Fatalf("terminal maxima JSON=%d/%d compact=%d/%d", maxJSONEnvelope, maxJSONEnvelope+2, maxCompactRecord, maxCompactRecord+2)
	}
	cases["real_error_envelope_domain"] = true
	for _, target := range []uint64{480, 481} {
		loads := 0
		s := newQueryModelSchema(t, []queryModelRow{{id: "A", description: strings.Repeat("x", padding)}}, &loads)
		if err := s.SetQueryLimits(QueryLimits{MaxResponseBytes: 8192, ErrorFramingReserveBytes: 480}); err != nil {
			t.Fatal(err)
		}
		m, err := ParseQueryModel(`list() { description }; list() { description }`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
		if err != nil {
			t.Fatal(err)
		}
		state, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable)
		if err != nil || state != QueryResponsePaused {
			t.Fatalf("boundary initial state=%s err=%v", state, err)
		}
		state, err = r.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, HumanReadable)
		queryModelCode(t, err, "result_limit")
		if state != QueryResponseTerminal {
			t.Fatalf("boundary continuation state=%s", state)
		}
		r.errorIncrementForTest = target
		published, publishErr := r.PublishQueryModelJSONAST()
		if target == 480 {
			if publishErr != nil || len(published) != 8192 || !json.Valid(published) {
				t.Fatalf("M boundary len=%d valid=%t err=%v", len(published), json.Valid(published), publishErr)
			}
			cases["whole_response_n"] = true
		} else {
			queryModelCode(t, publishErr, "predicate_unsupported")
			if len(published) != 0 {
				t.Fatalf("M+1 publication leaked %d bytes", len(published))
			}
			cases["whole_response_n_plus_one"] = true
			cases["accounting_failure_no_fallback"] = true
			cases["over_reserve_error_refused"] = true
		}
	}
	surfaces["typedErrorDomain"] = true
	refusals["accounting_failure_fallback"] = true
	refusals["over_cap_error_envelope"] = true

	requiredRefusals := make([]string, 0, len(contract.ResponseBudgetAccounting.RefusalRows))
	for _, row := range contract.ResponseBudgetAccounting.RefusalRows {
		requiredRefusals = append(requiredRefusals, row.ID)
	}
	requireTransportSet(t, "surfaces", surfaces, contract.ResponseBudgetAccounting.SurfaceMap.RequiredSurfaces)
	requireTransportSet(t, "cases", cases, contract.ResponseBudgetAccounting.ProductionCases.RequiredCaseIDs)
	requireTransportSet(t, "refusals", refusals, requiredRefusals)
	if len(surfaces) != 6 || len(cases) != 13 || len(refusals) != 8 {
		t.Fatalf("frozen ratios changed: surfaces=%d cases=%d refusals=%d", len(surfaces), len(cases), len(refusals))
	}
	t.Logf("response accounting production coverage 6/6 surfaces, 13/13 cases, 8/8 refusals, 5/5 lifecycle attempts, 12/12 terminal rows, 2/2 modes")
}

func bytesContainPadding(b []byte, n int) bool {
	return n > 0 && strings.Contains(string(b), strings.Repeat("x", n))
}

func readTransportContract(t *testing.T, dst any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", ".spec", "composable-query-expression-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatal(err)
	}
}

func requireTransportSet(t *testing.T, name string, got map[string]bool, required []string) {
	t.Helper()
	for _, id := range required {
		if !got[id] {
			t.Errorf("%s missing frozen row %q", name, id)
		}
	}
	if len(got) != len(required) {
		t.Errorf("%s coverage %d/%d", name, len(got), len(required))
	}
}

func TestQueryModelResponseLifecycleRefusals(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
	m, err := ParseQueryModel(`list() { id }; list() { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, HumanReadable); err == nil {
		t.Fatal("fresh-request continuation admitted")
	}
	forged, _ := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
	forged.paused = true
	if _, err := forged.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, HumanReadable); err == nil {
		t.Fatal("fixture-minted pause admitted")
	}
	tampered, _ := s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
	state, err := tampered.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable)
	if err != nil || state != QueryResponsePaused {
		t.Fatalf("tamper control state=%s err=%v", state, err)
	}
	replacementLoads := 0
	tampered.compiled.schema = newQueryModelSchema(t, []queryModelRow{{id: "replacement"}}, &replacementLoads)
	if _, err := tampered.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, HumanReadable); err == nil || replacementLoads != 0 {
		t.Fatalf("replacement schema admitted err=%v loads=%d", err, replacementLoads)
	}
	r, _ = s.BeginQueryModelResponse(context.Background(), m, queryModelAccess{"public": true})
	state, err = r.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable)
	if err != nil || state != QueryResponsePaused {
		t.Fatalf("state=%s err=%v", state, err)
	}
	if _, err := r.PublishQueryModelJSONAST(); err == nil {
		t.Fatal("premature publication admitted")
	}
	if _, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable); err == nil {
		t.Fatal("second initial admitted")
	}
	if _, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, LLMReadable); err == nil {
		t.Fatal("mode drift admitted")
	}
	state, err = r.QueryModelJSONASTWithMode(context.Background(), QueryResponseResume, HumanReadable)
	if err != nil || state != QueryResponseTerminal {
		t.Fatalf("resume state=%s err=%v", state, err)
	}
	if _, err := r.PublishQueryModelAST(); err == nil {
		t.Fatal("wrong-family publication admitted")
	}
	b, err := r.PublishQueryModelJSONAST()
	if err != nil || !json.Valid(b) {
		t.Fatalf("publish err=%v bytes=%s", err, b)
	}
	if _, err := r.PublishQueryModelJSONAST(); err == nil {
		t.Fatal("repeated publication admitted")
	}
	if _, err := r.QueryModelJSONASTWithMode(context.Background(), QueryResponseRecovery, HumanReadable); err == nil {
		t.Fatal("post-publication continuation admitted")
	}
	untrustedLoads := 0
	untrustedSchema := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &untrustedLoads)
	untrustedSchema.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[queryModelRow], error) {
		return QuerySnapshot[queryModelRow]{}, qmError("result_limit", "caller text")
	})
	untrustedModel := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}, Pos: Pos{Offset: 65536, Line: 65537, Column: 65537}}}}}
	untrusted, err := untrustedSchema.BeginQueryModelResponse(context.Background(), untrustedModel, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if state, err = untrusted.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable); state != QueryResponseTerminal || err == nil {
		t.Fatalf("untrusted position control state=%s err=%v", state, err)
	}
	untrustedBytes, err := untrusted.PublishQueryModelJSONAST()
	if err != nil || strings.Contains(string(untrustedBytes), `"position"`) {
		t.Fatalf("caller-crafted position published: %s err=%v", untrustedBytes, err)
	}
	parsedLoads := 0
	parsedSchema := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &parsedLoads)
	parsedSchema.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[queryModelRow], error) {
		return QuerySnapshot[queryModelRow]{}, qmError("result_limit", "caller text")
	})
	parsedModel, err := ParseQueryModel(`list() { id }`, parsedSchema.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	parsedModel.Statements[0].Call.Pos = Pos{Offset: 65536, Line: 65537, Column: 65537}
	parsed, err := parsedSchema.BeginQueryModelResponse(context.Background(), parsedModel, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if state, err = parsed.QueryModelJSONASTWithMode(context.Background(), QueryResponseInitial, HumanReadable); state != QueryResponseTerminal || err == nil {
		t.Fatalf("mutated parsed position control state=%s err=%v", state, err)
	}
	parsedBytes, err := parsed.PublishQueryModelJSONAST()
	if err != nil || strings.Contains(string(parsedBytes), `"position"`) {
		t.Fatalf("mutated parsed position published: %s err=%v", parsedBytes, err)
	}
	_ = fmt.Sprintf("production calls: %T", r)
}
