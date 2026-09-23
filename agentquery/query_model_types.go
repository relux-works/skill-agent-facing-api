package agentquery

import (
	"context"
	"time"
)

type QueryModel struct {
	Statements   []QueryStatement `json:"statements"`
	limitDigest  [32]byte
	parsedDigest [32]byte
}

type QueryStatement struct {
	Call      Statement       `json:"call"`
	Where     *Expression     `json:"where,omitempty"`
	SortOrder []SortCriterion `json:"sortOrder,omitempty"`
	GroupBy   *GroupCriterion `json:"groupBy,omitempty"`
	Skip      *uint64         `json:"skip,omitempty"`
	Take      *uint64         `json:"take,omitempty"`
}

type ExpressionKind string

const (
	ExpressionPredicate    ExpressionKind = "predicate"
	ExpressionNot          ExpressionKind = "not"
	ExpressionSatisfiesAll ExpressionKind = "satisfiesAll"
	ExpressionSatisfiesAny ExpressionKind = "satisfiesAny"
)

type Expression struct {
	Kind      ExpressionKind `json:"kind"`
	Predicate *Predicate     `json:"predicate,omitempty"`
	Child     *Expression    `json:"child,omitempty"`
	Children  []*Expression  `json:"children,omitempty"`
	Pos       Pos            `json:"pos"`
}

type Predicate struct {
	Field    string            `json:"field"`
	Operator PredicateOperator `json:"operator"`
	Value    *Literal          `json:"value,omitempty"`
	Values   []Literal         `json:"values,omitempty"`
	Lower    *Literal          `json:"lower,omitempty"`
	Upper    *Literal          `json:"upper,omitempty"`
}

type PredicateOperator string

const (
	OperatorEquals             PredicateOperator = "equals"
	OperatorNotEquals          PredicateOperator = "notEquals"
	OperatorContains           PredicateOperator = "contains"
	OperatorMatchesRegex       PredicateOperator = "matchesRegex"
	OperatorLessThan           PredicateOperator = "lessThan"
	OperatorLessThanOrEqual    PredicateOperator = "lessThanOrEqual"
	OperatorGreaterThan        PredicateOperator = "greaterThan"
	OperatorGreaterThanOrEqual PredicateOperator = "greaterThanOrEqual"
	OperatorInRange            PredicateOperator = "inRange"
	OperatorInSet              PredicateOperator = "inSet"
	OperatorHasElement         PredicateOperator = "hasElement"
	OperatorIsNull             PredicateOperator = "isNull"
	OperatorIsMissing          PredicateOperator = "isMissing"
)

type LiteralKind string

const (
	LiteralString    LiteralKind = "string"
	LiteralInt64     LiteralKind = "int64"
	LiteralBoolean   LiteralKind = "boolean"
	LiteralTimestamp LiteralKind = "timestamp"
)

type Literal struct {
	Kind    LiteralKind `json:"kind"`
	Text    string      `json:"text,omitempty"`
	Int64   int64       `json:"int64,omitempty"`
	Boolean bool        `json:"boolean,omitempty"`
}

type QuerySortDirection string

const (
	QuerySortAscending  QuerySortDirection = "ascending"
	QuerySortDescending QuerySortDirection = "descending"
)

type SortCriterion struct {
	Field     string             `json:"field"`
	Direction QuerySortDirection `json:"direction"`
	Pos       Pos                `json:"pos"`
}
type GroupCriterion struct {
	Field string `json:"field"`
	Pos   Pos    `json:"pos"`
}

type QueryClause string

const (
	QueryClauseWhere      QueryClause = "where"
	QueryClauseSortOrder  QueryClause = "sortOrder"
	QueryClauseGroupBy    QueryClause = "groupBy"
	QueryClauseSkip       QueryClause = "skip"
	QueryClauseTake       QueryClause = "take"
	QueryClauseProjection QueryClause = "projection"
)

type QueryOperationCapability struct {
	Operation string        `json:"operation"`
	Clauses   []QueryClause `json:"clauses"`
}

type QueryLimits struct {
	MaxSourceBytes                uint64
	MaxStatements                 uint64
	MaxExpressionDepth            uint64
	MaxExpressionNodes            uint64
	MaxLiteralBytes               uint64
	MaxRegexBytesPerStatement     uint64
	MaxSetEntries                 uint64
	MaxSortCriteria               uint64
	MaxSnapshotRows               uint64
	MaxSnapshotTypedBytes         uint64
	MaxStoredScalarBytes          uint64
	MaxStoredArrayEntries         uint64
	MaxGroupCardinality           uint64
	MaxSkip                       uint64
	MaxTake                       uint64
	MaxTopLevelResultsWithoutTake uint64
	MaxWorkUnits                  uint64
	MaxResponseBytes              uint64
	ErrorFramingReserveBytes      uint64
}

type QueryResponseAttempt string
type QueryResponseState string

const (
	QueryResponseInitial  QueryResponseAttempt = "initial"
	QueryResponseRetry    QueryResponseAttempt = "retry"
	QueryResponseRenewal  QueryResponseAttempt = "renewal"
	QueryResponseResume   QueryResponseAttempt = "resume"
	QueryResponseRecovery QueryResponseAttempt = "recovery"
	QueryResponsePaused   QueryResponseState   = "paused"
	QueryResponseTerminal QueryResponseState   = "terminal"
)

type FieldKind string

const (
	FieldString      FieldKind = "string"
	FieldIdentifier  FieldKind = "identifier"
	FieldEnum        FieldKind = "enum"
	FieldInt64       FieldKind = "int64"
	FieldBoolean     FieldKind = "boolean"
	FieldTimestamp   FieldKind = "timestamp"
	FieldScalarArray FieldKind = "scalarArray"
)

type Presence string

const (
	PresencePresent Presence = "present"
	PresenceNull    Presence = "null"
	PresenceMissing Presence = "missing"
)

type FieldSensitivity string

const (
	FieldSensitivityPublic FieldSensitivity = "public"
	FieldSensitivitySecret FieldSensitivity = "secret"
)

type QueryFieldSpec[T any] struct {
	Name         string
	Kind         FieldKind
	ElementKind  FieldKind
	Operators    []PredicateOperator
	EnumValues   []string
	Accessor     func(T) (FieldValue, error)
	AccessorCost uint64
	Visibility   string
	Sensitivity  FieldSensitivity
	Sortable     bool
	Groupable    bool
	Identity     bool
}
type ScalarValue struct {
	Kind      FieldKind
	String    string
	Int64     int64
	Boolean   bool
	Timestamp time.Time
}
type FieldValue struct {
	Presence Presence
	Kind     FieldKind
	Scalar   ScalarValue
	Elements []ScalarValue
}
type QueryAccess interface{ AllowsVisibility(label string) bool }
type SnapshotLimits struct {
	MaxRows  uint64
	MaxBytes uint64
}
type QuerySnapshot[T any] struct {
	Items        []T
	DecodedBytes uint64
}
type BoundedSnapshotLoader[T any] func(context.Context, SnapshotLimits) (QuerySnapshot[T], error)
type GroupKey struct {
	Presence Presence `json:"presence"`
	Value    any      `json:"value,omitempty"`
}
type GroupResult struct {
	Key   GroupKey         `json:"key"`
	Count uint64           `json:"count"`
	Items []map[string]any `json:"items"`
}

func Not(child *Expression) *Expression { return &Expression{Kind: ExpressionNot, Child: child} }
func SatisfiesAll(children ...*Expression) *Expression {
	return &Expression{Kind: ExpressionSatisfiesAll, Children: children}
}
func SatisfiesAny(children ...*Expression) *Expression {
	return &Expression{Kind: ExpressionSatisfiesAny, Children: children}
}
func leaf(field string, op PredicateOperator, value *Literal) *Expression {
	return &Expression{Kind: ExpressionPredicate, Predicate: &Predicate{Field: field, Operator: op, Value: value}}
}
func Equals(field string, value Literal) *Expression { return leaf(field, OperatorEquals, &value) }
func NotEquals(field string, value Literal) *Expression {
	return leaf(field, OperatorNotEquals, &value)
}
func Contains(field string, value Literal) *Expression { return leaf(field, OperatorContains, &value) }
func MatchesRegex(field string, pattern Literal) *Expression {
	return leaf(field, OperatorMatchesRegex, &pattern)
}
func LessThan(field string, value Literal) *Expression { return leaf(field, OperatorLessThan, &value) }
func LessThanOrEqual(field string, value Literal) *Expression {
	return leaf(field, OperatorLessThanOrEqual, &value)
}
func GreaterThan(field string, value Literal) *Expression {
	return leaf(field, OperatorGreaterThan, &value)
}
func GreaterThanOrEqual(field string, value Literal) *Expression {
	return leaf(field, OperatorGreaterThanOrEqual, &value)
}
func InRange(field string, lower, upper Literal) *Expression {
	return &Expression{Kind: ExpressionPredicate, Predicate: &Predicate{Field: field, Operator: OperatorInRange, Lower: &lower, Upper: &upper}}
}
func InSet(field string, values ...Literal) *Expression {
	return &Expression{Kind: ExpressionPredicate, Predicate: &Predicate{Field: field, Operator: OperatorInSet, Values: values}}
}
func HasElement(field string, value Literal) *Expression {
	return leaf(field, OperatorHasElement, &value)
}
func IsNull(field string) *Expression    { return leaf(field, OperatorIsNull, nil) }
func IsMissing(field string) *Expression { return leaf(field, OperatorIsMissing, nil) }
func StringValue(value string) Literal   { return Literal{Kind: LiteralString, Text: value} }
func Int64Value(value int64) Literal     { return Literal{Kind: LiteralInt64, Int64: value} }
func BooleanValue(value bool) Literal    { return Literal{Kind: LiteralBoolean, Boolean: value} }
func TimestampValue(value time.Time) Literal {
	return Literal{Kind: LiteralTimestamp, Text: value.Format(time.RFC3339Nano)}
}
