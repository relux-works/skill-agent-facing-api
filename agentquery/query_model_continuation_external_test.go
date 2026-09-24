package agentquery_test

import (
	"context"
	"testing"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
)

type publicQueryAccess map[string]bool

func (a publicQueryAccess) AllowsVisibility(label string) bool { return a[label] }

func TestQueryModelContinuationUsesSealedExecutionDependencies(t *testing.T) {
	type row struct{ id string }
	attempts := []agentquery.QueryResponseAttempt{
		agentquery.QueryResponseRetry,
		agentquery.QueryResponseRenewal,
		agentquery.QueryResponseResume,
		agentquery.QueryResponseRecovery,
	}
	executed := 0
	for _, attempt := range attempts {
		t.Run(string(attempt), func(t *testing.T) {
			originalLoads, originalHandlers := 0, 0
			replacementLoads, replacementHandlers := 0, 0
			s := agentquery.NewSchema[row]()
			s.Operation("list", func(ctx agentquery.OperationContext[row]) (any, error) {
				originalHandlers++
				items, err := ctx.Items()
				if err != nil {
					return nil, err
				}
				return items[0].id + "/original-handler", nil
			})
			if err := s.RegisterQueryField(agentquery.QueryFieldSpec[row]{
				Name: "id", Kind: agentquery.FieldIdentifier, Operators: []agentquery.PredicateOperator{agentquery.OperatorEquals},
				Accessor: func(v row) (agentquery.FieldValue, error) {
					return agentquery.FieldValue{Presence: agentquery.PresencePresent, Kind: agentquery.FieldIdentifier, Scalar: agentquery.ScalarValue{Kind: agentquery.FieldIdentifier, String: v.id}}, nil
				},
				AccessorCost: 1, Visibility: "public", Sensitivity: agentquery.FieldSensitivityPublic, Identity: true,
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryOperationCapability(agentquery.QueryOperationCapability{Operation: "list", Clauses: []agentquery.QueryClause{agentquery.QueryClauseProjection}}); err != nil {
				t.Fatal(err)
			}
			s.SetQuerySnapshotLoader(func(context.Context, agentquery.SnapshotLimits) (agentquery.QuerySnapshot[row], error) {
				originalLoads++
				return agentquery.QuerySnapshot[row]{Items: []row{{id: "original-loader"}}}, nil
			})
			m, err := agentquery.ParseQueryModel(`list(); list()`, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			request, err := s.BeginQueryModelResponse(context.Background(), m, publicQueryAccess{"public": true})
			if err != nil {
				t.Fatal(err)
			}
			state, err := request.QueryModelJSONASTWithMode(context.Background(), agentquery.QueryResponseInitial, agentquery.HumanReadable)
			if err != nil || state != agentquery.QueryResponsePaused {
				t.Fatalf("initial state=%s err=%v", state, err)
			}

			// Public compatibility setters may configure later requests, but they
			// must not replace dependencies captured by this opaque request.
			s.SetQuerySnapshotLoader(func(context.Context, agentquery.SnapshotLimits) (agentquery.QuerySnapshot[row], error) {
				replacementLoads++
				return agentquery.QuerySnapshot[row]{Items: []row{{id: "replacement-loader"}}}, nil
			})
			s.Operation("list", func(agentquery.OperationContext[row]) (any, error) {
				replacementHandlers++
				return "replacement-handler", nil
			})

			state, err = request.QueryModelJSONASTWithMode(context.Background(), attempt, agentquery.HumanReadable)
			if err != nil || state != agentquery.QueryResponseTerminal {
				t.Fatalf("continuation state=%s err=%v", state, err)
			}
			published, err := request.PublishQueryModelJSONAST()
			if err != nil || string(published) != `["original-loader/original-handler","original-loader/original-handler"]` {
				t.Fatalf("published=%s err=%v", published, err)
			}
			if originalLoads != 2 || originalHandlers != 2 || replacementLoads != 0 || replacementHandlers != 0 {
				t.Fatalf("dependency calls original=%d/%d replacement=%d/%d", originalLoads, originalHandlers, replacementLoads, replacementHandlers)
			}
			executed++
		})
	}
	if executed != len(attempts) {
		t.Fatalf("sealed continuation dependency coverage %d/%d", executed, len(attempts))
	}
	t.Logf("sealed continuation dependency coverage %d/%d through BeginQueryModelResponse and QueryModelJSONASTWithMode", executed, len(attempts))
}

func TestQueryModelContinuationUsesSealedCallerModel(t *testing.T) {
	type row struct{ id string }
	attempts := []agentquery.QueryResponseAttempt{
		agentquery.QueryResponseRetry,
		agentquery.QueryResponseRenewal,
		agentquery.QueryResponseResume,
		agentquery.QueryResponseRecovery,
	}
	executed := 0
	for _, attempt := range attempts {
		t.Run(string(attempt), func(t *testing.T) {
			s := agentquery.NewSchema[row]()
			s.Operation("list", func(ctx agentquery.OperationContext[row]) (any, error) {
				return ctx.Items()
			})
			if err := s.RegisterQueryField(agentquery.QueryFieldSpec[row]{
				Name: "id", Kind: agentquery.FieldIdentifier, Operators: []agentquery.PredicateOperator{agentquery.OperatorEquals},
				Accessor: func(v row) (agentquery.FieldValue, error) {
					return agentquery.FieldValue{Presence: agentquery.PresencePresent, Kind: agentquery.FieldIdentifier, Scalar: agentquery.ScalarValue{Kind: agentquery.FieldIdentifier, String: v.id}}, nil
				},
				AccessorCost: 1, Visibility: "public", Sensitivity: agentquery.FieldSensitivityPublic, Identity: true,
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.RegisterQueryOperationCapability(agentquery.QueryOperationCapability{
				Operation: "list",
				Clauses:   []agentquery.QueryClause{agentquery.QueryClauseTake, agentquery.QueryClauseProjection},
			}); err != nil {
				t.Fatal(err)
			}
			s.SetQuerySnapshotLoader(func(context.Context, agentquery.SnapshotLimits) (agentquery.QuerySnapshot[row], error) {
				return agentquery.QuerySnapshot[row]{Items: []row{{id: "A"}, {id: "B"}}}, nil
			})
			model, err := agentquery.ParseQueryModel(`list() take 1 { id }; list() take 1 { id }`, s.QueryLimits())
			if err != nil {
				t.Fatal(err)
			}
			request, err := s.BeginQueryModelResponse(context.Background(), model, publicQueryAccess{"public": true})
			if err != nil {
				t.Fatal(err)
			}
			state, err := request.QueryModelJSONASTWithMode(context.Background(), agentquery.QueryResponseInitial, agentquery.HumanReadable)
			if err != nil || state != agentquery.QueryResponsePaused {
				t.Fatalf("initial state=%s err=%v", state, err)
			}

			// The caller still owns the exported model, but the opaque request
			// must retain the value compiled at BeginQueryModelResponse.
			*model.Statements[1].Take = 2

			state, err = request.QueryModelJSONASTWithMode(context.Background(), attempt, agentquery.HumanReadable)
			if err != nil || state != agentquery.QueryResponseTerminal {
				t.Fatalf("continuation state=%s err=%v", state, err)
			}
			published, err := request.PublishQueryModelJSONAST()
			if err != nil || string(published) != `[[{"id":"A"}],[{"id":"A"}]]` {
				t.Fatalf("published=%s err=%v", published, err)
			}
			executed++
		})
	}
	if executed != len(attempts) {
		t.Fatalf("sealed caller-model continuation coverage %d/%d", executed, len(attempts))
	}
	t.Logf("sealed caller-model continuation coverage %d/%d through BeginQueryModelResponse and QueryModelJSONASTWithMode", executed, len(attempts))
}
