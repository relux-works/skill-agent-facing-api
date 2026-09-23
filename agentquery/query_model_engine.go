package agentquery

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type truth uint8

const operatorLegacyEqualsFold PredicateOperator = "legacyEqualsFold"

const (
	truthFalse truth = iota
	truthTrue
	truthUnknown
)

type compiledExpression struct {
	kind     ExpressionKind
	leaf     *compiledPredicate
	child    *compiledExpression
	children []*compiledExpression
}
type compiledPredicate struct {
	specName     string
	operator     PredicateOperator
	value        *Literal
	values       []Literal
	lower, upper *Literal
	regex        *regexp.Regexp
	legacyFold   bool
}
type compiledStatement[T any] struct {
	source           QueryStatement
	expr             *compiledExpression
	fields           []string
	projectionFields []string
	identityField    string
	handler          OperationHandler[T]
	selector         *FieldSelector[T]
}
type CompiledQuery[T any] struct {
	schema     *Schema[T]
	digest     [32]byte
	trustedPos bool
	loader     BoundedSnapshotLoader[T]
	statements []compiledStatement[T]
	workUsed   *uint64
}

type QueryResponseRequest[T any] struct {
	compiled  *CompiledQuery[T]
	schema    *Schema[T]
	digest    [32]byte
	next      int
	started   bool
	paused    bool
	terminal  bool
	published bool
	mode      OutputMode
	modeSet   bool
	jsonMode  bool
	results   []any
	latched   error
	// committedSuccessBytes is the size of the complete response framing for
	// the atomically committed statement prefix.  It is request-owned: every
	// continuation reuses it and no renderer keeps a parallel counter.
	committedSuccessBytes uint64
	committedWire         []byte
	// errorIncrementForTest is the frozen contract's bounded-writer seam. It
	// can only enlarge an already encoded terminal frame; production callers
	// cannot reach it because QueryResponseRequest is opaque.
	errorIncrementForTest uint64
	errorPositionForTest  *Pos
}

func (s *Schema[T]) SetQuerySnapshotLoader(loader BoundedSnapshotLoader[T]) { s.queryLoader = loader }

func (s *Schema[T]) RegisterQueryField(spec QueryFieldSpec[T]) error {
	if s.querySealed {
		return qmError("predicate_unsupported", "query model configuration is sealed")
	}
	if spec.Sensitivity != FieldSensitivityPublic && spec.Sensitivity != FieldSensitivitySecret {
		return qmError("predicate_type", "invalid field registration")
	}
	if spec.Sensitivity == FieldSensitivitySecret && spec.Groupable {
		return qmError("query_modifier", "secret-bearing field cannot be groupable")
	}
	if spec.Name == "" || spec.Visibility == "" || spec.Accessor == nil || spec.AccessorCost == 0 || !validFieldKind(spec.Kind) || (spec.Kind == FieldScalarArray && !validScalarKind(spec.ElementKind)) {
		return qmError("predicate_type", "invalid field registration")
	}
	if _, ok := s.queryFields[spec.Name]; ok {
		return qmError("predicate_type", "invalid field registration")
	}
	seen := map[PredicateOperator]bool{}
	for _, op := range spec.Operators {
		if seen[op] || !operatorAllowed(spec.Kind, op) {
			return qmError("predicate_operator", "operator is not available")
		}
		seen[op] = true
	}
	if spec.Kind == FieldScalarArray && (spec.Sortable || spec.Groupable) {
		return qmError("query_modifier", "invalid query modifier")
	}
	if spec.Identity && spec.Kind == FieldScalarArray {
		return qmError("predicate_type", "invalid field registration")
	}
	if spec.Kind == FieldEnum && len(spec.EnumValues) == 0 {
		return qmError("predicate_type", "invalid field registration")
	}
	spec.Operators = append([]PredicateOperator(nil), spec.Operators...)
	spec.EnumValues = append([]string(nil), spec.EnumValues...)
	s.queryFields[spec.Name] = spec
	s.queryFieldOrder = append(s.queryFieldOrder, spec.Name)
	return nil
}
func validFieldKind(k FieldKind) bool {
	switch k {
	case FieldString, FieldIdentifier, FieldEnum, FieldInt64, FieldBoolean, FieldTimestamp, FieldScalarArray:
		return true
	}
	return false
}
func validScalarKind(k FieldKind) bool { return validFieldKind(k) && k != FieldScalarArray }
func operatorAllowed(k FieldKind, o PredicateOperator) bool {
	if o == OperatorIsNull || o == OperatorIsMissing {
		return true
	}
	switch k {
	case FieldString, FieldIdentifier:
		return o == OperatorEquals || o == OperatorNotEquals || o == OperatorContains || o == OperatorMatchesRegex || o == OperatorLessThan || o == OperatorLessThanOrEqual || o == OperatorGreaterThan || o == OperatorGreaterThanOrEqual || o == OperatorInRange || o == OperatorInSet
	case FieldEnum:
		return o == OperatorEquals || o == OperatorNotEquals || o == OperatorInSet
	case FieldInt64, FieldTimestamp:
		return o == OperatorEquals || o == OperatorNotEquals || o == OperatorLessThan || o == OperatorLessThanOrEqual || o == OperatorGreaterThan || o == OperatorGreaterThanOrEqual || o == OperatorInRange || o == OperatorInSet
	case FieldBoolean:
		return o == OperatorEquals || o == OperatorNotEquals || o == OperatorInSet
	case FieldScalarArray:
		return o == OperatorHasElement
	}
	return false
}

func (s *Schema[T]) RegisterQueryOperationCapability(c QueryOperationCapability) error {
	if s.querySealed {
		return qmError("predicate_unsupported", "query model configuration is sealed")
	}
	if c.Operation == "" {
		return qmError("predicate_unsupported", "query operation is not available")
	}
	if _, ok := s.operations[c.Operation]; !ok {
		return qmError("predicate_unsupported", "query operation is not available")
	}
	if _, ok := s.queryCapabilities[c.Operation]; ok {
		return qmError("predicate_unsupported", "query operation is not available")
	}
	if len(c.Clauses) == 0 {
		return qmError("query_modifier", "invalid query modifier")
	}
	order := []QueryClause{QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection}
	seen := map[QueryClause]bool{}
	for _, v := range c.Clauses {
		if seen[v] || !knownClause(v) {
			return qmError("query_modifier", "invalid query modifier")
		}
		seen[v] = true
	}
	c.Clauses = nil
	for _, v := range order {
		if seen[v] {
			c.Clauses = append(c.Clauses, v)
		}
	}
	s.queryCapabilities[c.Operation] = c
	return nil
}
func knownClause(c QueryClause) bool {
	switch c {
	case QueryClauseWhere, QueryClauseSortOrder, QueryClauseGroupBy, QueryClauseSkip, QueryClauseTake, QueryClauseProjection:
		return true
	}
	return false
}

func (s *Schema[T]) sealQuery() { s.querySealed = true }
func (s *Schema[T]) CompileQueryModel(ctx context.Context, m *QueryModel, access QueryAccess) (*CompiledQuery[T], error) {
	s.sealQuery()
	if err := ctx.Err(); err != nil {
		return nil, qmError("query_cancelled", "query cancelled")
	}
	if m == nil || len(m.Statements) == 0 {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	digest := queryLimitsDigest(s.queryLimits)
	if m.limitDigest != ([32]byte{}) && m.limitDigest != digest {
		return nil, qmError("predicate_limit", "query limit mismatch")
	}
	if uint64(len(m.Statements)) > s.queryLimits.MaxStatements {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	// Compilation owns every execution-relevant byte of the request model.
	// QueryModel is exported and intentionally mutable, so retaining any of its
	// pointers or slice backing arrays would let a caller change a later
	// statement after an observed pause. Sealing is also the first validation
	// traversal: it must refuse an excessive caller graph before allocating or
	// walking the residual graph, and before the parsed-position digest marshals
	// it.
	statements, err := sealQueryStatements(m.Statements, s.queryLimits)
	if err != nil {
		return nil, err
	}
	trustedPos := m.parsedDigest != ([32]byte{}) && m.parsedDigest == queryModelParsedDigestStatements(statements)
	for _, st := range statements {
		cap, ok := s.queryCapabilities[st.Call.Operation]
		if !ok {
			return nil, qmError("predicate_unsupported", "query operation is not available")
		}
		allowed := map[QueryClause]bool{}
		for _, v := range cap.Clauses {
			allowed[v] = true
		}
		for _, v := range statementClauses(st) {
			if !allowed[v] {
				return nil, qmError("predicate_unsupported", "query clause is not available")
			}
		}
		if err := validateModifierAliases(st, allowed); err != nil {
			return nil, err
		}
		var err error
		st, err = lowerModifierAliases(st)
		if err != nil {
			return nil, err
		}
		if st.Skip != nil && *st.Skip > s.queryLimits.MaxSkip {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		if st.Take != nil {
			if *st.Take == 0 {
				return nil, qmError("query_modifier", "invalid query modifier")
			}
			if *st.Take > s.queryLimits.MaxTake {
				return nil, qmError("predicate_limit", "query limit exceeded")
			}
		}
	}
	identity := ""
	for _, name := range s.queryFieldOrder {
		if s.queryFields[name].Identity {
			if identity != "" {
				return nil, qmError("predicate_data", "identity field is invalid")
			}
			identity = name
		}
	}
	if identity == "" {
		return nil, qmError("predicate_data", "identity field is invalid")
	}
	identitySpec := s.queryFields[identity]
	if access == nil || !access.AllowsVisibility(identitySpec.Visibility) {
		return nil, qmError("predicate_field", "field is not available")
	}
	workUsed := uint64(0)
	c := &CompiledQuery[T]{schema: s, digest: digest, trustedPos: trustedPos, loader: s.queryLoader, workUsed: &workUsed}
	for _, st := range statements {
		cs := compiledStatement[T]{source: st}
		cap, ok := s.queryCapabilities[st.Call.Operation]
		if !ok {
			return nil, qmError("predicate_unsupported", "query operation is not available")
		}
		allowed := map[QueryClause]bool{}
		for _, v := range cap.Clauses {
			allowed[v] = true
		}
		clauses := statementClauses(st)
		for _, v := range clauses {
			if !allowed[v] {
				return nil, qmError("predicate_unsupported", "query clause is not available")
			}
		}
		if err := validateModifierAliases(st, allowed); err != nil {
			return nil, err
		}
		var lowerErr error
		st, lowerErr = lowerModifierAliases(st)
		if lowerErr != nil {
			return nil, lowerErr
		}
		cs.source = st
		expr := st.Where
		for _, a := range st.Call.Args {
			if a.Key == "" {
				continue
			}
			if _, legacyFilter := s.filters[a.Key]; legacyFilter {
				spec, exists := s.queryFields[a.Key]
				if !exists {
					return nil, qmError("predicate_unsupported", "query operation is not available")
				}
				if !allowed[QueryClauseWhere] {
					return nil, qmError("predicate_unsupported", "query clause is not available")
				}
				lit, err := legacyLiteral(spec, a.Value)
				if err != nil {
					return nil, err
				}
				node := leaf(a.Key, operatorLegacyEqualsFold, &lit)
				if expr == nil {
					expr = node
				} else {
					expr = SatisfiesAll(node, expr)
				}
			}
		}
		if expr != nil {
			ce, fields, err := s.compileExpression(expr, access)
			if err != nil {
				return nil, err
			}
			cs.expr = ce
			cs.fields = fields
		}
		if !containsString(cs.fields, identity) {
			cs.fields = append(cs.fields, identity)
		}
		for _, f := range st.Call.Fields {
			spec, exists := s.queryFields[f]
			if !exists || access == nil || !access.AllowsVisibility(spec.Visibility) {
				return nil, qmError("predicate_field", "field is not available")
			}
			if !containsString(cs.fields, f) {
				cs.fields = append(cs.fields, f)
			}
		}
		for _, criterion := range st.SortOrder {
			if uint64(len(st.SortOrder)) > s.queryLimits.MaxSortCriteria {
				return nil, qmError("predicate_limit", "query limit exceeded")
			}
			if criterion.Direction != QuerySortAscending && criterion.Direction != QuerySortDescending {
				return nil, qmError("query_modifier", "invalid query modifier")
			}
			spec, exists := s.queryFields[criterion.Field]
			if !exists || access == nil || !access.AllowsVisibility(spec.Visibility) {
				return nil, qmError("predicate_field", "field is not available")
			}
			if !spec.Sortable || spec.Kind == FieldScalarArray {
				return nil, qmError("query_modifier", "invalid query modifier")
			}
			if !containsString(cs.fields, criterion.Field) {
				cs.fields = append(cs.fields, criterion.Field)
			}
		}
		seenSort := map[string]bool{}
		for _, criterion := range st.SortOrder {
			if seenSort[criterion.Field] {
				return nil, qmError("query_modifier", "invalid query modifier")
			}
			seenSort[criterion.Field] = true
		}
		if st.GroupBy != nil {
			spec, exists := s.queryFields[st.GroupBy.Field]
			if !exists || access == nil || !access.AllowsVisibility(spec.Visibility) {
				return nil, qmError("predicate_field", "field is not available")
			}
			if !spec.Groupable || spec.Kind == FieldScalarArray || spec.Sensitivity == FieldSensitivitySecret {
				return nil, qmError("query_modifier", "invalid query modifier")
			}
			if !containsString(cs.fields, st.GroupBy.Field) {
				cs.fields = append(cs.fields, st.GroupBy.Field)
			}
		}
		cs.identityField = identity
		if len(st.Call.Fields) > 0 || st.GroupBy != nil {
			requested := st.Call.Fields
			if len(requested) == 0 {
				sel, err := s.newSelector(nil)
				if err != nil {
					return nil, qmError("predicate_unsupported", "query operation is not available")
				}
				requested = sel.Fields()
			}
			for _, f := range requested {
				spec, exists := s.queryFields[f]
				if !exists || access == nil || !access.AllowsVisibility(spec.Visibility) {
					return nil, qmError("predicate_field", "field is not available")
				}
				cs.projectionFields = append(cs.projectionFields, f)
				if !containsString(cs.fields, f) {
					cs.fields = append(cs.fields, f)
				}
			}
		}
		cs.handler = s.operations[st.Call.Operation]
		if len(cs.projectionFields) == 0 {
			selector, err := s.newSelector(nil)
			if err != nil {
				return nil, qmError("predicate_unsupported", "query operation is not available")
			}
			cs.selector = selector
		}
		cs.source.Where = expr
		c.statements = append(c.statements, cs)
	}
	return c, nil
}

type queryModelSealBudget struct {
	remaining uint64
}

func (b *queryModelSealBudget) consume(n uint64) error {
	if n > b.remaining {
		return qmError("predicate_limit", "query limit exceeded")
	}
	b.remaining -= n
	return nil
}

func (b *queryModelSealBudget) consumeString(value string) error {
	return b.consume(uint64(len(value)))
}

func sealQueryStatements(source []QueryStatement, limits QueryLimits) ([]QueryStatement, error) {
	budget := &queryModelSealBudget{remaining: limits.MaxSourceBytes}
	if err := budget.consume(uint64(len(source))); err != nil {
		return nil, err
	}
	cloned := make([]QueryStatement, len(source))
	for i, statement := range source {
		if uint64(len(statement.SortOrder)) > limits.MaxSortCriteria {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		if err := budget.consume(uint64(len(statement.Call.Args)) + uint64(len(statement.Call.Fields)) + uint64(len(statement.SortOrder))); err != nil {
			return nil, err
		}
		if err := budget.consumeString(statement.Call.Operation); err != nil {
			return nil, err
		}
		if !validLegacyIdentifier(statement.Call.Operation) {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		for _, arg := range statement.Call.Args {
			if err := budget.consumeString(arg.Key); err != nil {
				return nil, err
			}
			if err := budget.consumeString(arg.Value); err != nil {
				return nil, err
			}
			if (arg.Key != "" && !validLegacyIdentifier(arg.Key)) || !validRenderableString(arg.Value) {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
		}
		for _, field := range statement.Call.Fields {
			if err := budget.consumeString(field); err != nil {
				return nil, err
			}
			if !validLegacyIdentifier(field) {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
		}
		for _, criterion := range statement.SortOrder {
			if err := budget.consumeString(criterion.Field); err != nil {
				return nil, err
			}
			if err := budget.consumeString(string(criterion.Direction)); err != nil {
				return nil, err
			}
			if !validFieldIdentifier(criterion.Field) || (criterion.Direction != QuerySortAscending && criterion.Direction != QuerySortDescending) {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
		}
		if statement.GroupBy != nil {
			if err := budget.consume(1); err != nil {
				return nil, err
			}
			if err := budget.consumeString(statement.GroupBy.Field); err != nil {
				return nil, err
			}
			if !validFieldIdentifier(statement.GroupBy.Field) {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
		}
		cloned[i] = statement
		cloned[i].Call.Args = append([]Arg(nil), statement.Call.Args...)
		cloned[i].Call.Fields = append([]string(nil), statement.Call.Fields...)
		cloned[i].SortOrder = append([]SortCriterion(nil), statement.SortOrder...)
		cloned[i].GroupBy = cloneGroupCriterion(statement.GroupBy)
		cloned[i].Skip = cloneUint64(statement.Skip)
		cloned[i].Take = cloneUint64(statement.Take)
		if statement.Where != nil {
			nodes := uint64(0)
			where, err := cloneExpressionBounded(statement.Where, limits, budget, make(map[*Expression]bool), 1, &nodes)
			if err != nil {
				return nil, err
			}
			cloned[i].Where = where
		}
	}
	return cloned, nil
}

func cloneExpressionBounded(source *Expression, limits QueryLimits, budget *queryModelSealBudget, path map[*Expression]bool, depth uint64, nodes *uint64) (*Expression, error) {
	if source == nil || path[source] {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	if depth > limits.MaxExpressionDepth {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	*nodes = *nodes + 1
	if *nodes > limits.MaxExpressionNodes {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	if err := budget.consume(1); err != nil {
		return nil, err
	}
	path[source] = true
	defer delete(path, source)
	cloned := &Expression{Kind: source.Kind, Pos: source.Pos}
	switch source.Kind {
	case ExpressionNot:
		if source.Child == nil || source.Predicate != nil || len(source.Children) > 0 {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		child, err := cloneExpressionBounded(source.Child, limits, budget, path, depth+1, nodes)
		if err != nil {
			return nil, err
		}
		cloned.Child = child
	case ExpressionSatisfiesAll, ExpressionSatisfiesAny:
		if source.Child != nil || source.Predicate != nil || len(source.Children) == 0 {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		// Every child contributes at least one occurrence. Refuse before making
		// a caller-sized slice or visiting the out-of-bound remainder.
		if uint64(len(source.Children)) > limits.MaxExpressionNodes-*nodes {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		cloned.Children = make([]*Expression, len(source.Children))
		for i, child := range source.Children {
			copy, err := cloneExpressionBounded(child, limits, budget, path, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			cloned.Children[i] = copy
		}
	case ExpressionPredicate:
		if source.Predicate == nil || source.Child != nil || len(source.Children) > 0 {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		predicate, err := clonePredicateBounded(source.Predicate, limits, budget)
		if err != nil {
			return nil, err
		}
		cloned.Predicate = predicate
	default:
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	return cloned, nil
}

func clonePredicateBounded(source *Predicate, limits QueryLimits, budget *queryModelSealBudget) (*Predicate, error) {
	if uint64(len(source.Values)) > limits.MaxSetEntries {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	if err := budget.consume(1); err != nil {
		return nil, err
	}
	if err := budget.consumeString(source.Field); err != nil {
		return nil, err
	}
	if err := budget.consumeString(string(source.Operator)); err != nil {
		return nil, err
	}
	if !validFieldIdentifier(source.Field) {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	cloned := *source
	var err error
	if cloned.Value, err = cloneLiteralBounded(source.Value, limits, budget); err != nil {
		return nil, err
	}
	cloned.Values = make([]Literal, len(source.Values))
	for i := range source.Values {
		literal, literalErr := cloneLiteralBounded(&source.Values[i], limits, budget)
		if literalErr != nil {
			return nil, literalErr
		}
		cloned.Values[i] = *literal
	}
	if cloned.Lower, err = cloneLiteralBounded(source.Lower, limits, budget); err != nil {
		return nil, err
	}
	if cloned.Upper, err = cloneLiteralBounded(source.Upper, limits, budget); err != nil {
		return nil, err
	}
	if !validExternalPredicateGrammar(&cloned) {
		return nil, qmError("predicate_syntax", "invalid query syntax")
	}
	return &cloned, nil
}

// validExternalPredicateGrammar applies the same frozen grammar accepted by
// RenderQueryModel without rendering or traversing the expression a second
// time. The bounded sealing pass calls it before a caller-built model can
// reach a snapshot loader or operation handler.
func validExternalPredicateGrammar(p *Predicate) bool {
	switch p.Operator {
	case OperatorIsNull, OperatorIsMissing:
		return p.Value == nil && p.Lower == nil && p.Upper == nil && len(p.Values) == 0
	case OperatorInRange:
		return p.Lower != nil && p.Upper != nil && p.Value == nil && len(p.Values) == 0 && validLiteral(*p.Lower) && validLiteral(*p.Upper)
	case OperatorInSet:
		if len(p.Values) == 0 || p.Value != nil || p.Lower != nil || p.Upper != nil {
			return false
		}
		for _, value := range p.Values {
			if !validLiteral(value) {
				return false
			}
		}
		return true
	default:
		return knownOperator(p.Operator) && p.Value != nil && p.Lower == nil && p.Upper == nil && len(p.Values) == 0 && validLiteral(*p.Value)
	}
}

func cloneLiteralBounded(source *Literal, limits QueryLimits, budget *queryModelSealBudget) (*Literal, error) {
	if source == nil {
		return nil, nil
	}
	if uint64(len(source.Text)) > limits.MaxLiteralBytes {
		return nil, qmError("predicate_limit", "query limit exceeded")
	}
	if err := budget.consume(1); err != nil {
		return nil, err
	}
	if err := budget.consumeString(source.Text); err != nil {
		return nil, err
	}
	cloned := *source
	return &cloned, nil
}

func cloneGroupCriterion(source *GroupCriterion) *GroupCriterion {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func cloneUint64(source *uint64) *uint64 {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func containsString(xs []string, value string) bool {
	for _, x := range xs {
		if x == value {
			return true
		}
	}
	return false
}
func statementClauses(s QueryStatement) []QueryClause {
	var r []QueryClause
	if s.Where != nil {
		r = append(r, QueryClauseWhere)
	}
	if len(s.SortOrder) > 0 {
		r = append(r, QueryClauseSortOrder)
	}
	if s.GroupBy != nil {
		r = append(r, QueryClauseGroupBy)
	}
	if s.Skip != nil {
		r = append(r, QueryClauseSkip)
	}
	if s.Take != nil {
		r = append(r, QueryClauseTake)
	}
	if len(s.Call.Fields) > 0 {
		r = append(r, QueryClauseProjection)
	}
	return r
}
func validateModifierAliases(s QueryStatement, allowed map[QueryClause]bool) error {
	for _, a := range s.Call.Args {
		var clause QueryClause
		switch {
		case strings.HasPrefix(a.Key, "sort_"):
			clause = QueryClauseSortOrder
		case a.Key == "skip":
			clause = QueryClauseSkip
		case a.Key == "take":
			clause = QueryClauseTake
		default:
			continue
		}
		if !allowed[clause] {
			return qmError("predicate_unsupported", "query clause is not available")
		}
		if (clause == QueryClauseSortOrder && len(s.SortOrder) > 0) || (clause == QueryClauseSkip && s.Skip != nil) || (clause == QueryClauseTake && s.Take != nil) {
			return qmError("query_modifier", "conflicting query modifier")
		}
	}
	return nil
}
func lowerModifierAliases(s QueryStatement) (QueryStatement, error) {
	seenSort := map[string]bool{}
	seenSkip := false
	seenTake := false
	for _, a := range s.Call.Args {
		switch {
		case strings.HasPrefix(a.Key, "sort_"):
			field := strings.TrimPrefix(a.Key, "sort_")
			if field == "" || seenSort[field] {
				return s, qmError("query_modifier", "invalid query modifier")
			}
			seenSort[field] = true
			direction := QuerySortDirection("")
			if a.Value == "asc" {
				direction = QuerySortAscending
			}
			if a.Value == "desc" {
				direction = QuerySortDescending
			}
			if direction == "" {
				return s, qmError("query_modifier", "invalid query modifier")
			}
			s.SortOrder = append(s.SortOrder, SortCriterion{Field: field, Direction: direction})
		case a.Key == "skip" || a.Key == "take":
			if (a.Key == "skip" && seenSkip) || (a.Key == "take" && seenTake) {
				return s, qmError("query_modifier", "invalid query modifier")
			}
			value, err := strconv.ParseUint(a.Value, 10, 64)
			if err != nil {
				return s, qmError("query_modifier", "invalid query modifier")
			}
			if a.Key == "skip" {
				seenSkip = true
				s.Skip = &value
			} else {
				seenTake = true
				s.Take = &value
			}
		}
	}
	return s, nil
}
func legacyLiteral[T any](s QueryFieldSpec[T], v string) (Literal, error) {
	switch s.Kind {
	case FieldString, FieldIdentifier:
		return StringValue(v), nil
	case FieldEnum:
		for _, allowed := range s.EnumValues {
			if strings.EqualFold(allowed, v) {
				return StringValue(allowed), nil
			}
		}
		return Literal{}, qmError("predicate_type", "invalid predicate value")
	case FieldInt64:
		return Literal{}, qmError("predicate_type", "invalid predicate value")
	case FieldBoolean:
		if strings.EqualFold(v, "true") {
			return BooleanValue(true), nil
		}
		if strings.EqualFold(v, "false") {
			return BooleanValue(false), nil
		}
	}
	return Literal{}, qmError("predicate_type", "invalid predicate value")
}

func (s *Schema[T]) compileExpression(root *Expression, access QueryAccess) (*compiledExpression, []string, error) {
	path := map[*Expression]bool{}
	seenFields := map[string]bool{}
	var fields []string
	var nodes uint64
	var regexBytes uint64
	var walk func(*Expression, uint64) (*compiledExpression, error)
	walk = func(e *Expression, depth uint64) (*compiledExpression, error) {
		if e == nil || path[e] {
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		if depth > s.queryLimits.MaxExpressionDepth {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		nodes++
		if nodes > s.queryLimits.MaxExpressionNodes {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		path[e] = true
		defer delete(path, e)
		ce := &compiledExpression{kind: e.Kind}
		switch e.Kind {
		case ExpressionNot:
			if e.Child == nil || e.Predicate != nil || len(e.Children) > 0 {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
			x, err := walk(e.Child, depth+1)
			if err != nil {
				return nil, err
			}
			ce.child = x
		case ExpressionSatisfiesAll, ExpressionSatisfiesAny:
			if e.Child != nil || e.Predicate != nil || len(e.Children) == 0 {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
			for _, ch := range e.Children {
				x, err := walk(ch, depth+1)
				if err != nil {
					return nil, err
				}
				ce.children = append(ce.children, x)
			}
		case ExpressionPredicate:
			if e.Predicate == nil || e.Child != nil || len(e.Children) > 0 {
				return nil, qmError("predicate_syntax", "invalid query syntax")
			}
			p := e.Predicate
			spec, ok := s.queryFields[p.Field]
			if !ok || access == nil || !access.AllowsVisibility(spec.Visibility) {
				return nil, qmError("predicate_field", "field is not available")
			}
			requestedOperator := p.Operator
			if requestedOperator == operatorLegacyEqualsFold {
				requestedOperator = OperatorEquals
			}
			if !containsOp(spec.Operators, requestedOperator) {
				return nil, qmError("predicate_operator", "operator is not available")
			}
			if !seenFields[p.Field] {
				seenFields[p.Field] = true
				fields = append(fields, p.Field)
			}
			cp := &compiledPredicate{specName: p.Field, operator: requestedOperator, value: p.Value, values: p.Values, lower: p.Lower, upper: p.Upper, legacyFold: p.Operator == operatorLegacyEqualsFold}
			validationPredicate := *p
			validationPredicate.Operator = requestedOperator
			if err := validatePredicate(spec, &validationPredicate, cp, s.queryLimits); err != nil {
				return nil, err
			}
			if cp.regex != nil {
				regexBytes += uint64(len(cp.value.Text))
				if regexBytes > s.queryLimits.MaxRegexBytesPerStatement {
					return nil, qmError("predicate_limit", "query limit exceeded")
				}
			}
			ce.leaf = cp
		default:
			return nil, qmError("predicate_syntax", "invalid query syntax")
		}
		return ce, nil
	}
	ce, err := walk(root, 1)
	return ce, fields, err
}
func containsOp(xs []PredicateOperator, o PredicateOperator) bool {
	for _, x := range xs {
		if x == o {
			return true
		}
	}
	return false
}
func validatePredicate[T any](s QueryFieldSpec[T], p *Predicate, cp *compiledPredicate, l QueryLimits) error {
	presence := p.Operator == OperatorIsNull || p.Operator == OperatorIsMissing
	if presence {
		if p.Value != nil || p.Lower != nil || p.Upper != nil || len(p.Values) > 0 {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		return nil
	}
	if p.Operator == OperatorInRange {
		if p.Lower == nil || p.Upper == nil || p.Value != nil || len(p.Values) > 0 {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		if literalBytes(*p.Lower) > l.MaxLiteralBytes || literalBytes(*p.Upper) > l.MaxLiteralBytes {
			return qmError("predicate_limit", "query limit exceeded")
		}
		if !literalMatches(s, *p.Lower) || !literalMatches(s, *p.Upper) {
			return qmError("predicate_type", "invalid predicate value")
		}
		return nil
	}
	if p.Operator == OperatorInSet {
		if len(p.Values) == 0 || p.Value != nil || p.Lower != nil || p.Upper != nil {
			return qmError("predicate_syntax", "invalid query syntax")
		}
		if uint64(len(p.Values)) > l.MaxSetEntries {
			return qmError("predicate_limit", "query limit exceeded")
		}
		for _, v := range p.Values {
			if literalBytes(v) > l.MaxLiteralBytes {
				return qmError("predicate_limit", "query limit exceeded")
			}
			if !literalMatches(s, v) {
				return qmError("predicate_type", "invalid predicate value")
			}
		}
		return nil
	}
	if p.Value == nil || p.Lower != nil || p.Upper != nil || len(p.Values) > 0 {
		return qmError("predicate_syntax", "invalid query syntax")
	}
	if literalBytes(*p.Value) > l.MaxLiteralBytes {
		return qmError("predicate_limit", "query limit exceeded")
	}
	if !literalMatches(s, *p.Value) {
		return qmError("predicate_type", "invalid predicate value")
	}
	if p.Operator == OperatorMatchesRegex {
		if p.Value.Kind != LiteralString {
			return qmError("predicate_type", "invalid predicate value")
		}
		if uint64(len(p.Value.Text)) > l.MaxRegexBytesPerStatement {
			return qmError("predicate_limit", "query limit exceeded")
		}
		r, err := regexp.Compile(p.Value.Text)
		if err != nil {
			return qmError("predicate_regex", "invalid regular expression")
		}
		cp.regex = r
	}
	return nil
}
func literalBytes(v Literal) uint64 {
	if v.Kind == LiteralString || v.Kind == LiteralTimestamp {
		return uint64(len(v.Text))
	}
	return 0
}
func literalMatches[T any](s QueryFieldSpec[T], v Literal) bool {
	k := s.Kind
	if k == FieldScalarArray {
		k = s.ElementKind
	}
	switch k {
	case FieldString, FieldIdentifier, FieldEnum:
		if v.Kind != LiteralString {
			return false
		}
		if k == FieldEnum {
			for _, allowed := range s.EnumValues {
				if allowed == v.Text {
					return true
				}
			}
			return false
		}
		return true
	case FieldInt64:
		return v.Kind == LiteralInt64
	case FieldBoolean:
		return v.Kind == LiteralBoolean
	case FieldTimestamp:
		return v.Kind == LiteralTimestamp && validTimestamp(v.Text)
	}
	return false
}

func (s *Schema[T]) QueryModelAST(ctx context.Context, m *QueryModel, access QueryAccess) (any, error) {
	r, err := s.BeginQueryModelResponse(ctx, m, access)
	if err != nil {
		return nil, err
	}
	state, err := r.QueryModelAST(ctx, QueryResponseInitial)
	for err == nil && state == QueryResponsePaused {
		state, err = r.QueryModelAST(ctx, QueryResponseResume)
	}
	if err != nil {
		_, publishErr := r.PublishQueryModelAST()
		if publishErr != nil {
			return nil, publishErr
		}
		// Preserve the compatibility one-shot contract: typed terminal records
		// are observable through the opaque request publication API, while this
		// wrapper continues to return nil alongside its execution error.
		return nil, err
	}
	published, err := r.PublishQueryModelAST()
	if err != nil {
		return nil, err
	}
	records := published.([]any)
	if len(records) == 1 {
		return records[0], nil
	}
	return records, nil
}
func (s *Schema[T]) QueryModelJSONASTWithMode(ctx context.Context, m *QueryModel, access QueryAccess, mode OutputMode) ([]byte, error) {
	r, err := s.BeginQueryModelResponse(ctx, m, access)
	if err != nil {
		return nil, err
	}
	state, err := r.QueryModelJSONASTWithMode(ctx, QueryResponseInitial, mode)
	for err == nil && state == QueryResponsePaused {
		state, err = r.QueryModelJSONASTWithMode(ctx, QueryResponseResume, mode)
	}
	if err != nil {
		b, publishErr := r.PublishQueryModelJSONAST()
		if publishErr != nil {
			return nil, publishErr
		}
		return b, err
	}
	return r.PublishQueryModelJSONAST()
}

func (s *Schema[T]) BeginQueryModelResponse(ctx context.Context, m *QueryModel, access QueryAccess) (*QueryResponseRequest[T], error) {
	c, err := s.CompileQueryModel(ctx, m, access)
	if err != nil {
		return nil, err
	}
	return &QueryResponseRequest[T]{compiled: c, schema: c.schema, digest: c.digest}, nil
}

func (r *QueryResponseRequest[T]) QueryModelAST(ctx context.Context, attempt QueryResponseAttempt) (QueryResponseState, error) {
	return r.dispatch(ctx, attempt, HumanReadable, false)
}

func (r *QueryResponseRequest[T]) QueryModelJSONASTWithMode(ctx context.Context, attempt QueryResponseAttempt, mode OutputMode) (QueryResponseState, error) {
	return r.dispatch(ctx, attempt, mode, true)
}

func (r *QueryResponseRequest[T]) dispatch(ctx context.Context, attempt QueryResponseAttempt, mode OutputMode, jsonMode bool) (QueryResponseState, error) {
	if r == nil || r.compiled == nil || r.schema == nil || r.compiled.schema != r.schema || r.compiled.digest != r.digest || r.digest != queryLimitsDigest(r.schema.queryLimits) || r.terminal || r.published {
		return QueryResponseTerminal, qmError("predicate_unsupported", "query response transition is not available")
	}
	if !knownQueryResponseAttempt(attempt) {
		return QueryResponseTerminal, qmError("predicate_unsupported", "query response transition is not available")
	}
	if !r.started {
		if attempt != QueryResponseInitial {
			return QueryResponseTerminal, qmError("predicate_unsupported", "query response transition is not available")
		}
		r.started = true
		r.mode = mode
		r.modeSet = true
		r.jsonMode = jsonMode
		if !jsonMode || mode == HumanReadable {
			r.committedWire = []byte("[]")
			r.committedSuccessBytes = 2
		}
	} else {
		if !r.paused || attempt == QueryResponseInitial || jsonMode != r.jsonMode || mode != r.mode {
			return QueryResponseTerminal, qmError("predicate_unsupported", "query response transition is not available")
		}
	}
	if jsonMode && mode != HumanReadable && mode != LLMReadable {
		return QueryResponseTerminal, qmError("predicate_unsupported", "query response transition is not available")
	}
	r.paused = false
	statement := r.compiled.statements[r.next]
	one := &CompiledQuery[T]{schema: r.compiled.schema, digest: r.compiled.digest, trustedPos: r.compiled.trustedPos, loader: r.compiled.loader, statements: []compiledStatement[T]{statement}, workUsed: r.compiled.workUsed}
	v, err := one.execute(ctx)
	if err != nil {
		r.latched = err
		r.terminal = true
		return QueryResponseTerminal, err
	}
	candidate := append(append([]any(nil), r.results...), v)
	encoded, err := stageQueryStatement(r.committedWire, v, r.mode, r.jsonMode, statement)
	if err != nil {
		r.latched = qmError("result_limit", "query result limit exceeded")
		r.terminal = true
		return QueryResponseTerminal, r.latched
	}
	limits := r.compiled.schema.queryLimits
	if limits.ErrorFramingReserveBytes > limits.MaxResponseBytes || uint64(len(encoded)) > limits.MaxResponseBytes-limits.ErrorFramingReserveBytes {
		r.latched = qmError("result_limit", "query result limit exceeded")
		r.terminal = true
		return QueryResponseTerminal, r.latched
	}
	r.results = candidate
	r.committedSuccessBytes = uint64(len(encoded))
	r.committedWire = append(r.committedWire[:0], encoded...)
	r.next++
	if r.next < len(r.compiled.statements) {
		r.paused = true
		return QueryResponsePaused, nil
	}
	r.terminal = true
	return QueryResponseTerminal, nil
}

func knownQueryResponseAttempt(attempt QueryResponseAttempt) bool {
	switch attempt {
	case QueryResponseInitial, QueryResponseRetry, QueryResponseRenewal, QueryResponseResume, QueryResponseRecovery:
		return true
	default:
		return false
	}
}

func (r *QueryResponseRequest[T]) PublishQueryModelAST() (any, error) {
	if r == nil || !r.terminal || r.published || r.jsonMode {
		return nil, qmError("predicate_unsupported", "query response publication is not available")
	}
	r.published = true
	if r.latched != nil {
		envelope, err := r.terminalErrorEnvelope()
		if err != nil {
			return nil, err
		}
		out := append(append([]any(nil), r.results...), envelope)
		wire, err := appendJSONError(r.committedWire, envelope)
		if err != nil {
			return nil, err
		}
		if err := r.validateErrorPublication(nil, wire); err != nil {
			return nil, err
		}
		return out, nil
	}
	return append([]any(nil), r.results...), nil
}
func (r *QueryResponseRequest[T]) PublishQueryModelJSONAST() ([]byte, error) {
	if r == nil || !r.terminal || r.published || !r.jsonMode {
		return nil, qmError("predicate_unsupported", "query response publication is not available")
	}
	r.published = true
	if r.latched != nil {
		envelope, err := r.terminalErrorEnvelope()
		if err != nil {
			return nil, err
		}
		var out []byte
		if r.mode == LLMReadable {
			out = append([]byte(nil), r.committedWire...)
			record, recordErr := renderCompactError(envelope)
			if recordErr != nil {
				err = recordErr
			} else if len(out) == 0 {
				out = record
			} else {
				out = append(append(out, '\n'), record...)
			}
		} else {
			out, err = appendJSONError(r.committedWire, envelope)
		}
		if err != nil {
			return nil, qmError("predicate_unsupported", "query response publication is not available")
		}
		if r.errorIncrementForTest != 0 {
			out, err = padQueryErrorIncrement(out, r.committedSuccessBytes, r.errorIncrementForTest, r.mode)
			if err != nil {
				return nil, qmError("predicate_unsupported", "query response publication is not available")
			}
		}
		if err := r.validateErrorPublication(nil, out); err != nil {
			return nil, err
		}
		return out, nil
	}
	return append([]byte(nil), r.committedWire...), nil
}

func appendJSONError(prefix []byte, envelope queryTerminalError) ([]byte, error) {
	if len(prefix) < 2 || prefix[0] != '[' || prefix[len(prefix)-1] != ']' {
		return nil, qmError("predicate_unsupported", "query response publication is not available")
	}
	record, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	out := append([]byte(nil), prefix[:len(prefix)-1]...)
	if len(prefix) > 2 {
		out = append(out, ',')
	}
	out = append(out, record...)
	out = append(out, ']')
	return out, nil
}

func padQueryErrorIncrement(out []byte, committed, target uint64, mode OutputMode) ([]byte, error) {
	if uint64(len(out)) < committed || uint64(len(out))-committed > target {
		return nil, qmError("predicate_unsupported", "query response publication is not available")
	}
	padding := target - (uint64(len(out)) - committed)
	if padding == 0 {
		return out, nil
	}
	pad := []byte(strings.Repeat(" ", int(padding)))
	if mode == LLMReadable {
		return append(out, pad...), nil
	}
	if len(out) == 0 || out[len(out)-1] != ']' {
		return nil, qmError("predicate_unsupported", "query response publication is not available")
	}
	padded := make([]byte, 0, uint64(len(out))+padding)
	padded = append(padded, out[:len(out)-1]...)
	padded = append(padded, pad...)
	padded = append(padded, ']')
	return padded, nil
}

type queryTerminalError struct {
	Error queryTerminalErrorBody `json:"error"`
}

type queryTerminalErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Statement uint64 `json:"statement,omitempty"`
	Position  *Pos   `json:"position,omitempty"`
}

var queryTerminalMessages = map[string]string{
	"predicate_syntax":      "query syntax is invalid",
	"predicate_unsupported": "query model is not supported",
	"predicate_field":       "field is not available",
	"predicate_operator":    "operator is not available for field",
	"predicate_type":        "query value has invalid type",
	"predicate_regex":       "regular expression is invalid",
	"query_modifier":        "query modifier is invalid",
	"predicate_limit":       "query limit exceeded",
	"predicate_cost":        "query work limit exceeded",
	"predicate_data":        "query data is invalid",
	"result_limit":          "query result limit exceeded",
	"query_cancelled":       "query was cancelled",
}

func (r *QueryResponseRequest[T]) terminalErrorEnvelope() (queryTerminalError, error) {
	qe, ok := r.latched.(*Error)
	if !ok {
		return queryTerminalError{}, qmError("predicate_unsupported", "query response publication is not available")
	}
	message, ok := queryTerminalMessages[qe.Code]
	if !ok {
		return queryTerminalError{}, qmError("predicate_unsupported", "query response publication is not available")
	}
	var position *Pos
	if r.errorPositionForTest != nil {
		p := *r.errorPositionForTest
		if p.Offset <= 65536 && p.Line <= 65537 && p.Column <= 65537 {
			position = &p
		}
	} else if r.compiled.trustedPos && r.next < len(r.compiled.statements) {
		p := r.compiled.statements[r.next].source.Call.Pos
		if p.Offset <= 65536 && p.Line <= 65537 && p.Column <= 65537 && (p.Offset != 0 || p.Line != 0 || p.Column != 0) {
			position = &p
		}
	}
	return queryTerminalError{Error: queryTerminalErrorBody{Code: qe.Code, Message: message, Statement: uint64(r.next + 1), Position: position}}, nil
}

func (r *QueryResponseRequest[T]) validateErrorPublication(_ []any, wire []byte) error {
	limits := r.compiled.schema.queryLimits
	if uint64(len(wire)) > limits.MaxResponseBytes || uint64(len(wire)) < r.committedSuccessBytes {
		return qmError("predicate_unsupported", "query response publication is not available")
	}
	incremental := uint64(len(wire)) - r.committedSuccessBytes
	if incremental > limits.ErrorFramingReserveBytes {
		return qmError("predicate_unsupported", "query response publication is not available")
	}
	return nil
}

func renderQuerySuccess[T any](results []any, mode OutputMode, jsonMode bool, statements []compiledStatement[T]) ([]byte, error) {
	if jsonMode && mode == LLMReadable {
		return renderCompactResponse(results, statements)
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, result := range results {
		if i > 0 {
			b.WriteByte(',')
		}
		statement, err := renderJSONStatement(result)
		if err != nil {
			return nil, err
		}
		b.Write(statement)
	}
	b.WriteByte(']')
	return []byte(b.String()), nil
}

func stageQueryStatement[T any](prefix []byte, result any, mode OutputMode, jsonMode bool, statement compiledStatement[T]) ([]byte, error) {
	if jsonMode && mode == LLMReadable {
		record, err := renderCompactStatement(result, statement.projectionFields)
		if err != nil {
			return nil, err
		}
		if len(prefix) == 0 {
			return record, nil
		}
		out := append([]byte(nil), prefix...)
		out = append(out, '\n')
		out = append(out, record...)
		return out, nil
	}
	record, err := renderJSONStatement(result)
	if err != nil {
		return nil, err
	}
	if len(prefix) < 2 || prefix[0] != '[' || prefix[len(prefix)-1] != ']' {
		return nil, qmError("predicate_unsupported", "query response transition is not available")
	}
	out := append([]byte(nil), prefix[:len(prefix)-1]...)
	if len(prefix) > 2 {
		out = append(out, ',')
	}
	out = append(out, record...)
	out = append(out, ']')
	return out, nil
}

func renderJSONStatement(result any) ([]byte, error) {
	groups, ok := result.([]GroupResult)
	if !ok {
		return json.Marshal(result)
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, group := range groups {
		if i > 0 {
			b.WriteByte(',')
		}
		presence, err := json.Marshal(group.Key.Presence)
		if err != nil {
			return nil, err
		}
		items, err := json.Marshal(group.Items)
		if err != nil {
			return nil, err
		}
		b.WriteString(`{"key":{"presence":`)
		b.Write(presence)
		if group.Key.Presence == PresencePresent {
			token, err := canonicalGroupKeyToken(group.Key)
			if err != nil {
				return nil, err
			}
			b.WriteString(`,"value":`)
			b.Write(token)
		}
		b.WriteString(`},"count":`)
		b.WriteString(strconv.FormatUint(group.Count, 10))
		b.WriteString(`,"items":`)
		b.Write(items)
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return []byte(b.String()), nil
}

func renderCompactResponse[T any](results []any, statements []compiledStatement[T]) ([]byte, error) {
	var b strings.Builder
	for i, result := range results {
		if i > 0 {
			b.WriteByte('\n')
		}
		fields := []string(nil)
		if i < len(statements) {
			fields = statements[i].projectionFields
		}
		record, err := renderCompactStatement(result, fields)
		if err != nil {
			return nil, err
		}
		b.Write(record)
	}
	return []byte(b.String()), nil
}

func renderCompactStatement(result any, fields []string) ([]byte, error) {
	switch groups := result.(type) {
	case []GroupResult:
		var b strings.Builder
		for i, group := range groups {
			if i > 0 {
				b.WriteByte('\n')
			}
			token := ""
			if group.Key.Presence == PresencePresent {
				encoded, err := canonicalGroupKeyToken(group.Key)
				if err != nil {
					return nil, err
				}
				token = string(encoded)
			}
			cells := []any{"@group", string(group.Key.Presence), token, group.Count}
			for j, cell := range cells {
				if j > 0 {
					b.WriteByte(',')
				}
				b.WriteString(escapeCSV(cell))
			}
			b.WriteByte('\n')
			items, err := FormatCompact(group.Items, fields)
			if err != nil {
				return nil, err
			}
			b.Write(items)
		}
		return []byte(b.String()), nil
	default:
		return FormatCompact(result, fields)
	}
}

func canonicalGroupKeyToken(key GroupKey) ([]byte, error) {
	if key.Presence != PresencePresent {
		return nil, nil
	}
	encoded, err := json.Marshal(key.Value)
	if err != nil {
		return nil, qmError("predicate_data", "query data is invalid")
	}
	return encoded, nil
}

func renderCompactError(envelope queryTerminalError) ([]byte, error) {
	message, err := json.Marshal(envelope.Error.Message)
	if err != nil {
		return nil, err
	}
	offset, line, column := "", "", ""
	if envelope.Error.Position != nil {
		offset = strconv.Itoa(envelope.Error.Position.Offset)
		line = strconv.Itoa(envelope.Error.Position.Line)
		column = strconv.Itoa(envelope.Error.Position.Column)
	}
	var b strings.Builder
	b.WriteString("@error,")
	b.WriteString(envelope.Error.Code)
	b.WriteByte(',')
	b.Write(message)
	b.WriteByte(',')
	b.WriteString(strconv.FormatUint(envelope.Error.Statement, 10))
	b.WriteByte(',')
	b.WriteString(offset)
	b.WriteByte(',')
	b.WriteString(line)
	b.WriteByte(',')
	b.WriteString(column)
	b.WriteByte('\n')
	return []byte(b.String()), nil
}
func (c *CompiledQuery[T]) execute(ctx context.Context) (any, error) {
	if c.schema == nil || c.digest != queryLimitsDigest(c.schema.queryLimits) {
		return nil, qmError("predicate_limit", "query limit mismatch")
	}
	if c.loader == nil {
		return nil, qmError("predicate_unsupported", "bounded snapshot loader is not available")
	}
	results := make([]any, 0, len(c.statements))
	for _, st := range c.statements {
		snap, err := c.loader(ctx, SnapshotLimits{MaxRows: c.schema.queryLimits.MaxSnapshotRows, MaxBytes: c.schema.queryLimits.MaxSnapshotTypedBytes})
		if err != nil {
			return nil, err
		}
		if uint64(len(snap.Items)) > c.schema.queryLimits.MaxSnapshotRows || snap.DecodedBytes > c.schema.queryLimits.MaxSnapshotTypedBytes {
			return nil, qmError("predicate_limit", "query limit exceeded")
		}
		selected := make([]T, 0)
		selectedValues := make([]map[string]FieldValue, 0)
		identities := map[string]bool{}
		for _, item := range snap.Items {
			vals := map[string]FieldValue{}
			for _, f := range st.fields {
				if !chargeWork(c.workUsed, c.schema.queryFields[f].AccessorCost, c.schema.queryLimits.MaxWorkUnits) {
					return nil, qmError("predicate_cost", "query cost exceeded")
				}
				v, e := c.schema.queryFields[f].Accessor(item)
				if e != nil {
					return nil, qmError("predicate_data", "field data is invalid")
				}
				if e = validateFieldValue(c.schema.queryFields[f], v, c.schema.queryLimits); e != nil {
					return nil, e
				}
				vals[f] = v
			}
			for _, f := range st.fields {
				if c.schema.queryFields[f].Identity {
					v := vals[f]
					if v.Presence != PresencePresent {
						return nil, qmError("predicate_data", "identity field is invalid")
					}
					key := fieldIdentityKey(v)
					if identities[key] {
						return nil, qmError("predicate_data", "identity field is invalid")
					}
					identities[key] = true
				}
			}
			if st.expr != nil && !chargeRegexWork(st.expr, vals, c.workUsed, c.schema.queryLimits.MaxWorkUnits) {
				return nil, qmError("predicate_cost", "query cost exceeded")
			}
			if st.expr == nil || evalExpression(st.expr, vals, c.schema.queryFields) == truthTrue {
				selected = append(selected, item)
				selectedValues = append(selectedValues, vals)
			}
		}
		criteria := append([]SortCriterion(nil), st.source.SortOrder...)
		identityExplicit := false
		for _, criterion := range criteria {
			if criterion.Field == st.identityField {
				identityExplicit = true
			}
		}
		if !identityExplicit {
			criteria = append(criteria, SortCriterion{Field: st.identityField, Direction: QuerySortAscending})
		}
		orderRows := make([]int, len(selected))
		for i := range orderRows {
			orderRows[i] = i
		}
		costFailed := false
		sort.SliceStable(orderRows, func(a, b int) bool {
			i, j := orderRows[a], orderRows[b]
			for _, criterion := range criteria {
				if !chargeWork(c.workUsed, 1, c.schema.queryLimits.MaxWorkUnits) {
					costFailed = true
					return false
				}
				left, right := selectedValues[i][criterion.Field], selectedValues[j][criterion.Field]
				cmp := compareFieldValues(left, right)
				if cmp != 0 {
					if criterion.Direction == QuerySortDescending && left.Presence == PresencePresent && right.Presence == PresencePresent {
						cmp = -cmp
					}
					return cmp < 0
				}
			}
			return false
		})
		if costFailed {
			return nil, qmError("predicate_cost", "query cost exceeded")
		}
		sortedRows := make([]T, len(orderRows))
		sortedValues := make([]map[string]FieldValue, len(orderRows))
		for i, original := range orderRows {
			sortedRows[i], sortedValues[i] = selected[original], selectedValues[original]
		}
		selected, selectedValues = sortedRows, sortedValues

		if st.source.GroupBy != nil {
			groups := make(map[string]*GroupResult)
			keys := make(map[string]FieldValue)
			for _, values := range selectedValues {
				v := values[st.source.GroupBy.Field]
				key := fieldIdentityKey(v)
				g := groups[key]
				if g == nil {
					if uint64(len(groups)) >= c.schema.queryLimits.MaxGroupCardinality {
						return nil, qmError("predicate_limit", "query limit exceeded")
					}
					g = &GroupResult{Key: groupKey(v)}
					groups[key] = g
					keys[key] = v
				}
				g.Count++
				g.Items = append(g.Items, projectValues(values, st.projectionFields))
			}
			order := make([]string, 0, len(groups))
			for key := range groups {
				order = append(order, key)
			}
			sort.Slice(order, func(i, j int) bool { return compareFieldValues(keys[order[i]], keys[order[j]]) < 0 })
			paged, err := pageBounds(order, st.source.Skip, st.source.Take, c.schema.queryLimits)
			if err != nil {
				return nil, err
			}
			out := make([]GroupResult, 0, len(paged))
			for _, key := range paged {
				out = append(out, *groups[key])
			}
			results = append(results, out)
			continue
		}
		indices := make([]int, len(selected))
		for i := range indices {
			indices[i] = i
		}
		paged, err := pageBounds(indices, st.source.Skip, st.source.Take, c.schema.queryLimits)
		if err != nil {
			return nil, err
		}
		if len(st.projectionFields) > 0 {
			projected := make([]map[string]any, 0, len(paged))
			for _, i := range paged {
				projected = append(projected, projectValues(selectedValues[i], st.projectionFields))
			}
			results = append(results, projected)
			continue
		}
		pagedRows := make([]T, 0, len(paged))
		for _, i := range paged {
			pagedRows = append(pagedRows, selected[i])
		}
		selected = pagedRows
		h := st.handler
		if h != nil {
			if st.selector == nil {
				return nil, qmError("predicate_unsupported", "query operation is not available")
			}
			res, e := h(OperationContext[T]{Statement: st.source.Call, Selector: st.selector, Items: func() ([]T, error) { return selected, nil }, Predicate: MatchAll[T]()})
			if e != nil {
				return nil, e
			}
			results = append(results, res)
			continue
		}
		return nil, qmError("predicate_unsupported", "query operation is not available")
	}
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

func presenceRank(p Presence) int {
	if p == PresencePresent {
		return 0
	}
	if p == PresenceNull {
		return 1
	}
	return 2
}
func compareFieldValues(a, b FieldValue) int {
	if a.Presence != b.Presence {
		if presenceRank(a.Presence) < presenceRank(b.Presence) {
			return -1
		}
		return 1
	}
	if a.Presence != PresencePresent {
		return 0
	}
	left, right := a.Scalar, b.Scalar
	switch a.Kind {
	case FieldString, FieldIdentifier, FieldEnum:
		return strings.Compare(left.String, right.String)
	case FieldInt64:
		if left.Int64 < right.Int64 {
			return -1
		}
		if left.Int64 > right.Int64 {
			return 1
		}
	case FieldBoolean:
		if left.Boolean != right.Boolean {
			if !left.Boolean {
				return -1
			}
			return 1
		}
	case FieldTimestamp:
		if left.Timestamp.Before(right.Timestamp) {
			return -1
		}
		if left.Timestamp.After(right.Timestamp) {
			return 1
		}
	}
	return 0
}
func fieldIdentityKey(v FieldValue) string {
	if v.Presence != PresencePresent {
		return string(v.Presence)
	}
	switch v.Scalar.Kind {
	case FieldString, FieldIdentifier, FieldEnum:
		return fmt.Sprintf("%s:%s", v.Scalar.Kind, v.Scalar.String)
	case FieldInt64:
		return fmt.Sprintf("%s:%d", v.Scalar.Kind, v.Scalar.Int64)
	case FieldBoolean:
		return fmt.Sprintf("%s:%t", v.Scalar.Kind, v.Scalar.Boolean)
	case FieldTimestamp:
		return fmt.Sprintf("%s:%s", v.Scalar.Kind, v.Scalar.Timestamp.UTC().Format(time.RFC3339Nano))
	default:
		return string(v.Scalar.Kind)
	}
}
func groupKey(v FieldValue) GroupKey {
	k := GroupKey{Presence: v.Presence}
	if v.Presence != PresencePresent {
		return k
	}
	k.Value = projectedScalarValue(v.Scalar)
	return k
}
func projectValues(values map[string]FieldValue, fields []string) map[string]any {
	row := make(map[string]any, len(fields))
	for _, name := range fields {
		v := values[name]
		if v.Presence == PresenceMissing {
			continue
		}
		if v.Presence == PresenceNull {
			row[name] = nil
		} else {
			row[name] = projectedFieldValue(v)
		}
	}
	return row
}
func pageBounds[T any](values []T, skip, take *uint64, limits QueryLimits) ([]T, error) {
	start := uint64(0)
	if skip != nil {
		start = *skip
	}
	if start > uint64(len(values)) {
		start = uint64(len(values))
	}
	remaining := uint64(len(values)) - start
	if take == nil && remaining > limits.MaxTopLevelResultsWithoutTake {
		return nil, qmError("result_limit", "result limit exceeded")
	}
	end := uint64(len(values))
	if take != nil && *take < remaining {
		end = start + *take
	}
	return values[int(start):int(end)], nil
}

func projectedFieldValue(value FieldValue) any {
	if value.Kind == FieldScalarArray {
		out := make([]any, 0, len(value.Elements))
		for _, element := range value.Elements {
			out = append(out, projectedScalarValue(element))
		}
		return out
	}
	return projectedScalarValue(value.Scalar)
}

func projectedScalarValue(value ScalarValue) any {
	switch value.Kind {
	case FieldString, FieldIdentifier, FieldEnum:
		return value.String
	case FieldInt64:
		return value.Int64
	case FieldBoolean:
		return value.Boolean
	case FieldTimestamp:
		return value.Timestamp.UTC().Format(time.RFC3339Nano)
	default:
		return nil
	}
}
func chargeWork(used *uint64, add, limit uint64) bool {
	if used == nil || *used > limit || add > limit-*used {
		return false
	}
	*used += add
	return true
}
func chargeRegexWork(e *compiledExpression, vals map[string]FieldValue, used *uint64, limit uint64) bool {
	if e == nil {
		return true
	}
	if e.leaf != nil && e.leaf.regex != nil {
		v := vals[e.leaf.specName]
		if v.Presence == PresencePresent {
			if !chargeWork(used, uint64(1+len(e.leaf.value.Text)+len(v.Scalar.String)), limit) {
				return false
			}
		}
	}
	if !chargeRegexWork(e.child, vals, used, limit) {
		return false
	}
	for _, child := range e.children {
		if !chargeRegexWork(child, vals, used, limit) {
			return false
		}
	}
	return true
}
func validateFieldValue[T any](s QueryFieldSpec[T], v FieldValue, l QueryLimits) error {
	if v.Presence != PresencePresent && v.Presence != PresenceNull && v.Presence != PresenceMissing {
		return qmError("predicate_data", "field data is invalid")
	}
	if v.Presence != PresencePresent {
		if v.Scalar != (ScalarValue{}) || len(v.Elements) != 0 {
			return qmError("predicate_data", "field data is invalid")
		}
		return nil
	}
	if v.Kind != s.Kind {
		return qmError("predicate_data", "field data is invalid")
	}
	if s.Kind != FieldScalarArray {
		if len(v.Elements) != 0 {
			return qmError("predicate_data", "field data is invalid")
		}
		if v.Scalar.Kind != s.Kind {
			return qmError("predicate_data", "field data is invalid")
		}
		if !validScalarValue(v.Scalar) {
			return qmError("predicate_data", "field data is invalid")
		}
		if (s.Kind == FieldString || s.Kind == FieldIdentifier || s.Kind == FieldEnum) && (!utf8.ValidString(v.Scalar.String) || uint64(len(v.Scalar.String)) > l.MaxStoredScalarBytes) {
			return qmError("predicate_data", "field data is invalid")
		}
		if s.Kind == FieldEnum && !registeredEnumValue(s.EnumValues, v.Scalar.String) {
			return qmError("predicate_data", "field data is invalid")
		}
	}
	if s.Kind == FieldScalarArray {
		if v.Scalar != (ScalarValue{}) {
			return qmError("predicate_data", "field data is invalid")
		}
		if uint64(len(v.Elements)) > l.MaxStoredArrayEntries {
			return qmError("predicate_limit", "query limit exceeded")
		}
		for _, x := range v.Elements {
			if x.Kind != s.ElementKind {
				return qmError("predicate_data", "field data is invalid")
			}
			if !validScalarValue(x) {
				return qmError("predicate_data", "field data is invalid")
			}
			if (x.Kind == FieldString || x.Kind == FieldIdentifier || x.Kind == FieldEnum) && (!utf8.ValidString(x.String) || uint64(len(x.String)) > l.MaxStoredScalarBytes) {
				return qmError("predicate_data", "field data is invalid")
			}
			if x.Kind == FieldEnum && !registeredEnumValue(s.EnumValues, x.String) {
				return qmError("predicate_data", "field data is invalid")
			}
		}
	}
	return nil
}

func validScalarValue(v ScalarValue) bool {
	zeroTime := time.Time{}
	switch v.Kind {
	case FieldString, FieldIdentifier, FieldEnum:
		return v.Int64 == 0 && !v.Boolean && v.Timestamp == zeroTime
	case FieldInt64:
		return v.String == "" && !v.Boolean && v.Timestamp == zeroTime
	case FieldBoolean:
		return v.String == "" && v.Int64 == 0 && v.Timestamp == zeroTime
	case FieldTimestamp:
		return v.String == "" && v.Int64 == 0 && !v.Boolean
	default:
		return false
	}
}

func registeredEnumValue(domain []string, value string) bool {
	return containsString(domain, value)
}
func evalExpression[T any](e *compiledExpression, vals map[string]FieldValue, specs map[string]QueryFieldSpec[T]) truth {
	switch e.kind {
	case ExpressionNot:
		v := evalExpression(e.child, vals, specs)
		if v == truthTrue {
			return truthFalse
		}
		if v == truthFalse {
			return truthTrue
		}
		return truthUnknown
	case ExpressionSatisfiesAll:
		r := truthTrue
		for _, c := range e.children {
			v := evalExpression(c, vals, specs)
			if v == truthFalse {
				return truthFalse
			}
			if v == truthUnknown {
				r = truthUnknown
			}
		}
		return r
	case ExpressionSatisfiesAny:
		r := truthFalse
		for _, c := range e.children {
			v := evalExpression(c, vals, specs)
			if v == truthTrue {
				return truthTrue
			}
			if v == truthUnknown {
				r = truthUnknown
			}
		}
		return r
	case ExpressionPredicate:
		return evalPredicate(e.leaf, vals[e.leaf.specName], specs[e.leaf.specName])
	}
	return truthUnknown
}
func evalPredicate[T any](p *compiledPredicate, v FieldValue, s QueryFieldSpec[T]) truth {
	if p.operator == OperatorIsNull {
		if v.Presence == PresenceNull {
			return truthTrue
		}
		return truthFalse
	}
	if p.operator == OperatorIsMissing {
		if v.Presence == PresenceMissing {
			return truthTrue
		}
		return truthFalse
	}
	if v.Presence != PresencePresent {
		return truthUnknown
	}
	if p.operator == OperatorHasElement {
		for _, x := range v.Elements {
			if compareScalarLiteral(x, *p.value) == 0 {
				return truthTrue
			}
		}
		return truthFalse
	}
	x := v.Scalar
	switch p.operator {
	case OperatorEquals:
		if p.legacyFold && p.value.Kind == LiteralString {
			return boolTruth(strings.EqualFold(x.String, p.value.Text))
		}
		return boolTruth(compareScalarLiteral(x, *p.value) == 0)
	case OperatorNotEquals:
		return boolTruth(compareScalarLiteral(x, *p.value) != 0)
	case OperatorContains:
		return boolTruth(strings.Contains(x.String, p.value.Text))
	case OperatorMatchesRegex:
		return boolTruth(p.regex.MatchString(x.String))
	case OperatorLessThan:
		return boolTruth(compareScalarLiteral(x, *p.value) < 0)
	case OperatorLessThanOrEqual:
		return boolTruth(compareScalarLiteral(x, *p.value) <= 0)
	case OperatorGreaterThan:
		return boolTruth(compareScalarLiteral(x, *p.value) > 0)
	case OperatorGreaterThanOrEqual:
		return boolTruth(compareScalarLiteral(x, *p.value) >= 0)
	case OperatorInRange:
		return boolTruth(compareScalarLiteral(x, *p.lower) >= 0 && compareScalarLiteral(x, *p.upper) < 0)
	case OperatorInSet:
		for _, z := range p.values {
			if compareScalarLiteral(x, z) == 0 {
				return truthTrue
			}
		}
		return truthFalse
	}
	return truthUnknown
}
func boolTruth(v bool) truth {
	if v {
		return truthTrue
	}
	return truthFalse
}
func compareScalarLiteral(v ScalarValue, l Literal) int {
	switch l.Kind {
	case LiteralString:
		return strings.Compare(v.String, l.Text)
	case LiteralInt64:
		if v.Int64 < l.Int64 {
			return -1
		}
		if v.Int64 > l.Int64 {
			return 1
		}
	case LiteralBoolean:
		if v.Boolean == l.Boolean {
			return 0
		}
		if !v.Boolean {
			return -1
		}
		return 1
	case LiteralTimestamp:
		t, _ := time.Parse(time.RFC3339Nano, l.Text)
		if v.Timestamp.Before(t) {
			return -1
		}
		if v.Timestamp.After(t) {
			return 1
		}
	}
	return 0
}

func (s *Schema[T]) QueryModelSchema(ctx context.Context, access QueryAccess) (map[string]any, error) {
	s.sealQuery()
	if ctx.Err() != nil {
		return nil, qmError("query_cancelled", "query cancelled")
	}
	if access == nil {
		return nil, qmError("predicate_field", "field is not available")
	}
	if len(s.queryCapabilities) == 0 {
		return nil, qmError("predicate_unsupported", "query operation is not available")
	}
	fields := make([]map[string]any, 0)
	for _, name := range s.queryFieldOrder {
		f := s.queryFields[name]
		if !access.AllowsVisibility(f.Visibility) {
			continue
		}
		item := map[string]any{"name": f.Name, "kind": f.Kind, "operators": append([]PredicateOperator(nil), f.Operators...), "sortable": f.Sortable, "groupable": f.Groupable, "identity": f.Identity}
		if f.Kind == FieldScalarArray {
			item["elementKind"] = f.ElementKind
		}
		fields = append(fields, item)
	}
	caps := make([]QueryOperationCapability, 0, len(s.queryCapabilities))
	for _, c := range s.queryCapabilities {
		if c.Operation == "" || len(c.Clauses) == 0 {
			return nil, qmError("predicate_unsupported", "query operation is not available")
		}
		seen := map[QueryClause]bool{}
		for _, clause := range c.Clauses {
			if !knownClause(clause) || seen[clause] {
				return nil, qmError("predicate_unsupported", "query operation is not available")
			}
			seen[clause] = true
		}
		c.Clauses = append([]QueryClause(nil), c.Clauses...)
		caps = append(caps, c)
	}
	sort.Slice(caps, func(i, j int) bool { return caps[i].Operation < caps[j].Operation })
	return map[string]any{
		"version": 1, "grammarRevision": "agentquery.composable-query.v1",
		"operations": caps,
		"composites": []string{"not", "satisfiesAll", "satisfiesAny"},
		"predicates": []string{"equals", "notEquals", "contains", "matchesRegex", "lessThan", "lessThanOrEqual", "greaterThan", "greaterThanOrEqual", "inRange", "inSet", "hasElement", "isNull", "isMissing"},
		"fields":     fields, "limits": s.queryLimits,
		"modifiers": map[string]any{
			"canonical":            []string{"sortOrder", "groupBy", "skip", "take", "projection"},
			"compatibilityAliases": []string{"flat filter", "sort_FIELD", "skip", "take"},
		},
		"resultShapes": queryModelResultShapes(),
		"errorCodes":   []string{"predicate_syntax", "predicate_unsupported", "predicate_field", "predicate_operator", "predicate_type", "predicate_regex", "query_modifier", "predicate_limit", "predicate_cost", "predicate_data", "result_limit", "query_cancelled"},
		"examples": map[string]any{
			"positive": []string{
				`list() where satisfiesAll(equals(type, "task"), satisfiesAny(contains(name, "predicate"), matchesRegex(description, "(?i)group(?:ing|by)")), not(equals(status, "done"))) { id name status }`,
				`list() where inRange(created, timestamp("2026-09-01T00:00:00+03:00"), timestamp("2026-10-01T00:00:00+03:00")) sortOrder(updated descending, id ascending) skip 20 take 10 { id updated }`,
				`list() where not(isMissing(assignee)) sortOrder(updated descending) groupBy(status) skip 0 take 5 { id name assignee }`,
			},
			"refusals": []string{
				`list() where satisfiesAll()`, `list() where matchesRegex(name, "(?=x)")`, `list() where matchesRegex(status, "x")`, `list() where equals(created, "2026-09-01")`, `list() where inSet(status, [])`, `list() sortOrder(tags ascending)`, `list() groupBy(tags)`, `list() sortOrder(name ascending, name descending)`, `list() skip 100001`, `list() take 0`,
			},
		},
	}, nil
}

const queryModelResultShapesJSON = `{"ungrouped":"array of projected rows","groupedJson":{"key":{"presence":"present|null|missing","value":"present keys only; native JSON type is fixed by groupKeyEncodingTable"},"count":"uint64 before projection","items":"array of projected rows in sorted row order"},"groupedCompactHeader":{"logicalCells":["@group","presence","canonicalJsonTokenOrEmpty","count"],"framing":"existing compact CSV encoder applied exactly once to each logical cell","exampleString":"@group,present,\"\"\"alice\"\"\",2","exampleInt64":"@group,present,-7,1","exampleNull":"@group,null,,1"},"jsonStringEncoding":{"encoder":"Go encoding/json.Marshal applied to the validated string with its default HTML escaping enabled","quoteAndBackslash":"U+0022 and U+005C use the two-byte escapes \\\" and \\\\","c0Controls":"U+0008, U+0009, U+000A, U+000C, and U+000D use \\b, \\t, \\n, \\f, and \\r; every other U+0000..U+001F scalar uses lowercase \\u00xx","nonASCII":"valid non-ASCII Unicode scalars remain their original UTF-8 bytes except U+2028 and U+2029; supplementary-plane scalars are not rewritten as UTF-16 surrogate-pair escapes","lineSeparators":"U+2028 and U+2029 use lowercase \\u2028 and \\u2029","htmlSensitive":"U+003C, U+003E, and U+0026 use lowercase \\u003c, \\u003e, and \\u0026","solidus":"U+002F is not escaped as \\/","application":"the resulting token bytes are emitted verbatim as JSON key.value and become the compact third logical cell before one CSV framing pass"},"groupKeyEncodingTable":[{"keyVariant":"present","fieldKind":"string","jsonType":"string","jsonValue":"validated stored UTF-8 bytes","jsonLexicalEncoding":"resultShapes.jsonStringEncoding","compactLogicalCell":"same exact canonical JSON string-token bytes"},{"keyVariant":"present","fieldKind":"identifier","jsonType":"string","jsonValue":"validated identifier bytes","jsonLexicalEncoding":"resultShapes.jsonStringEncoding","compactLogicalCell":"same exact canonical JSON string-token bytes"},{"keyVariant":"present","fieldKind":"enum","jsonType":"string","jsonValue":"registered canonical enum value","jsonLexicalEncoding":"resultShapes.jsonStringEncoding","compactLogicalCell":"same exact canonical JSON string-token bytes"},{"keyVariant":"present","fieldKind":"int64","jsonType":"number","jsonValue":"minimal base-10 integer with minus only when negative","compactLogicalCell":"same canonical JSON number token"},{"keyVariant":"present","fieldKind":"boolean","jsonType":"boolean","jsonValue":"true or false","compactLogicalCell":"same canonical JSON Boolean token"},{"keyVariant":"present","fieldKind":"timestamp","jsonType":"string","jsonValue":"UTC RFC3339Nano after absolute-instant coalescing","jsonLexicalEncoding":"resultShapes.jsonStringEncoding after UTC RFC3339Nano construction","compactLogicalCell":"same exact canonical JSON string-token bytes"},{"keyVariant":"null","fieldKind":"any groupable scalar","jsonType":"absent","jsonValue":"value member omitted","compactLogicalCell":"empty"},{"keyVariant":"missing","fieldKind":"any groupable scalar","jsonType":"absent","jsonValue":"value member omitted","compactLogicalCell":"empty"}],"groupKeyKindSource":"registered groupBy field; GroupKey carries no caller-controlled kind tag","timestampCanonicalization":"coalesce equal absolute instants, then UTC time.RFC3339Nano with uppercase T/Z and trimmed trailing fractional zeroes","rendererInvariant":"JSON and compact consume one precomputed canonical typed group-key token; neither renderer may select a source member, stringify independently, or choose a different valid JSON escape spelling","groupOrder":"present typed ascending, null, missing","paginationTarget":"rows without groupBy; groups with groupBy"}`

func queryModelResultShapes() map[string]any {
	var result map[string]any
	if err := json.Unmarshal([]byte(queryModelResultShapesJSON), &result); err != nil {
		panic("invalid frozen query-model result shape registry")
	}
	return result
}
