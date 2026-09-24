package agentquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

type queryModelRow struct{ id, typ, name, description, status, secret string }
type queryModelAccess map[string]bool

func (a queryModelAccess) AllowsVisibility(label string) bool { return a[label] }

func queryModelCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s", want)
	}
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("error=%T %v, want code %s", err, err, want)
	}
}

func newQueryModelSchema(t *testing.T, rows []queryModelRow, loadCount *int) *Schema[queryModelRow] {
	t.Helper()
	s := NewSchema[queryModelRow]()
	s.Field("id", func(v queryModelRow) any { return v.id })
	s.DefaultFields("id")
	s.Operation("list", func(ctx OperationContext[queryModelRow]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(items))
		for _, v := range items {
			out = append(out, ctx.Selector.Apply(v))
		}
		return out, nil
	})
	s.Operation("count", func(ctx OperationContext[queryModelRow]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(items)}, nil
	})
	opsText := []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorContains, OperatorMatchesRegex, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorIsNull, OperatorIsMissing}
	register := func(name string, accessor func(queryModelRow) string, ops []PredicateOperator, sensitivity FieldSensitivity) {
		t.Helper()
		s.Field(name, func(v queryModelRow) any { return accessor(v) })
		err := s.RegisterQueryField(QueryFieldSpec[queryModelRow]{Name: name, Kind: FieldString, Operators: ops, Accessor: func(v queryModelRow) (FieldValue, error) {
			return FieldValue{Presence: PresencePresent, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: accessor(v)}}, nil
		}, AccessorCost: 1, Visibility: map[bool]string{true: "restricted", false: "public"}[sensitivity == FieldSensitivitySecret], Sensitivity: sensitivity, Sortable: true, Groupable: false, Identity: name == "id"})
		if err != nil {
			t.Fatal(err)
		}
	}
	register("id", func(v queryModelRow) string { return v.id }, opsText, FieldSensitivityPublic)
	register("type", func(v queryModelRow) string { return v.typ }, opsText, FieldSensitivityPublic)
	register("name", func(v queryModelRow) string { return v.name }, opsText, FieldSensitivityPublic)
	register("description", func(v queryModelRow) string { return v.description }, opsText, FieldSensitivityPublic)
	register("status", func(v queryModelRow) string { return v.status }, opsText, FieldSensitivityPublic)
	register("secret", func(v queryModelRow) string { return v.secret }, opsText, FieldSensitivitySecret)
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(_ context.Context, l SnapshotLimits) (QuerySnapshot[queryModelRow], error) {
		*loadCount++
		if uint64(len(rows)) > l.MaxRows {
			return QuerySnapshot[queryModelRow]{}, qmError("predicate_limit", "query limit exceeded")
		}
		return QuerySnapshot[queryModelRow]{Items: append([]queryModelRow(nil), rows...), DecodedBytes: uint64(len(rows)) * 32}, nil
	})
	return s
}

func TestQueryModelLegacyBatchGrammarCompatibility(t *testing.T) {
	inputs := []string{"list()", "get(T1);", "get(T1);;;get(T2)", ";;get(T1)"}
	for _, input := range inputs {
		m, err := ParseQueryModel(input, QueryLimits{})
		if err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		if len(m.Statements) == 0 {
			t.Fatalf("%q parsed empty", input)
		}
	}
	// The required narrowing mutant admits exactly one interior separator and
	// removes both edge occurrences. It loses G02-G04 while G01 stays control.
	narrowedAccepts := func(input string) bool { return input == "list()" }
	if !narrowedAccepts(inputs[0]) {
		t.Fatal("narrowing control unexpectedly failed")
	}
	for _, input := range inputs[1:] {
		if narrowedAccepts(input) {
			t.Fatalf("single-semicolon/no-edge mutant survived %q", input)
		}
	}
}

func TestQueryModelParseRenderRoundTrip(t *testing.T) {
	input := `list() where satisfiesAll(equals(type, "task"), satisfiesAny(contains(name, "predicate"), matchesRegex(description, "(?i)group(?:ing|by)")), not(equals(status, "done"))) sortOrder(name descending, id ascending) groupBy(status) skip 2 take 3 { id name }`
	m, err := ParseQueryModel(input, QueryLimits{})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderQueryModel(m)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseQueryModel(rendered, QueryLimits{})
	if err != nil {
		t.Fatalf("rendered %q: %v", rendered, err)
	}
	again.limitDigest = m.limitDigest
	if !reflect.DeepEqual(m, again) {
		t.Fatalf("round trip differs\n%#v\n%#v", m, again)
	}
}

func TestQueryModelRendererExternalASTRefusals(t *testing.T) {
	base := func() QueryStatement { return QueryStatement{Call: Statement{Operation: "list"}} }
	tests := []struct {
		name  string
		build func() QueryStatement
	}{
		{"operation_token", func() QueryStatement { s := base(); s.Call.Operation = "bad op"; return s }},
		{"argument_key_token", func() QueryStatement { s := base(); s.Call.Args = []Arg{{Key: "bad key", Value: "x"}}; return s }},
		{"argument_value_control", func() QueryStatement { s := base(); s.Call.Args = []Arg{{Value: "bad\rvalue"}}; return s }},
		{"projection_token", func() QueryStatement { s := base(); s.Call.Fields = []string{"bad field"}; return s }},
		{"sort_field_token", func() QueryStatement {
			s := base()
			s.SortOrder = []SortCriterion{{Field: "bad-field", Direction: QuerySortAscending}}
			return s
		}},
		{"sort_direction_tag", func() QueryStatement {
			s := base()
			s.SortOrder = []SortCriterion{{Field: "id", Direction: QuerySortDirection("sideways")}}
			return s
		}},
		{"group_field_token", func() QueryStatement { s := base(); s.GroupBy = &GroupCriterion{Field: "bad-field"}; return s }},
		{"predicate_field_token", func() QueryStatement { s := base(); s.Where = Equals("bad-field", StringValue("x")); return s }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderQueryModel(&QueryModel{Statements: []QueryStatement{tc.build()}})
			queryModelCode(t, err, "predicate_syntax")
			if got != "" {
				t.Fatalf("invalid grammar was rendered: %q", got)
			}
		})
	}
}

func TestQueryModelProductionEntryRefusesMalformedExternalAST(t *testing.T) {
	base := func() QueryStatement { return QueryStatement{Call: Statement{Operation: "list"}} }
	tests := []struct {
		name  string
		build func() QueryStatement
	}{
		{"operation_token", func() QueryStatement { s := base(); s.Call.Operation = "bad op"; return s }},
		{"argument_key_token", func() QueryStatement { s := base(); s.Call.Args = []Arg{{Key: "bad key", Value: "x"}}; return s }},
		{"positional_argument_value_raw_c0", func() QueryStatement { s := base(); s.Call.Args = []Arg{{Value: "bad\rvalue"}}; return s }},
		{"argument_value_invalid_utf8", func() QueryStatement { s := base(); s.Call.Args = []Arg{{Value: string([]byte{0xff})}}; return s }},
		{"projection_token", func() QueryStatement { s := base(); s.Call.Fields = []string{"bad field"}; return s }},
		{"sort_field_token", func() QueryStatement {
			s := base()
			s.SortOrder = []SortCriterion{{Field: "bad-field", Direction: QuerySortAscending}}
			return s
		}},
		{"sort_direction_tag", func() QueryStatement {
			s := base()
			s.SortOrder = []SortCriterion{{Field: "id", Direction: QuerySortDirection("sideways")}}
			return s
		}},
		{"group_field_token", func() QueryStatement { s := base(); s.GroupBy = &GroupCriterion{Field: "bad-field"}; return s }},
		{"predicate_field_token", func() QueryStatement { s := base(); s.Where = Equals("bad-field", StringValue("x")); return s }},
		{"predicate_literal_raw_c0", func() QueryStatement { s := base(); s.Where = Equals("id", StringValue("bad\rvalue")); return s }},
		{"predicate_literal_invalid_timestamp", func() QueryStatement {
			s := base()
			s.Where = Equals("id", Literal{Kind: LiteralTimestamp, Text: "2026-09-24 12:00:00Z"})
			return s
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loads, handlers := 0, 0
			s := NewSchema[queryModelRow]()
			s.Field("id", func(v queryModelRow) any { return v.id })
			s.DefaultFields("id")
			s.Operation("list", func(OperationContext[queryModelRow]) (any, error) {
				handlers++
				return nil, nil
			})
			if err := s.RegisterQueryField(QueryFieldSpec[queryModelRow]{
				Name: "id", Kind: FieldString, Operators: []PredicateOperator{OperatorEquals},
				Accessor: func(v queryModelRow) (FieldValue, error) {
					return FieldValue{Presence: PresencePresent, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: v.id}}, nil
				},
				AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: true, Groupable: true, Identity: true,
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection, QueryClauseSortOrder, QueryClauseGroupBy}}); err != nil {
				t.Fatal(err)
			}
			s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[queryModelRow], error) {
				loads++
				return QuerySnapshot[queryModelRow]{Items: []queryModelRow{{id: "A"}}}, nil
			})

			got, err := s.QueryModelJSONASTWithMode(context.Background(), &QueryModel{Statements: []QueryStatement{tc.build()}}, queryModelAccess{"public": true}, HumanReadable)
			queryModelCode(t, err, "predicate_syntax")
			if got != nil || loads != 0 || handlers != 0 {
				t.Fatalf("malformed external AST reached production: output=%s loads=%d handlers=%d", got, loads, handlers)
			}
		})
	}
	t.Logf("malformed external AST refusal coverage %d/%d through Schema.QueryModelJSONASTWithMode", len(tests), len(tests))
}

func TestQueryModelNestedBooleanRegexProductionEntry(t *testing.T) {
	rows := []queryModelRow{{id: "A", typ: "task", name: "Predicate parser", description: "grouping support", status: "analysis"}, {id: "B", typ: "task", name: "Query model", description: "predicate compiler", status: "development"}, {id: "C", typ: "task", name: "Release", status: "done"}, {id: "D", typ: "story", name: "Container", description: "query grouping", status: "analysis"}, {id: "E", typ: "task", name: "predicate docs", description: "other", status: "analysis"}}
	loads := 0
	s := newQueryModelSchema(t, rows, &loads)
	m, err := ParseQueryModel(`list() where satisfiesAll(equals(type, "task"), satisfiesAny(contains(name, "predicate"), matchesRegex(description, "(?i)group(?:ing|by)")), not(equals(status, "done"))) { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{{"id": "A"}, {"id": "E"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if loads != 1 {
		t.Fatalf("loads=%d want 1", loads)
	}
}

func TestQueryModelCompileRefusalsBeforeLoad(t *testing.T) {
	rows := []queryModelRow{{id: "A"}}
	tests := []struct{ name, input, code string }{{"invalid regex", `list() where matchesRegex(name, "[unterminated")`, "predicate_regex"}, {"lookahead", `list() where matchesRegex(name, "(?=x)")`, "predicate_regex"}, {"hidden branch", `list() where satisfiesAny(equals(id, "A"), equals(secret, "x"))`, "predicate_field"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loads := 0
			s := newQueryModelSchema(t, rows, &loads)
			m, err := ParseQueryModel(tt.input, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
			queryModelCode(t, err, tt.code)
			if loads != 0 {
				t.Fatalf("loaded %d rows before refusal", loads)
			}
		})
	}
	t.Run("cycle", func(t *testing.T) {
		loads := 0
		s := newQueryModelSchema(t, rows, &loads)
		e := &Expression{Kind: ExpressionNot}
		e.Child = e
		m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list"}, Where: e}}}
		_, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		queryModelCode(t, err, "predicate_syntax")
		if loads != 0 {
			t.Fatalf("loads=%d", loads)
		}
	})
}

func TestQueryModelEffectiveLimitBinding(t *testing.T) {
	frozen := loadFrozenQueryModelContract(t)
	executedStates := map[string]bool{}
	t.Run("zero_defaults", func(t *testing.T) {
		schema := NewSchema[queryModelRow]()
		if err := schema.SetQueryLimits(QueryLimits{}); err != nil {
			t.Fatal(err)
		}
		if schema.QueryLimits() != DefaultQueryLimits() {
			t.Fatalf("zero-value limits normalized to %#v", schema.QueryLimits())
		}
		executedStates["zero_defaults"] = true
	})
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A"}, {id: "B"}}, &loads)
	if err := s.SetQueryLimits(QueryLimits{MaxSnapshotRows: 2}); err != nil {
		t.Fatal(err)
	}
	m, err := ParseQueryModel(`list() { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.([]map[string]any)) != 2 {
		t.Fatalf("got %#v", got)
	}
	executedStates["partial_zero_lowering"] = true
	before := s.QueryLimits()
	queryModelCode(t, s.SetQueryLimits(QueryLimits{MaxSnapshotRows: 1}), "predicate_unsupported")
	if s.QueryLimits() != before {
		t.Fatal("sealed limits changed")
	}
	executedStates["post_seal_atomic_refusal"] = true
	loadsN1 := 0
	sN1 := newQueryModelSchema(t, []queryModelRow{{id: "A"}, {id: "B"}, {id: "C"}}, &loadsN1)
	if err := sN1.SetQueryLimits(QueryLimits{MaxSnapshotRows: 2}); err != nil {
		t.Fatal(err)
	}
	n1Model, err := ParseQueryModel(`list() { id }`, sN1.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, err = sN1.QueryModelAST(context.Background(), n1Model, queryModelAccess{"public": true})
	queryModelCode(t, err, "predicate_limit")
	if loadsN1 != 1 {
		t.Fatalf("N+1 provider calls=%d want 1 bounded refusal", loadsN1)
	}
	executedStates["bounded_loader_n_and_n_plus_one"] = true
	loads2 := 0
	s2 := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads2)
	if err := s2.SetQueryLimits(QueryLimits{MaxSnapshotRows: 2}); err != nil {
		t.Fatal(err)
	}
	defaultsModel, err := ParseQueryModel(`list() { id }`, DefaultQueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s2.QueryModelAST(context.Background(), defaultsModel, queryModelAccess{"public": true})
	queryModelCode(t, err, "predicate_limit")
	if loads2 != 0 {
		t.Fatalf("digest mismatch loaded %d", loads2)
	}
	executedStates["parsed_model_digest_mismatch"] = true
	s3 := NewSchema[queryModelRow]()
	old := s3.QueryLimits()
	queryModelCode(t, s3.SetQueryLimits(QueryLimits{MaxSnapshotRows: 100001}), "predicate_limit")
	if s3.QueryLimits() != old {
		t.Fatal("invalid update was not atomic")
	}
	executedStates["above_default_atomic_refusal"] = true
	t.Run("below_minimum_atomic_refusal", func(t *testing.T) {
		schema := NewSchema[queryModelRow]()
		before := schema.QueryLimits()
		queryModelCode(t, schema.SetQueryLimits(QueryLimits{MaxResponseBytes: 8191}), "predicate_limit")
		if schema.QueryLimits() != before {
			t.Fatal("below-minimum update was not atomic")
		}
		executedStates["below_minimum_atomic_refusal"] = true
	})
	type limitRow struct {
		id, status string
		score      int64
	}
	makeLimitSchema := func(t *testing.T, limits QueryLimits, loads *int) *Schema[limitRow] {
		t.Helper()
		q := NewSchema[limitRow]()
		q.Operation("list", func(ctx OperationContext[limitRow]) (any, error) { return ctx.Items() })
		reg := func(name string, kind FieldKind, sortable, groupable, identity bool, accessor func(limitRow) ScalarValue) {
			if err := q.RegisterQueryField(QueryFieldSpec[limitRow]{Name: name, Kind: kind, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v limitRow) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: kind, Scalar: accessor(v)}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: sortable, Groupable: groupable, Identity: identity}); err != nil {
				t.Fatal(err)
			}
		}
		reg("id", FieldIdentifier, true, false, true, func(v limitRow) ScalarValue { return ScalarValue{Kind: FieldIdentifier, String: v.id} })
		reg("status", FieldString, true, true, false, func(v limitRow) ScalarValue { return ScalarValue{Kind: FieldString, String: v.status} })
		reg("score", FieldInt64, true, true, false, func(v limitRow) ScalarValue { return ScalarValue{Kind: FieldInt64, Int64: v.score} })
		if err := q.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseTake, QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		if err := q.SetQueryLimits(limits); err != nil {
			t.Fatal(err)
		}
		q.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[limitRow], error) {
			*loads++
			return QuerySnapshot[limitRow]{Items: []limitRow{{"B", "done", 2}, {"A", "analysis", 1}}}, nil
		})
		return q
	}
	t.Run("sorting_uses_sealed_work_limit", func(t *testing.T) {
		n := 0
		q := makeLimitSchema(t, QueryLimits{MaxWorkUnits: 4}, &n)
		m, _ := ParseQueryModel(`list() sortOrder(score ascending) { id }`, q.QueryLimits())
		_, err := q.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		queryModelCode(t, err, "predicate_cost")
		if n != 1 {
			t.Fatalf("loads=%d", n)
		}
		executedStates["sorting_uses_sealed_work_limit"] = true
	})
	t.Run("grouping_uses_sealed_cardinality_limit", func(t *testing.T) {
		n := 0
		q := makeLimitSchema(t, QueryLimits{MaxGroupCardinality: 1}, &n)
		m, _ := ParseQueryModel(`list() groupBy(status) { id }`, q.QueryLimits())
		_, err := q.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		queryModelCode(t, err, "predicate_limit")
		if n != 1 {
			t.Fatalf("loads=%d", n)
		}
		executedStates["grouping_uses_sealed_cardinality_limit"] = true
	})
	t.Run("pagination_uses_sealed_top_level_limit", func(t *testing.T) {
		n := 0
		q := makeLimitSchema(t, QueryLimits{MaxTopLevelResultsWithoutTake: 1}, &n)
		m, _ := ParseQueryModel(`list() { id }`, q.QueryLimits())
		_, err := q.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		queryModelCode(t, err, "result_limit")
		positive, _ := ParseQueryModel(`list() take 1 { id }`, q.QueryLimits())
		got, err := q.QueryModelAST(context.Background(), positive, queryModelAccess{"public": true})
		if err != nil || len(got.([]map[string]any)) != 1 {
			t.Fatalf("positive=%#v err=%v", got, err)
		}
		executedStates["pagination_uses_sealed_top_level_limit"] = true
	})
	for _, name := range frozen.LimitConfiguration.StateMatrix.DeclaredCases {
		if !executedStates[name] {
			t.Errorf("frozen effective-limit state %q was not executed", name)
		}
	}
	if got, want := fmt.Sprintf("%d/%d", len(executedStates), len(frozen.LimitConfiguration.StateMatrix.DeclaredCases)), frozen.LimitConfiguration.StateMatrix.RequiredExecutedRatio; got != want {
		t.Fatalf("effective-limit state matrix %s want %s", got, want)
	} else {
		t.Logf("effective-limit state matrix %s", got)
	}
}

func TestQueryModelOperationCapabilityAndDiscovery(t *testing.T) {
	frozen := loadFrozenQueryModelContract(t)
	access := queryModelAccess{"public": true}
	executed := 0
	executedRows := map[string]bool{}
	t.Run("O01_full_list", func(t *testing.T) {
		type row struct {
			id, typ, status string
			updated         time.Time
		}
		rows := []row{{"A", "task", "analysis", time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)}, {"B", "task", "development", time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)}, {"F", "task", "development", time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)}, {"C", "task", "done", time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)}, {"D", "story", "development", time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)}}
		loads := 0
		schema := NewSchema[row]()
		schema.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
		reg := func(name string, kind FieldKind, sortable, groupable, identity bool, accessor func(row) ScalarValue) {
			t.Helper()
			if err := schema.RegisterQueryField(QueryFieldSpec[row]{Name: name, Kind: kind, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: kind, Scalar: accessor(v)}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: sortable, Groupable: groupable, Identity: identity}); err != nil {
				t.Fatal(err)
			}
		}
		reg("id", FieldIdentifier, true, false, true, func(v row) ScalarValue { return ScalarValue{Kind: FieldIdentifier, String: v.id} })
		reg("type", FieldString, true, true, false, func(v row) ScalarValue { return ScalarValue{Kind: FieldString, String: v.typ} })
		reg("status", FieldString, true, true, false, func(v row) ScalarValue { return ScalarValue{Kind: FieldString, String: v.status} })
		reg("updated", FieldTimestamp, true, true, false, func(v row) ScalarValue { return ScalarValue{Kind: FieldTimestamp, Timestamp: v.updated} })
		if err := schema.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		schema.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
			loads++
			return QuerySnapshot[row]{Items: rows}, nil
		})
		m, err := ParseQueryModel(`list() where equals(type, "task") sortOrder(updated descending) groupBy(status) skip 1 take 1 { id }`, schema.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		got, err := schema.QueryModelAST(context.Background(), m, access)
		if err != nil {
			t.Fatal(err)
		}
		want := []GroupResult{{Key: GroupKey{Presence: PresencePresent, Value: "development"}, Count: 2, Items: []map[string]any{{"id": "F"}, {"id": "B"}}}}
		if !reflect.DeepEqual(got, want) || loads != 1 {
			t.Fatalf("got %#v loads=%d", got, loads)
		}
		executed++
		executedRows["full-list"] = true
	})

	t.Run("O01_standard_name_list_where_only", func(t *testing.T) {
		loads := 0
		s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
		s.queryCapabilities = make(map[string]QueryOperationCapability)
		if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere}}); err != nil {
			t.Fatal(err)
		}
		m, err := ParseQueryModel(`list() sortOrder(id ascending)`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.QueryModelAST(context.Background(), m, access)
		queryModelCode(t, err, "predicate_unsupported")
		if loads != 0 {
			t.Fatalf("counterfactual list loaded %d snapshots", loads)
		}
		executed++
	})

	loads := 0
	rows := []queryModelRow{{id: "A", status: "analysis"}, {id: "B", status: "done"}, {id: "C", status: "analysis"}}
	s := newQueryModelSchema(t, rows, &loads)
	FilterableField(s, "status", func(v queryModelRow) string { return v.status })
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "count", Clauses: []QueryClause{QueryClauseWhere}}); err != nil {
		t.Fatal(err)
	}
	t.Run("O02_where", func(t *testing.T) {
		m, err := ParseQueryModel(`count() where equals(status, "analysis")`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.QueryModelAST(context.Background(), m, access)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, map[string]any{"count": 2}) {
			t.Fatalf("got %#v", got)
		}
		executed++
		executedRows["where-only-count"] = true
	})
	refusals := []struct {
		name, input, code string
	}{
		{"sort", `count() sortOrder(id ascending)`, "predicate_unsupported"},
		{"group", `count() groupBy(status)`, "predicate_unsupported"},
		{"skip", `count() skip 1`, "predicate_unsupported"},
		{"take", `count() take 1`, "predicate_unsupported"},
		{"projection", `count() { id }`, "predicate_unsupported"},
		{"sort_alias", `count(sort_id=asc)`, "predicate_unsupported"},
		{"skip_alias", `count(skip=1)`, "predicate_unsupported"},
		{"take_alias", `count(take=1)`, "predicate_unsupported"},
		{"unknown", `count() where equals(unknownField, "analysis")`, "predicate_field"},
		{"hidden", `count() where equals(secret, "hidden")`, "predicate_field"},
	}
	for _, tc := range refusals {
		t.Run("O02_"+tc.name, func(t *testing.T) {
			before := loads
			m, err := ParseQueryModel(tc.input, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.QueryModelAST(context.Background(), m, access)
			queryModelCode(t, err, tc.code)
			if loads != before {
				t.Fatalf("%s loaded before refusal", tc.name)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "unknownField") {
				t.Fatalf("catalog-bearing refusal: %v", err)
			}
			executed++
		})
	}
	t.Run("O02_standard_name_count_sort_admitted", func(t *testing.T) {
		counterLoads := 0
		counter := newQueryModelSchema(t, rows, &counterLoads)
		if err := counter.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "count", Clauses: []QueryClause{QueryClauseWhere, QueryClauseSortOrder}}); err != nil {
			t.Fatal(err)
		}
		m, err := ParseQueryModel(`count() sortOrder(id descending)`, counter.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		got, err := counter.QueryModelAST(context.Background(), m, access)
		if err != nil {
			t.Fatalf("registered counterfactual count capability was ignored: %v", err)
		}
		if !reflect.DeepEqual(got, map[string]any{"count": 3}) || counterLoads != 1 {
			t.Fatalf("count got %#v loads=%d", got, counterLoads)
		}
		executed++
	})

	t.Run("O03_custom", func(t *testing.T) {
		customLoads := 0
		custom := newQueryModelSchema(t, rows, &customLoads)
		custom.Operation("customSearch", func(ctx OperationContext[queryModelRow]) (any, error) { return ctx.Items() })
		if err := custom.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "customSearch", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		m, err := ParseQueryModel(`customSearch() where equals(status, "done") { id }`, custom.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		got, err := custom.QueryModelAST(context.Background(), m, access)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []map[string]any{{"id": "B"}}) || customLoads != 1 {
			t.Fatalf("custom got %#v loads=%d", got, customLoads)
		}
		executed++
		executedRows["custom-operation"] = true
	})

	t.Run("O04_unregistered_lifecycle", func(t *testing.T) {
		unregisteredLoads := 0
		unregistered := newQueryModelSchema(t, rows, &unregisteredLoads)
		unregistered.Operation("get", func(ctx OperationContext[queryModelRow]) (any, error) { return ctx.Items() })
		m, err := ParseQueryModel(`get(A) where equals(status, "analysis")`, unregistered.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		_, err = unregistered.QueryModelAST(context.Background(), m, access)
		queryModelCode(t, err, "predicate_unsupported")
		executed++
		executedRows["unregistered-operation"] = true
		for _, attempt := range []QueryResponseAttempt{QueryResponseRetry, QueryResponseResume, QueryResponseRecovery} {
			t.Run(string(attempt), func(t *testing.T) {
				registeredLoads := 0
				registered := newQueryModelSchema(t, rows, &registeredLoads)
				batch, e := ParseQueryModel(`list() { id }; list() { id }`, registered.QueryLimits())
				if e != nil {
					t.Fatal(e)
				}
				request, e := registered.BeginQueryModelResponse(context.Background(), batch, access)
				if e != nil {
					t.Fatal(e)
				}
				beforeCatalog, e := registered.QueryModelSchema(context.Background(), access)
				if e != nil {
					t.Fatal(e)
				}
				state, e := request.QueryModelAST(context.Background(), QueryResponseInitial)
				if e != nil || state != QueryResponsePaused {
					t.Fatalf("pause=(%s,%v)", state, e)
				}
				if _, e = request.QueryModelAST(context.Background(), QueryResponseAttempt("forged")); e == nil {
					t.Fatal("caller-forged continuation attempt admitted")
				}
				state, e = request.QueryModelAST(context.Background(), attempt)
				if e != nil || state != QueryResponseTerminal {
					t.Fatalf("continuation=(%s,%v)", state, e)
				}
				afterCatalog, e := registered.QueryModelSchema(context.Background(), access)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(afterCatalog["operations"], beforeCatalog["operations"]) {
					t.Fatalf("continuation manufactured capability: before=%#v after=%#v", beforeCatalog["operations"], afterCatalog["operations"])
				}
				_, e = registered.QueryModelAST(context.Background(), m, access)
				queryModelCode(t, e, "predicate_unsupported")
				executed++
			})
		}
	})

	t.Run("O05_discovery", func(t *testing.T) {
		discoveryLoads := 0
		discovery := NewSchema[queryModelRow]()
		discovery.Operation("list", func(ctx OperationContext[queryModelRow]) (any, error) { return ctx.Items() })
		allOperators := []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorContains, OperatorMatchesRegex, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorHasElement, OperatorIsNull, OperatorIsMissing}
		fields := []struct {
			name        string
			kind        FieldKind
			elementKind FieldKind
			enumValues  []string
			visibility  string
			sensitivity FieldSensitivity
			sortable    bool
			groupable   bool
			identity    bool
		}{
			{"id", FieldIdentifier, "", nil, "public", FieldSensitivityPublic, true, false, true},
			{"type", FieldEnum, "", []string{"task", "story"}, "public", FieldSensitivityPublic, true, true, false},
			{"name", FieldString, "", nil, "public", FieldSensitivityPublic, true, true, false},
			{"description", FieldString, "", nil, "public", FieldSensitivityPublic, false, false, false},
			{"status", FieldEnum, "", []string{"analysis", "development", "done"}, "public", FieldSensitivityPublic, true, true, false},
			{"assignee", FieldIdentifier, "", nil, "public", FieldSensitivityPublic, true, true, false},
			{"score", FieldInt64, "", nil, "public", FieldSensitivityPublic, true, true, false},
			{"active", FieldBoolean, "", nil, "public", FieldSensitivityPublic, true, true, false},
			{"created", FieldTimestamp, "", nil, "public", FieldSensitivityPublic, true, true, false},
			{"updated", FieldTimestamp, "", nil, "public", FieldSensitivityPublic, true, true, false},
			{"labels", FieldScalarArray, FieldString, nil, "public", FieldSensitivityPublic, false, false, false},
			{"secret", FieldString, "", nil, "restricted", FieldSensitivitySecret, false, false, false},
		}
		for _, field := range fields {
			operators := make([]PredicateOperator, 0)
			for _, operator := range allOperators {
				if operatorAllowed(field.kind, operator) {
					operators = append(operators, operator)
				}
			}
			if err := discovery.RegisterQueryField(QueryFieldSpec[queryModelRow]{Name: field.name, Kind: field.kind, ElementKind: field.elementKind, Operators: operators, EnumValues: field.enumValues, Accessor: func(queryModelRow) (FieldValue, error) {
				return FieldValue{Presence: PresenceMissing, Kind: field.kind}, nil
			}, AccessorCost: 1, Visibility: field.visibility, Sensitivity: field.sensitivity, Sortable: field.sortable, Groupable: field.groupable, Identity: field.identity}); err != nil {
				t.Fatal(err)
			}
		}
		if err := discovery.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		discovery.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[queryModelRow], error) {
			discoveryLoads++
			return QuerySnapshot[queryModelRow]{}, nil
		})
		for _, tc := range []struct {
			name       string
			access     QueryAccess
			fieldNames []string
			wantError  bool
		}{{"normal", queryModelAccess{"public": true}, frozen.DiscoveryShape.NormalVisibleFields, false}, {"restricted", queryModelAccess{"public": true, "restricted": true}, frozen.DiscoveryShape.RestrictedVisibleFields, false}, {"nil", nil, nil, true}} {
			t.Run(tc.name, func(t *testing.T) {
				meta, err := discovery.QueryModelSchema(context.Background(), tc.access)
				if tc.wantError {
					queryModelCode(t, err, "predicate_field")
					if meta != nil {
						t.Fatalf("partial discovery metadata %#v", meta)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					keys := make([]string, 0, len(meta))
					for key := range meta {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					wantKeys := append([]string(nil), frozen.DiscoveryShape.TopLevelKeys...)
					sort.Strings(wantKeys)
					if !reflect.DeepEqual(keys, wantKeys) {
						t.Fatalf("discovery keys=%v want %v", keys, wantKeys)
					}
					if !reflect.DeepEqual(meta["resultShapes"], frozen.ResultShapes) {
						t.Fatalf("resultShapes differ from frozen registry")
					}
					if !reflect.DeepEqual(meta["operations"], []QueryOperationCapability{{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection}}}) {
						t.Fatalf("operations=%#v", meta["operations"])
					}
					visible := meta["fields"].([]map[string]any)
					gotNames := make([]string, len(visible))
					for i, item := range visible {
						gotNames[i] = item["name"].(string)
						if gotNames[i] == "labels" && item["elementKind"] != FieldString {
							t.Fatalf("labels elementKind=%#v", item["elementKind"])
						}
					}
					if !reflect.DeepEqual(gotNames, tc.fieldNames) {
						t.Fatalf("visible fields=%v want %v", gotNames, tc.fieldNames)
					}
				}
				if discoveryLoads != 0 {
					t.Fatalf("discovery loaded %d snapshots", discoveryLoads)
				}
				executed++
			})
		}
		empty := NewSchema[queryModelRow]()
		meta, err := empty.QueryModelSchema(context.Background(), access)
		queryModelCode(t, err, "predicate_unsupported")
		if meta != nil {
			t.Fatalf("empty capability catalog leaked metadata %#v", meta)
		}
		executedRows["authorized-discovery"] = true
	})

	requiredCases := 0
	for name, ratio := range frozen.OperationCapabilityCoverage.CaseInventory {
		var numerator, denominator int
		if _, err := fmt.Sscanf(ratio, "%d/%d", &numerator, &denominator); err != nil || numerator != denominator {
			t.Fatalf("invalid frozen case inventory %s=%q", name, ratio)
		}
		requiredCases += numerator
	}
	for _, row := range frozen.OperationCapabilityCoverage.RequiredRows {
		if !executedRows[row] {
			t.Errorf("frozen operation surface %q was not executed", row)
		}
	}
	if got, want := fmt.Sprintf("%d/%d", len(executedRows), len(frozen.OperationCapabilityCoverage.RequiredRows)), frozen.OperationCapabilityCoverage.RequiredSurfaceRatio; got != want {
		t.Fatalf("operation capability surface coverage %s want %s", got, want)
	}
	if got, want := fmt.Sprintf("%d/%d", executed, requiredCases), frozen.OperationCapabilityCoverage.RequiredCaseRatio; got != want {
		t.Fatalf("operation capability executable coverage %s want %s", got, want)
	}
	t.Logf("operation capability coverage %s surfaces and %d/%d cases", frozen.OperationCapabilityCoverage.RequiredSurfaceRatio, executed, requiredCases)
}

func TestQueryModelCountWhereOnlyCapability(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A", status: "analysis"}, {id: "B", status: "done"}}, &loads)
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "count", Clauses: []QueryClause{QueryClauseWhere}}); err != nil {
		t.Fatal(err)
	}
	access := queryModelAccess{"public": true}
	m, err := ParseQueryModel(`count() where equals(status, "analysis")`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, access)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"count": 1}) {
		t.Fatalf("got %#v", got)
	}
	for _, input := range []string{`count() { id }`, `count(sort_name=asc)`, `count(skip=1)`} {
		before := loads
		m, err := ParseQueryModel(input, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.QueryModelAST(context.Background(), m, access)
		queryModelCode(t, err, "predicate_unsupported")
		if loads != before {
			t.Fatalf("%q loaded before capability refusal", input)
		}
	}
}

func TestQueryModelSecretGroupRegistrationRefusal(t *testing.T) {
	s := NewSchema[queryModelRow]()
	err := s.RegisterQueryField(QueryFieldSpec[queryModelRow]{Name: "secret", Kind: FieldString, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(queryModelRow) (FieldValue, error) { return FieldValue{}, nil }, AccessorCost: 1, Visibility: "restricted", Sensitivity: FieldSensitivitySecret, Groupable: true})
	queryModelCode(t, err, "query_modifier")
	s.Operation("list", func(ctx OperationContext[queryModelRow]) (any, error) { return ctx.Items() })
	if err = s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere}}); err != nil {
		t.Fatal(err)
	}
	meta, err := s.QueryModelSchema(context.Background(), queryModelAccess{"restricted": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta["fields"].([]map[string]any)) != 0 {
		t.Fatal("rejected field entered discovery")
	}
}

func TestQueryModelNumericLexemes(t *testing.T) {
	accepted := []string{`list() where equals(score, -9223372036854775808)`, `list() where equals(score, 9223372036854775807)`, `list() skip 0`, `list() skip 18446744073709551615`}
	for _, input := range accepted {
		if _, err := ParseQueryModel(input, QueryLimits{}); err != nil {
			t.Fatalf("accepted %q: %v", input, err)
		}
	}
	for _, input := range []string{`list() where equals(score, +1)`, `list() where equals(score, -0)`, `list() where equals(score, 01)`, `list() skip -1`, `list() skip 01`} {
		_, err := ParseQueryModel(input, QueryLimits{})
		queryModelCode(t, err, "predicate_syntax")
	}
	_, err := ParseQueryModel(`list() where equals(score, 9223372036854775808)`, QueryLimits{})
	queryModelCode(t, err, "predicate_type")
	_, err = ParseQueryModel(`list() skip 18446744073709551616`, QueryLimits{})
	queryModelCode(t, err, "predicate_limit")
}

func TestQueryModelStrictTimestampContract(t *testing.T) {
	valid := []string{
		"2026-09-01T00:00:00+23:59",
		"2026-09-01T00:00:00+01:59",
		"2026-09-01T00:00:00.1Z",
		"2024-02-29T23:59:59.123456789Z",
	}
	for _, value := range valid {
		t.Run("parse_accepts_"+value, func(t *testing.T) {
			input := `list() where equals(created, timestamp("` + value + `"))`
			if _, err := ParseQueryModel(input, QueryLimits{}); err != nil {
				t.Fatalf("ParseQueryModel rejected valid timestamp %q: %v", value, err)
			}
		})
	}

	invalid := []struct {
		name  string
		value string
	}{
		{"offset_hour_24", "2026-09-01T00:00:00+24:00"},
		{"offset_minute_60", "2026-09-01T00:00:00+01:60"},
		{"comma_fraction", "2026-09-01T00:00:00,1Z"},
		{"lowercase_t", "2026-09-01t00:00:00Z"},
		{"lowercase_z", "2026-09-01T00:00:00z"},
		{"negative_zero_offset", "2026-09-01T00:00:00-00:00"},
		{"leap_second", "2026-09-01T00:00:60Z"},
		{"hour_24", "2026-09-01T24:00:00Z"},
		{"fraction_too_precise", "2026-09-01T00:00:00.1234567890Z"},
		{"invalid_date", "2026-02-29T00:00:00Z"},
	}
	for _, tc := range invalid {
		t.Run("parse_refuses_"+tc.name, func(t *testing.T) {
			input := `list() where equals(created, timestamp("` + tc.value + `"))`
			_, err := ParseQueryModel(input, QueryLimits{})
			queryModelCode(t, err, "predicate_type")
		})
	}

	for _, tc := range invalid {
		t.Run("external_ast_refuses_"+tc.name, func(t *testing.T) {
			type row struct {
				id      string
				created time.Time
			}
			loads := 0
			s := NewSchema[row]()
			s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
			register := func(spec QueryFieldSpec[row]) {
				t.Helper()
				if err := s.RegisterQueryField(spec); err != nil {
					t.Fatal(err)
				}
			}
			register(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: v.id}}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: true, Identity: true})
			register(QueryFieldSpec[row]{Name: "created", Kind: FieldTimestamp, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: FieldTimestamp, Scalar: ScalarValue{Kind: FieldTimestamp, Timestamp: v.created}}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Sortable: true})
			if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere}}); err != nil {
				t.Fatal(err)
			}
			s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
				loads++
				return QuerySnapshot[row]{Items: []row{{id: "A", created: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}, DecodedBytes: 1}, nil
			})
			model := &QueryModel{Statements: []QueryStatement{{
				Call:  Statement{Operation: "list"},
				Where: Equals("created", Literal{Kind: LiteralTimestamp, Text: tc.value}),
			}}}
			got, err := s.QueryModelAST(context.Background(), model, queryModelAccess{"public": true})
			queryModelCode(t, err, "predicate_syntax")
			if got != nil || loads != 0 {
				t.Fatalf("invalid external timestamp reached snapshot: got=%#v loads=%d", got, loads)
			}
		})
	}
	t.Logf("strict timestamp refusal coverage %d/%d through ParseQueryModel and %d/%d through Schema.QueryModelAST", len(invalid), len(invalid), len(invalid), len(invalid))
}

func TestQueryModelRegexByteBoundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		code string
	}{{1024, ""}, {1025, "predicate_limit"}} {
		loads := 0
		s := newQueryModelSchema(t, []queryModelRow{{id: "A", name: "a"}}, &loads)
		m, err := ParseQueryModel(`list() where matchesRegex(name, "`+strings.Repeat("a", tc.n)+`") { id }`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		if tc.code == "" {
			if err != nil {
				t.Fatalf("N=%d: %v", tc.n, err)
			}
			if loads != 1 {
				t.Fatalf("N=%d loads=%d", tc.n, loads)
			}
		} else {
			queryModelCode(t, err, tc.code)
			if loads != 0 {
				t.Fatalf("N=%d loaded before refusal", tc.n)
			}
		}
	}
}

func TestQueryModelLegacyFilterLowersIntoCanonicalEvaluator(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A", status: "analysis"}, {id: "B", status: "done"}, {id: "C", status: "ANALYSIS"}}, &loads)
	FilterableField(s, "status", func(v queryModelRow) string { return v.status })
	m, err := ParseQueryModel(`list(status=ANALYSIS) { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{{"id": "A"}, {"id": "C"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if loads != 1 {
		t.Fatalf("loads=%d", loads)
	}
}

func TestQueryModelLegacyFilterRequiresLegacyRegistration(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A", status: "analysis"}, {id: "B", status: "done"}}, &loads)
	m, err := ParseQueryModel(`list(status=analysis) { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{{"id": "A"}, {"id": "B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unregistered legacy filter was lowered: got %#v want %#v", got, want)
	}
	if loads != 1 {
		t.Fatalf("loads=%d", loads)
	}
}

func TestQueryModelProjectionUsesTypedMaterialization(t *testing.T) {
	type row struct {
		ID, Public, Secret string
	}
	rows := []row{{ID: "A", Public: "raw-public", Secret: "must-not-leak"}}
	s := NewSchema[row]()
	s.Operation("list", func(ctx OperationContext[row]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return items, nil
	})
	register := func(name string, identity bool, accessor func(row) string) {
		t.Helper()
		err := s.RegisterQueryField(QueryFieldSpec[row]{
			Name: name, Kind: FieldString, Operators: []PredicateOperator{OperatorEquals},
			Accessor: func(v row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: accessor(v)}}, nil
			},
			AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: identity,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	register("id", true, func(v row) string { return v.ID })
	register("public", false, func(row) string { return "typed-public" })
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	loads := 0
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
		loads++
		return QuerySnapshot[row]{Items: rows}, nil
	})
	m, err := ParseQueryModel(`list() { public }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{{"public": "typed-public"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical projection escaped typed materialization: got %#v want %#v", got, want)
	}
	if strings.Contains(fmt.Sprintf("%#v", got), "must-not-leak") {
		t.Fatalf("unrequested raw field leaked: %#v", got)
	}
	if loads != 1 {
		t.Fatalf("loads=%d", loads)
	}
}

func TestQueryModelThreeValuedTruth(t *testing.T) {
	type row struct {
		id       string
		assignee FieldValue
	}
	rows := []row{{"A", FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "alice"}}}, {"B", FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "bob"}}}, {"C", FieldValue{Presence: PresenceNull, Kind: FieldIdentifier}}, {"D", FieldValue{Presence: PresenceMissing, Kind: FieldIdentifier}}}
	s := NewSchema[row]()
	s.Field("id", func(v row) any { return v.id })
	s.DefaultFields("id")
	s.Operation("list", func(ctx OperationContext[row]) (any, error) {
		items, _ := ctx.Items()
		out := make([]map[string]any, 0, len(items))
		for _, v := range items {
			out = append(out, ctx.Selector.Apply(v))
		}
		return out, nil
	})
	ops := []PredicateOperator{OperatorEquals, OperatorIsNull, OperatorIsMissing}
	if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: ops, Accessor: func(v row) (FieldValue, error) {
		return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: v.id}}, nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "assignee", Kind: FieldIdentifier, Operators: ops, Accessor: func(v row) (FieldValue, error) { return v.assignee, nil }, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
		return QuerySnapshot[row]{Items: rows}, nil
	})
	access := queryModelAccess{"public": true}
	cases := []struct {
		input string
		ids   []string
	}{{`list() where not(equals(assignee, "alice")) { id }`, []string{"B"}}, {`list() where satisfiesAny(isNull(assignee), isMissing(assignee)) { id }`, []string{"C", "D"}}, {`list() where satisfiesAll(equals(assignee, "alice"), not(isMissing(assignee))) { id }`, []string{"A"}}}
	for _, tc := range cases {
		m, err := ParseQueryModel(tc.input, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		v, err := s.QueryModelAST(context.Background(), m, access)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, x := range v.([]map[string]any) {
			ids = append(ids, x["id"].(string))
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatalf("%s got %v want %v", tc.input, ids, tc.ids)
		}
	}
}

func TestQueryModelExternalTreeBounds(t *testing.T) {
	makeDepth := func(n int) *Expression {
		e := Equals("id", StringValue("A"))
		for i := 1; i < n; i++ {
			e = Not(e)
		}
		return e
	}
	for _, tc := range []struct {
		depth int
		code  string
	}{{16, ""}, {17, "predicate_limit"}} {
		loads := 0
		s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
		m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}}, Where: makeDepth(tc.depth)}}}
		_, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
		if tc.code == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			queryModelCode(t, err, tc.code)
			if loads != 0 {
				t.Fatalf("depth %d loaded", tc.depth)
			}
		}
	}
}

func TestQueryModelExternalASTSealingIsBoundedByExpressionLimit(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
	access := queryModelAccess{"public": true}
	modelWithChildren := func(count int) *QueryModel {
		leaf := Equals("id", StringValue("A"))
		children := make([]*Expression, count)
		for i := range children {
			children[i] = leaf
		}
		return &QueryModel{Statements: []QueryStatement{{
			Call:  Statement{Operation: "list", Fields: []string{"id"}},
			Where: &Expression{Kind: ExpressionSatisfiesAll, Children: children},
		}}}
	}
	// Root plus 128 children is the first rejected expression-node shape.
	limitPlusOne := modelWithChildren(128)
	oversized := modelWithChildren(1_000_000)

	begin := func(model *QueryModel) {
		t.Helper()
		request, err := s.BeginQueryModelResponse(context.Background(), model, access)
		if request != nil {
			t.Fatal("excessive external AST produced a request")
		}
		queryModelCode(t, err, "predicate_limit")
	}
	// Warm runtime and schema one-time paths outside the allocation sample.
	begin(limitPlusOne)
	begin(oversized)
	measure := func(model *QueryModel) uint64 {
		t.Helper()
		const runs = 4
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for i := 0; i < runs; i++ {
			begin(model)
		}
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(model)
		return (after.TotalAlloc - before.TotalAlloc) / runs
	}

	boundaryBytes := measure(limitPlusOne)
	oversizedBytes := measure(oversized)
	// The residual 999,872 pointers are caller-owned and must not determine
	// sealing allocation. The allowance absorbs runtime bookkeeping while a
	// full-child preallocation mutant adds about 8 MiB per invocation.
	const runtimeAllowance = 256 << 10
	if oversizedBytes > boundaryBytes+runtimeAllowance {
		t.Fatalf("external AST sealing allocation follows rejected residual graph: boundary=%d oversized=%d allowance=%d", boundaryBytes, oversizedBytes, runtimeAllowance)
	}
	if loads != 0 {
		t.Fatalf("excessive external AST loaded snapshot %d times", loads)
	}
	t.Logf("bounded external AST sealing 2/2 rejected shapes through Schema.BeginQueryModelResponse: limit+1=%dB oversized=%dB", boundaryBytes, oversizedBytes)
}

func TestQueryModelNativeModifierRuntime(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A", name: "a"}}, &loads)
	m, err := ParseQueryModel(`list() sortOrder(name ascending) { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil || !reflect.DeepEqual(got, []map[string]any{{"id": "A"}}) || loads != 1 {
		t.Fatalf("got=%#v err=%v loads=%d", got, err, loads)
	}
}

func TestQueryModelLegacyModifierAliasesUseNativePipeline(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "B", name: "b"}, {id: "A", name: "a"}}, &loads)
	m, err := ParseQueryModel(`list(sort_name=asc, skip=0, take=1) { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []map[string]any{{"id": "A"}}) || loads != 1 {
		t.Fatalf("got=%#v loads=%d", got, loads)
	}
}

func TestQueryModelDuplicatePagingAliasesRefusedBeforeLoad(t *testing.T) {
	for _, input := range []string{`list(skip=0,skip=1) { id }`, `list(take=1,take=2) { id }`} {
		t.Run(input, func(t *testing.T) {
			loads := 0
			s := newQueryModelSchema(t, []queryModelRow{{id: "A"}, {id: "B"}}, &loads)
			m, err := ParseQueryModel(input, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
			queryModelCode(t, err, "query_modifier")
			if got != nil || loads != 0 {
				t.Fatalf("partial=%#v loads=%d", got, loads)
			}
		})
	}
}

func TestQueryModelAllPublicLeafOperators(t *testing.T) {
	type row struct {
		id     string
		values map[string]FieldValue
	}
	instant := time.Date(2026, 9, 2, 3, 4, 5, 0, time.UTC)
	r := row{id: "A", values: map[string]FieldValue{
		"text": {Presence: PresencePresent, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: "alpha"}}, "enum": {Presence: PresencePresent, Kind: FieldEnum, Scalar: ScalarValue{Kind: FieldEnum, String: "dev"}}, "num": {Presence: PresencePresent, Kind: FieldInt64, Scalar: ScalarValue{Kind: FieldInt64, Int64: 5}}, "flag": {Presence: PresencePresent, Kind: FieldBoolean, Scalar: ScalarValue{Kind: FieldBoolean, Boolean: true}}, "when": {Presence: PresencePresent, Kind: FieldTimestamp, Scalar: ScalarValue{Kind: FieldTimestamp, Timestamp: instant}}, "tags": {Presence: PresencePresent, Kind: FieldScalarArray, Elements: []ScalarValue{{Kind: FieldString, String: "x"}, {Kind: FieldString, String: "y"}}}, "nullable": {Presence: PresenceNull, Kind: FieldString}, "missing": {Presence: PresenceMissing, Kind: FieldString}}}
	s := NewSchema[row]()
	s.Field("id", func(v row) any { return v.id })
	s.DefaultFields("id")
	s.Operation("list", func(ctx OperationContext[row]) (any, error) {
		items, _ := ctx.Items()
		out := make([]map[string]any, 0, len(items))
		for _, v := range items {
			out = append(out, ctx.Selector.Apply(v))
		}
		return out, nil
	})
	reg := func(name string, kind, element FieldKind, ops []PredicateOperator, enum []string, identity bool) {
		err := s.RegisterQueryField(QueryFieldSpec[row]{Name: name, Kind: kind, ElementKind: element, Operators: ops, EnumValues: enum, Accessor: func(v row) (FieldValue, error) {
			if identity {
				return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: v.id}}, nil
			}
			return v.values[name], nil
		}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: identity})
		if err != nil {
			t.Fatal(err)
		}
	}
	presence := []PredicateOperator{OperatorIsNull, OperatorIsMissing}
	reg("id", FieldIdentifier, "", append([]PredicateOperator{OperatorEquals}, presence...), nil, true)
	reg("text", FieldString, "", []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorContains, OperatorMatchesRegex, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorIsNull, OperatorIsMissing}, nil, false)
	reg("enum", FieldEnum, "", []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorInSet, OperatorIsNull, OperatorIsMissing}, []string{"dev", "done"}, false)
	reg("num", FieldInt64, "", []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorIsNull, OperatorIsMissing}, nil, false)
	reg("flag", FieldBoolean, "", []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorInSet, OperatorIsNull, OperatorIsMissing}, nil, false)
	reg("when", FieldTimestamp, "", []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorIsNull, OperatorIsMissing}, nil, false)
	reg("tags", FieldScalarArray, FieldString, []PredicateOperator{OperatorHasElement, OperatorIsNull, OperatorIsMissing}, nil, false)
	reg("nullable", FieldString, "", presence, nil, false)
	reg("missing", FieldString, "", presence, nil, false)
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
		return QuerySnapshot[row]{Items: []row{r}}, nil
	})
	queries := map[string]string{"equals": `equals(text, "alpha")`, "notEquals": `notEquals(text, "beta")`, "contains": `contains(text, "ph")`, "matchesRegex": `matchesRegex(text, "^a.*a$")`, "lessThan": `lessThan(num, 6)`, "lessThanOrEqual": `lessThanOrEqual(num, 5)`, "greaterThan": `greaterThan(num, 4)`, "greaterThanOrEqual": `greaterThanOrEqual(num, 5)`, "inRange": `inRange(num, 5, 6)`, "inSet": `inSet(enum, ["done", "dev"])`, "hasElement": `hasElement(tags, "y")`, "isNull": `isNull(nullable)`, "isMissing": `isMissing(missing)`}
	for name, expression := range queries {
		t.Run(name, func(t *testing.T) {
			m, err := ParseQueryModel(`list() where `+expression+` { id }`, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.([]map[string]any)) != 1 {
				t.Fatalf("got %#v", got)
			}
		})
	}
}

func TestQueryModelFrozenTypeOperatorMatrix(t *testing.T) {
	var contract struct {
		FieldKinds map[string][]string `json:"fieldKinds"`
	}
	data, err := os.ReadFile(filepath.Join("..", ".spec", "composable-query-expression-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	allOperators := []PredicateOperator{OperatorEquals, OperatorNotEquals, OperatorContains, OperatorMatchesRegex, OperatorLessThan, OperatorLessThanOrEqual, OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorInRange, OperatorInSet, OperatorHasElement, OperatorIsNull, OperatorIsMissing}
	kinds := map[string]FieldKind{"string": FieldString, "identifier": FieldIdentifier, "enum": FieldEnum, "int64": FieldInt64, "boolean": FieldBoolean, "timestamp": FieldTimestamp, "scalarArray": FieldScalarArray}
	allowedRows := 0
	for kindName, operatorNames := range contract.FieldKinds {
		kind := kinds[kindName]
		allowed := make(map[PredicateOperator]bool, len(operatorNames))
		for _, operatorName := range operatorNames {
			operator := PredicateOperator(operatorName)
			allowed[operator] = true
			allowedRows++
			t.Run(kindName+"/"+operatorName, func(t *testing.T) {
				runFrozenTypeOperatorCase(t, kind, operator)
			})
		}
		for _, operator := range allOperators {
			if allowed[operator] {
				continue
			}
			t.Run(kindName+"/refuses_"+string(operator), func(t *testing.T) {
				loads := 0
				s := newMatrixSchema(t, kind, allowedOperators(operatorNames), PresencePresent, &loads)
				m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}}, Where: matrixExpression(kind, operator)}}}
				_, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
				queryModelCode(t, err, "predicate_operator")
				if loads != 0 {
					t.Fatalf("forbidden %s/%s loaded %d snapshots", kindName, operator, loads)
				}
			})
		}
		t.Run(kindName+"/refuses_wrong_literal_type", func(t *testing.T) {
			loads := 0
			operator := OperatorEquals
			wrong := StringValue("wrong")
			if kind == FieldString || kind == FieldIdentifier || kind == FieldEnum {
				wrong = Int64Value(1)
			}
			if kind == FieldScalarArray {
				operator = OperatorHasElement
				wrong = Int64Value(1)
			}
			s := newMatrixSchema(t, kind, []PredicateOperator{operator}, PresencePresent, &loads)
			m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}}, Where: leaf("value", operator, &wrong)}}}
			_, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
			queryModelCode(t, err, "predicate_type")
			if loads != 0 {
				t.Fatalf("wrong literal for %s loaded %d snapshots", kindName, loads)
			}
		})
	}
	if allowedRows != 57 {
		t.Fatalf("frozen type/operator coverage %d/57", allowedRows)
	}
	t.Logf("frozen type/operator coverage %d/57", allowedRows)
}

func TestQueryModelThreeValuedTruthTables(t *testing.T) {
	type row struct {
		id    string
		left  FieldValue
		right FieldValue
	}
	truthValue := func(code byte) FieldValue {
		if code == 'U' {
			return FieldValue{Presence: PresenceNull, Kind: FieldBoolean}
		}
		return FieldValue{Presence: PresencePresent, Kind: FieldBoolean, Scalar: ScalarValue{Kind: FieldBoolean, Boolean: code == 'T'}}
	}
	var rows []row
	for _, left := range []byte{'T', 'F', 'U'} {
		for _, right := range []byte{'T', 'F', 'U'} {
			rows = append(rows, row{id: string([]byte{left, right}), left: truthValue(left), right: truthValue(right)})
		}
	}
	s := NewSchema[row]()
	s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
	register := func(name string, identity bool, accessor func(row) FieldValue) {
		t.Helper()
		kind := FieldBoolean
		if identity {
			kind = FieldIdentifier
		}
		if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: name, Kind: kind, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) { return accessor(v), nil }, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: identity}); err != nil {
			t.Fatal(err)
		}
	}
	register("id", true, func(v row) FieldValue {
		return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: v.id}}
	})
	register("left", false, func(v row) FieldValue { return v.left })
	register("right", false, func(v row) FieldValue { return v.right })
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
		return QuerySnapshot[row]{Items: rows}, nil
	})
	left := Equals("left", BooleanValue(true))
	right := Equals("right", BooleanValue(true))
	cases := []struct {
		name string
		expr *Expression
		want []string
	}{
		{"and_true", SatisfiesAll(left, right), []string{"TT"}},
		{"and_false", Not(SatisfiesAll(left, right)), []string{"FF", "FT", "FU", "TF", "UF"}},
		{"or_true", SatisfiesAny(left, right), []string{"FT", "TF", "TT", "TU", "UT"}},
		{"or_false", Not(SatisfiesAny(left, right)), []string{"FF"}},
		{"not", Not(left), []string{"FF", "FT", "FU"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}}, Where: tc.expr}}}
			got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, item := range got.([]map[string]any) {
				ids = append(ids, item["id"].(string))
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("got %v want %v", ids, tc.want)
			}
		})
	}
}

func allowedOperators(names []string) []PredicateOperator {
	out := make([]PredicateOperator, len(names))
	for i, name := range names {
		out[i] = PredicateOperator(name)
	}
	return out
}

func runFrozenTypeOperatorCase(t *testing.T, kind FieldKind, operator PredicateOperator) {
	t.Helper()
	presence := PresencePresent
	if operator == OperatorIsNull {
		presence = PresenceNull
	} else if operator == OperatorIsMissing {
		presence = PresenceMissing
	}
	loads := 0
	s := newMatrixSchema(t, kind, []PredicateOperator{operator}, presence, &loads)
	m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}}, Where: matrixExpression(kind, operator)}}}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.([]map[string]any)) != 1 || loads != 1 {
		t.Fatalf("operator %s kind %s got %#v loads=%d", operator, kind, got, loads)
	}
}

func newMatrixSchema(t *testing.T, kind FieldKind, operators []PredicateOperator, presence Presence, loads *int) *Schema[int] {
	t.Helper()
	s := NewSchema[int]()
	s.Operation("list", func(ctx OperationContext[int]) (any, error) { return ctx.Items() })
	if err := s.RegisterQueryField(QueryFieldSpec[int]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(int) (FieldValue, error) {
		return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "A"}}, nil
	}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
		t.Fatal(err)
	}
	spec := QueryFieldSpec[int]{Name: "value", Kind: kind, Operators: operators, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic}
	if kind == FieldEnum {
		spec.EnumValues = []string{"a", "b", "c"}
	}
	if kind == FieldScalarArray {
		spec.ElementKind = FieldString
	}
	spec.Accessor = func(int) (FieldValue, error) {
		value := FieldValue{Presence: presence, Kind: kind}
		if presence != PresencePresent {
			return value, nil
		}
		switch kind {
		case FieldString, FieldIdentifier, FieldEnum:
			value.Scalar = ScalarValue{Kind: kind, String: "b"}
		case FieldInt64:
			value.Scalar = ScalarValue{Kind: kind, Int64: 2}
		case FieldBoolean:
			value.Scalar = ScalarValue{Kind: kind, Boolean: true}
		case FieldTimestamp:
			value.Scalar = ScalarValue{Kind: kind, Timestamp: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
		case FieldScalarArray:
			value.Elements = []ScalarValue{{Kind: FieldString, String: "b"}}
		}
		return value, nil
	}
	if err := s.RegisterQueryField(spec); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseWhere, QueryClauseProjection}}); err != nil {
		t.Fatal(err)
	}
	s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[int], error) {
		*loads++
		return QuerySnapshot[int]{Items: []int{1}}, nil
	})
	return s
}

func matrixExpression(kind FieldKind, operator PredicateOperator) *Expression {
	var a, b, c Literal
	switch kind {
	case FieldInt64:
		a, b, c = Int64Value(1), Int64Value(2), Int64Value(3)
	case FieldBoolean:
		a, b, c = BooleanValue(false), BooleanValue(true), BooleanValue(true)
	case FieldTimestamp:
		a = TimestampValue(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
		b = TimestampValue(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
		c = TimestampValue(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	default:
		a, b, c = StringValue("a"), StringValue("b"), StringValue("c")
	}
	switch operator {
	case OperatorIsNull:
		return IsNull("value")
	case OperatorIsMissing:
		return IsMissing("value")
	case OperatorInRange:
		return InRange("value", b, c)
	case OperatorInSet:
		return InSet("value", b)
	case OperatorNotEquals:
		return NotEquals("value", a)
	case OperatorContains:
		return Contains("value", b)
	case OperatorMatchesRegex:
		return MatchesRegex("value", StringValue("^b$"))
	case OperatorLessThan:
		return LessThan("value", c)
	case OperatorLessThanOrEqual:
		return LessThanOrEqual("value", b)
	case OperatorGreaterThan:
		return GreaterThan("value", a)
	case OperatorGreaterThanOrEqual:
		return GreaterThanOrEqual("value", b)
	case OperatorHasElement:
		return HasElement("value", b)
	default:
		return Equals("value", b)
	}
}

func TestQueryModelOpaqueResponseLifecycle(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
	m, err := ParseQueryModel(`list() { id }; list() where equals(id, "A") { id }`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	access := queryModelAccess{"public": true}
	for _, attempt := range []QueryResponseAttempt{QueryResponseRetry, QueryResponseAttempt("forged")} {
		r, beginErr := s.BeginQueryModelResponse(context.Background(), m, access)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		if _, dispatchErr := r.QueryModelAST(context.Background(), attempt); dispatchErr == nil {
			t.Fatalf("fresh %q attempt admitted", attempt)
		}
	}
	r, err := s.BeginQueryModelResponse(context.Background(), m, access)
	if err != nil {
		t.Fatal(err)
	}
	state, err := r.QueryModelAST(context.Background(), QueryResponseInitial)
	if err != nil || state != QueryResponsePaused {
		t.Fatalf("initial=(%s,%v)", state, err)
	}
	if _, err = r.PublishQueryModelAST(); err == nil {
		t.Fatal("publication while paused admitted")
	}
	if _, err = r.QueryModelAST(context.Background(), QueryResponseAttempt("forged")); err == nil {
		t.Fatal("forged paused continuation admitted")
	}
	state, err = r.QueryModelAST(context.Background(), QueryResponseResume)
	if err != nil || state != QueryResponseTerminal {
		t.Fatalf("resume=(%s,%v)", state, err)
	}
	if _, err = r.QueryModelAST(context.Background(), QueryResponseAttempt("forged")); err == nil {
		t.Fatal("forged terminal continuation admitted")
	}
	published, err := r.PublishQueryModelAST()
	if err != nil {
		t.Fatal(err)
	}
	if len(published.([]any)) != 2 {
		t.Fatalf("published %#v", published)
	}
	if _, err = r.PublishQueryModelAST(); err == nil {
		t.Fatal("second publication admitted")
	}
	if _, err = r.QueryModelAST(context.Background(), QueryResponseRecovery); err == nil {
		t.Fatal("continuation after publication admitted")
	}
	if loads != 2 {
		t.Fatalf("loads=%d want 2", loads)
	}
}

func TestQueryModelOpaqueResponseAttemptSet(t *testing.T) {
	access := queryModelAccess{"public": true}
	executed := 0
	t.Run("initial_terminal", func(t *testing.T) {
		loads := 0
		s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
		m, err := ParseQueryModel(`list() { id }`, s.QueryLimits())
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.BeginQueryModelResponse(context.Background(), m, access)
		if err != nil {
			t.Fatal(err)
		}
		state, err := r.QueryModelAST(context.Background(), QueryResponseInitial)
		if err != nil || state != QueryResponseTerminal {
			t.Fatalf("initial=(%s,%v)", state, err)
		}
		if _, err = r.PublishQueryModelAST(); err != nil {
			t.Fatal(err)
		}
		executed++
	})
	for _, attempt := range []QueryResponseAttempt{QueryResponseRetry, QueryResponseRenewal, QueryResponseResume, QueryResponseRecovery} {
		t.Run(string(attempt), func(t *testing.T) {
			loads := 0
			s := newQueryModelSchema(t, []queryModelRow{{id: "A"}}, &loads)
			m, err := ParseQueryModel(`list() { id }; list() { id }`, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			r, err := s.BeginQueryModelResponse(context.Background(), m, access)
			if err != nil {
				t.Fatal(err)
			}
			state, err := r.QueryModelAST(context.Background(), QueryResponseInitial)
			if err != nil || state != QueryResponsePaused {
				t.Fatalf("initial=(%s,%v)", state, err)
			}
			state, err = r.QueryModelAST(context.Background(), attempt)
			if err != nil || state != QueryResponseTerminal {
				t.Fatalf("%s=(%s,%v)", attempt, state, err)
			}
			if _, err = r.PublishQueryModelAST(); err != nil {
				t.Fatal(err)
			}
			executed++
		})
	}
	if executed != 5 {
		t.Fatalf("opaque response attempt coverage %d/5", executed)
	}
	t.Logf("opaque response attempt coverage %d/5", executed)
}

func TestQueryModelFieldValueTaggedUnionRefusal(t *testing.T) {
	cases := []struct {
		name  string
		value FieldValue
	}{
		{"null_scalar", FieldValue{Presence: PresenceNull, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: "x"}}},
		{"null_elements", FieldValue{Presence: PresenceNull, Kind: FieldString, Elements: []ScalarValue{{Kind: FieldString, String: "x"}}}},
		{"missing_scalar", FieldValue{Presence: PresenceMissing, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: "x"}}},
		{"missing_elements", FieldValue{Presence: PresenceMissing, Kind: FieldString, Elements: []ScalarValue{{Kind: FieldString, String: "x"}}}},
		{"present_scalar_elements", FieldValue{Presence: PresencePresent, Kind: FieldString, Scalar: ScalarValue{Kind: FieldString, String: "x"}, Elements: []ScalarValue{{Kind: FieldString, String: "x"}}}},
		{"present_array_scalar", FieldValue{Presence: PresencePresent, Kind: FieldScalarArray, Scalar: ScalarValue{Kind: FieldString, String: "x"}, Elements: []ScalarValue{{Kind: FieldString, String: "x"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			type row struct{}
			s := NewSchema[row]()
			s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
			if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "A"}}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
				t.Fatal(err)
			}
			kind, element := FieldString, FieldKind("")
			if tc.name == "present_array_scalar" {
				kind, element = FieldScalarArray, FieldString
			}
			if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "value", Kind: kind, ElementKind: element, Operators: []PredicateOperator{OperatorIsNull}, Accessor: func(row) (FieldValue, error) { return tc.value, nil }, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseProjection}}); err != nil {
				t.Fatal(err)
			}
			s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
				return QuerySnapshot[row]{Items: []row{{}}}, nil
			})
			m := &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"value"}}}}}
			got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
			queryModelCode(t, err, "predicate_data")
			if got != nil {
				t.Fatalf("partial result %#v", got)
			}
		})
	}
	t.Logf("tagged-union refusal coverage %d/%d", len(cases), 6)

	type scalarCase struct {
		name  string
		kind  FieldKind
		value ScalarValue
		enums []string
	}
	instant := time.Date(2026, 9, 24, 1, 2, 3, 4, time.UTC)
	scalarCases := []scalarCase{
		{name: "string", kind: FieldString, value: ScalarValue{Kind: FieldString, String: "text"}},
		{name: "identifier", kind: FieldIdentifier, value: ScalarValue{Kind: FieldIdentifier, String: "A"}},
		{name: "enum", kind: FieldEnum, value: ScalarValue{Kind: FieldEnum, String: "open"}, enums: []string{"open", "closed"}},
		{name: "int64", kind: FieldInt64, value: ScalarValue{Kind: FieldInt64}},
		{name: "boolean", kind: FieldBoolean, value: ScalarValue{Kind: FieldBoolean}},
		{name: "timestamp", kind: FieldTimestamp, value: ScalarValue{Kind: FieldTimestamp, Timestamp: instant}},
	}
	corruptions := []struct {
		name  string
		field string
		apply func(*ScalarValue)
	}{
		{name: "string", field: "string", apply: func(v *ScalarValue) { v.String = "inactive" }},
		{name: "int64", field: "int64", apply: func(v *ScalarValue) { v.Int64 = 1 }},
		{name: "boolean", field: "boolean", apply: func(v *ScalarValue) { v.Boolean = true }},
		{name: "timestamp", field: "timestamp", apply: func(v *ScalarValue) { v.Timestamp = instant.Add(time.Second) }},
	}
	activeField := func(kind FieldKind) string {
		switch kind {
		case FieldString, FieldIdentifier, FieldEnum:
			return "string"
		case FieldInt64:
			return "int64"
		case FieldBoolean:
			return "boolean"
		case FieldTimestamp:
			return "timestamp"
		default:
			return ""
		}
	}
	runValue := func(t *testing.T, kind, element FieldKind, enums []string, value FieldValue, wantCode string) {
		t.Helper()
		type row struct{}
		s := NewSchema[row]()
		s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
		if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(row) (FieldValue, error) {
			return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "A"}}, nil
		}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "value", Kind: kind, ElementKind: element, Operators: []PredicateOperator{OperatorIsNull}, EnumValues: enums, Accessor: func(row) (FieldValue, error) {
			return value, nil
		}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic}); err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
			return QuerySnapshot[row]{Items: []row{{}}}, nil
		})
		got, err := s.QueryModelAST(context.Background(), &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"value"}}}}}, queryModelAccess{"public": true})
		if wantCode != "" {
			queryModelCode(t, err, wantCode)
			if got != nil {
				t.Fatalf("partial result %#v", got)
			}
			return
		}
		if err != nil || got == nil {
			t.Fatalf("valid tagged scalar got=%#v err=%v", got, err)
		}
	}

	invalidInner := 0
	validInner := 0
	for _, scalar := range scalarCases {
		scalar := scalar
		t.Run("inner/valid/scalar/"+scalar.name, func(t *testing.T) {
			runValue(t, scalar.kind, "", scalar.enums, FieldValue{Presence: PresencePresent, Kind: scalar.kind, Scalar: scalar.value}, "")
		})
		validInner++
		t.Run("inner/valid/array/"+scalar.name, func(t *testing.T) {
			runValue(t, FieldScalarArray, scalar.kind, scalar.enums, FieldValue{Presence: PresencePresent, Kind: FieldScalarArray, Elements: []ScalarValue{scalar.value}}, "")
		})
		validInner++
		for _, corruption := range corruptions {
			corruption := corruption
			if corruption.field == activeField(scalar.kind) {
				continue
			}
			invalid := scalar.value
			corruption.apply(&invalid)
			t.Run("inner/refuse/scalar/"+scalar.name+"_with_"+corruption.name, func(t *testing.T) {
				runValue(t, scalar.kind, "", scalar.enums, FieldValue{Presence: PresencePresent, Kind: scalar.kind, Scalar: invalid}, "predicate_data")
			})
			invalidInner++
			t.Run("inner/refuse/array/"+scalar.name+"_with_"+corruption.name, func(t *testing.T) {
				runValue(t, FieldScalarArray, scalar.kind, scalar.enums, FieldValue{Presence: PresencePresent, Kind: FieldScalarArray, Elements: []ScalarValue{invalid}}, "predicate_data")
			})
			invalidInner++
		}
	}

	t.Run("inner/refuse/duplicate_identity_inactive_payload", func(t *testing.T) {
		type row struct{ inactive int64 }
		s := NewSchema[row]()
		s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
		if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) {
			return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "A", Int64: v.inactive}}, nil
		}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
			return QuerySnapshot[row]{Items: []row{{inactive: 1}, {inactive: 2}}}, nil
		})
		got, err := s.QueryModelAST(context.Background(), &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"id"}}}}}, queryModelAccess{"public": true})
		queryModelCode(t, err, "predicate_data")
		if got != nil {
			t.Fatalf("partial result %#v", got)
		}
	})
	invalidInner++
	if invalidInner != 37 || validInner != 12 {
		t.Fatalf("inner tagged-union inventory invalid=%d/37 valid=%d/12", invalidInner, validInner)
	}
	t.Logf("inner tagged-union production coverage invalid=%d/37 valid=%d/12", invalidInner, validInner)
}

func TestQueryModelStoredEnumDomainAndScalarIdentityRefusal(t *testing.T) {
	t.Run("stored_enum_domain", func(t *testing.T) {
		type row struct {
			id    string
			state string
			tags  []string
		}
		rows := []row{{id: "A", state: "rogue", tags: []string{"analysis", "rogue"}}}
		newSchema := func(t *testing.T, loads *int) *Schema[row] {
			t.Helper()
			s := NewSchema[row]()
			s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
			if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(v row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: v.id}}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "state", Kind: FieldEnum, Operators: []PredicateOperator{OperatorEquals}, EnumValues: []string{"analysis", "done"}, Accessor: func(v row) (FieldValue, error) {
				return FieldValue{Presence: PresencePresent, Kind: FieldEnum, Scalar: ScalarValue{Kind: FieldEnum, String: v.state}}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Groupable: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "tags", Kind: FieldScalarArray, ElementKind: FieldEnum, Operators: []PredicateOperator{OperatorHasElement}, EnumValues: []string{"analysis", "done"}, Accessor: func(v row) (FieldValue, error) {
				elements := make([]ScalarValue, 0, len(v.tags))
				for _, tag := range v.tags {
					elements = append(elements, ScalarValue{Kind: FieldEnum, String: tag})
				}
				return FieldValue{Presence: PresencePresent, Kind: FieldScalarArray, Elements: elements}, nil
			}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseGroupBy, QueryClauseProjection}}); err != nil {
				t.Fatal(err)
			}
			s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
				*loads++
				return QuerySnapshot[row]{Items: rows}, nil
			})
			return s
		}

		cases := []struct {
			name  string
			model *QueryModel
		}{
			{
				name: "scalar_projection_and_grouping",
				model: &QueryModel{Statements: []QueryStatement{{
					Call:    Statement{Operation: "list", Fields: []string{"id", "state"}},
					GroupBy: &GroupCriterion{Field: "state"},
				}}},
			},
			{
				name: "enum_array_materialization",
				model: &QueryModel{Statements: []QueryStatement{{
					Call: Statement{Operation: "list", Fields: []string{"tags"}},
				}}},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				loads := 0
				s := newSchema(t, &loads)
				got, err := s.QueryModelAST(context.Background(), tc.model, queryModelAccess{"public": true})
				queryModelCode(t, err, "predicate_data")
				if got != nil || loads != 1 {
					t.Fatalf("got=%#v loads=%d, want atomic refusal after one bounded load", got, loads)
				}
			})
		}
		t.Logf("stored enum canonical-domain refusal coverage %d/%d", len(cases), 2)
	})

	t.Run("scalar_identity_registration", func(t *testing.T) {
		type row struct{ ids []string }
		loads := 0
		s := NewSchema[row]()
		s.Operation("list", func(ctx OperationContext[row]) (any, error) { return ctx.Items() })
		err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "ids", Kind: FieldScalarArray, ElementKind: FieldIdentifier, Operators: []PredicateOperator{OperatorHasElement}, Accessor: func(v row) (FieldValue, error) {
			elements := make([]ScalarValue, 0, len(v.ids))
			for _, id := range v.ids {
				elements = append(elements, ScalarValue{Kind: FieldIdentifier, String: id})
			}
			return FieldValue{Presence: PresencePresent, Kind: FieldScalarArray, Elements: elements}, nil
		}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true})
		queryModelCode(t, err, "predicate_type")

		if err := s.RegisterQueryField(QueryFieldSpec[row]{Name: "id", Kind: FieldIdentifier, Operators: []PredicateOperator{OperatorEquals}, Accessor: func(row) (FieldValue, error) {
			return FieldValue{Presence: PresencePresent, Kind: FieldIdentifier, Scalar: ScalarValue{Kind: FieldIdentifier, String: "A"}}, nil
		}, AccessorCost: 1, Visibility: "public", Sensitivity: FieldSensitivityPublic, Identity: true}); err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "list", Clauses: []QueryClause{QueryClauseProjection}}); err != nil {
			t.Fatal(err)
		}
		s.SetQuerySnapshotLoader(func(context.Context, SnapshotLimits) (QuerySnapshot[row], error) {
			loads++
			return QuerySnapshot[row]{Items: []row{{ids: []string{"A"}}}}, nil
		})
		got, err := s.QueryModelAST(context.Background(), &QueryModel{Statements: []QueryStatement{{Call: Statement{Operation: "list", Fields: []string{"ids"}}}}}, queryModelAccess{"public": true})
		queryModelCode(t, err, "predicate_field")
		if got != nil || loads != 0 {
			t.Fatalf("got=%#v loads=%d, want catalog refusal before load", got, loads)
		}
		t.Log("scalar identity registration refusal coverage 2/2 public surfaces")
	})
}

func TestQueryModelCustomCapabilityAndRestrictedDiscovery(t *testing.T) {
	loads := 0
	s := newQueryModelSchema(t, []queryModelRow{{id: "A", secret: "x"}}, &loads)
	s.Operation("custom", func(ctx OperationContext[queryModelRow]) (any, error) {
		items, err := ctx.Items()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(items)}, nil
	})
	if err := s.RegisterQueryOperationCapability(QueryOperationCapability{Operation: "custom", Clauses: []QueryClause{QueryClauseWhere}}); err != nil {
		t.Fatal(err)
	}
	m, err := ParseQueryModel(`custom() where equals(id, "A")`, s.QueryLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryModelAST(context.Background(), m, queryModelAccess{"public": true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"count": 1}) {
		t.Fatalf("got %#v", got)
	}
	meta, err := s.QueryModelSchema(context.Background(), queryModelAccess{"public": true, "restricted": true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range meta["fields"].([]map[string]any) {
		if f["name"] == "secret" {
			found = true
			if f["groupable"].(bool) {
				t.Fatal("secret became groupable")
			}
		}
	}
	if !found {
		t.Fatal("authorized restricted field absent")
	}
}
