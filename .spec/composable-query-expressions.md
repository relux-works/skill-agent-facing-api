# Composable Query Expressions v1

Status: operation-capability surface revision-8 candidate for independent review

Story: `STORY-260908-3ieod5`

Architecture task: `TASK-260923-37ss7m`

Grammar revision: `agentquery.composable-query.v1`

## 1. Authority and scope

This specification is the repository source of truth for the first shared
`agentquery` implementation of composable predicates and query modifiers. It
implements the refreshed owner requirements in
`TASK-260923-37ss7m_product-brief.md`.

The revision-3 external research contract remains supporting evidence for
typed values, three-valued presence semantics, authorization order, bounded
execution, stable sorting, pagination, compatibility routing, and typed error
redaction. The following revision-3 restrictions are explicitly superseded:

- regular expressions are required through `matchesRegex`;
- grouping is required through one `groupBy` criterion;
- the public textual predicate grammar is the functional grammar below, not
  the earlier infix `where (...)` grammar;
- the implementation board is the four-task delivery sequence already under
  `STORY-260908-3ieod5`; no generic oracle or additional research prerequisite
  is required.

Consumer-specific task-board registration, transport, authorization, release,
and deployment remain downstream of this Story's two shared runtime slices.
Unrestricted reflection, object paths, network accessors, backtracking regex
engines, multiple grouping criteria, aggregates other than group `count`, and
pagination within a selected group are out of scope for v1.

Normative machine-readable limits, public identifiers, field/operator rows,
result shapes, and conformance cases are frozen in
[`composable-query-expression-vectors.json`](composable-query-expression-vectors.json).
That resource is also the implementation gate for the textual grammar and
public Go surface: its `grammar`, `publicTypes`, `publicConstants`,
`publicFunctions`, and `publicMethods` registries are exhaustive, not examples.

## 2. Required result pipeline

An opted-in operation executes one statement in this exact order:

1. authorize the operation and compile the complete query model, including
   projection, every expression branch, sort criteria, grouping, and paging;
2. load one consistent bounded snapshot;
3. materialize and validate every referenced typed field once per row;
4. retain rows whose expression evaluates to `true`;
5. apply `sortOrder`, appending the registered identity field ascending when
   the caller did not already specify it;
6. when present, apply `groupBy` to the sorted rows;
7. apply `skip` and `take` to the top-level collection (rows without grouping,
   groups with grouping);
8. project item fields and render the result.

Compilation failure performs no snapshot load. Snapshot, evaluation, grouping,
or rendering failure returns no partial result for that statement. A read batch
may retain earlier statement results only after every statement has passed the
request-wide preflight.

When `sortOrder` is omitted, the effective order is the identity field
ascending. Group members retain that row order. Group records themselves use
the key order in section 8 and never first-seen order.

## 3. Public Go model

The existing exported `Query`, `Statement`, `Arg`, `Parse`, `Render`,
`Schema.Query`, `FilterableField`, `SortSlice`, and `PaginateSlice` contracts
are not changed. In particular, no field is added to `Statement` or `Arg`, so
existing unkeyed composite literals remain source-compatible.

The additive public model uses these identifiers and field names:

```go
type QueryModel struct {
    Statements []QueryStatement `json:"statements"`
}

type QueryStatement struct {
    Call      Statement        `json:"call"`
    Where     *Expression      `json:"where,omitempty"`
    SortOrder []SortCriterion  `json:"sortOrder,omitempty"`
    GroupBy   *GroupCriterion  `json:"groupBy,omitempty"`
    Skip      *uint64          `json:"skip,omitempty"`
    Take      *uint64          `json:"take,omitempty"`
}

type ExpressionKind string

const (
    ExpressionPredicate   ExpressionKind = "predicate"
    ExpressionNot         ExpressionKind = "not"
    ExpressionSatisfiesAll ExpressionKind = "satisfiesAll"
    ExpressionSatisfiesAny ExpressionKind = "satisfiesAny"
)

type Expression struct {
    Kind      ExpressionKind `json:"kind"`
    Predicate *Predicate     `json:"predicate,omitempty"`
    Child     *Expression    `json:"child,omitempty"`
    Children  []*Expression `json:"children,omitempty"`
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

    QueryResponsePaused   QueryResponseState = "paused"
    QueryResponseTerminal QueryResponseState = "terminal"
)

type QueryResponseRequest[T any] struct { /* unexported fields */ }

func DefaultQueryLimits() QueryLimits
```

`Expression` is a tagged union. Exactly one payload is legal: `Predicate` for
`predicate`, `Child` for `not`, and a non-empty `Children` slice for
`satisfiesAll` or `satisfiesAny`. Nil children, empty composites, extra payload
fields, unknown tags, cycles, and excessive trees are `predicate_syntax`.
Shared acyclic subtrees are allowed and count once per occurrence. Source
positions are trusted only for nodes parsed from the current text; positions on
caller-constructed ASTs are ignored in errors.

The public constructor family returns `*Expression` and is exactly:

```go
func Not(child *Expression) *Expression
func SatisfiesAll(children ...*Expression) *Expression
func SatisfiesAny(children ...*Expression) *Expression

func Equals(field string, value Literal) *Expression
func NotEquals(field string, value Literal) *Expression
func Contains(field string, value Literal) *Expression
func MatchesRegex(field string, pattern Literal) *Expression
func LessThan(field string, value Literal) *Expression
func LessThanOrEqual(field string, value Literal) *Expression
func GreaterThan(field string, value Literal) *Expression
func GreaterThanOrEqual(field string, value Literal) *Expression
func InRange(field string, lower, upper Literal) *Expression
func InSet(field string, values ...Literal) *Expression
func HasElement(field string, value Literal) *Expression
func IsNull(field string) *Expression
func IsMissing(field string) *Expression

func StringValue(value string) Literal
func Int64Value(value int64) Literal
func BooleanValue(value bool) Literal
func TimestampValue(value time.Time) Literal
```

`TimestampValue` renders an RFC 3339 timestamp with an explicit offset and up
to nanosecond precision. A parsed timestamp retains its original valid offset
for rendering but compares as an absolute instant.

The additive entry points are:

```go
func ParseQueryModel(input string, limits QueryLimits) (*QueryModel, error)
func RenderQueryModel(model *QueryModel) (string, error)

func (s *Schema[T]) SetQueryLimits(limits QueryLimits) error
func (s *Schema[T]) QueryLimits() QueryLimits

func (s *Schema[T]) CompileQueryModel(
    ctx context.Context,
    model *QueryModel,
    access QueryAccess,
) (*CompiledQuery[T], error)

func (s *Schema[T]) QueryModelAST(
    ctx context.Context,
    model *QueryModel,
    access QueryAccess,
) (any, error)

func (s *Schema[T]) QueryModelJSONASTWithMode(
    ctx context.Context,
    model *QueryModel,
    access QueryAccess,
    mode OutputMode,
) ([]byte, error)

func (s *Schema[T]) BeginQueryModelResponse(
    ctx context.Context,
    model *QueryModel,
    access QueryAccess,
) (*QueryResponseRequest[T], error)

func (r *QueryResponseRequest[T]) QueryModelAST(
    ctx context.Context,
    attempt QueryResponseAttempt,
) (QueryResponseState, error)

func (r *QueryResponseRequest[T]) QueryModelJSONASTWithMode(
    ctx context.Context,
    attempt QueryResponseAttempt,
    mode OutputMode,
) (QueryResponseState, error)

func (r *QueryResponseRequest[T]) PublishQueryModelAST() (any, error)

func (r *QueryResponseRequest[T]) PublishQueryModelJSONAST() ([]byte, error)

func (s *Schema[T]) RegisterQueryOperationCapability(
    capability QueryOperationCapability,
) error

func (s *Schema[T]) QueryModelSchema(
    ctx context.Context,
    access QueryAccess,
) (map[string]any, error)
```

`NewSchema` initializes the schema with `DefaultQueryLimits()`. `SetQueryLimits`
is the only schema-level configuration route and uses the normalization rules
in section 11. It may be called repeatedly during setup, but the first
`CompileQueryModel`, `BeginQueryModelResponse`, `QueryModelAST`, or
`QueryModelJSONASTWithMode` call seals
the effective limits. A later setter call returns `predicate_unsupported` and
leaves the prior configuration unchanged. `QueryLimits` returns the normalized
effective copy used by every stage and by schema discovery.

`QueryResponseRequest` is an opaque request owner, not serialized caller
evidence. `BeginQueryModelResponse` fixes the schema identity, model, access
policy, normalized-limit digest, and one initially unselected output mode. It
performs whole-model compilation, authorization, capability validation, and
request-wide preflight before any statement can execute. Its first dispatch
must use `QueryResponseInitial`; a retry, renewal, resume, or recovery label on
a fresh request is `predicate_unsupported`, and a second initial label is
likewise refused.

Each successful dispatch executes and atomically serializes exactly one next
statement. If another preflighted statement remains, the method returns
`QueryResponsePaused, nil`; this returned production state is the only event
that admits one later retry, renewal, resume, or recovery dispatch on the same
request. If the dispatched statement is the last statement, or if it latches a
typed refusal, the method returns `QueryResponseTerminal` with the corresponding
nil or non-nil error. No fixture, caller-supplied flag, attempt label, context
value, or private-state mutation can cause `QueryResponsePaused`. The first
dispatch selects native, JSON, or compact mode, and every later dispatch and
publication must keep that mode. No dispatch can substitute another model,
access policy, schema, or limit digest. Retry, renewal, resume, and recovery are
auditable continuation labels over the same deterministic next-statement step;
none replays registration into a replacement schema or re-executes a committed
statement.

The two request dispatch methods return only the observable state and execution
error; they never return or write response bytes or a native result. Successful
statement records and a latched terminal error remain in one unpublished
request-owned cumulative buffer. Exactly one matching `PublishQueryModelAST` or
`PublishQueryModelJSONAST` call closes the framing and makes the cumulative
response observable. Publication before a terminal success/error state, through
the wrong output family, or for a second time is `predicate_unsupported` and
exposes no bytes. After publication every retry, renewal, resume, or recovery is
refused before execution. The one-shot `Schema.QueryModelAST` and
`Schema.QueryModelJSONASTWithMode` methods are source-compatible wrappers for
begin, initial dispatch, internal stepping to terminal on that same request, and
the matching single publication. The wrapper's internal loop calls the same
next-statement step and cannot allocate a replacement ledger. A second wrapper
invocation is a genuinely new request; only reuse of an unpublished opaque
request object denotes a continuation.

`ParseQueryModel` normalizes its argument with the same rules and privately
records the effective-limit digest in the returned model. A transport obtains
that argument from `Schema.QueryLimits()`. Compilation rejects a parsed model
whose non-empty digest differs from the sealed schema digest as
`predicate_limit`; it does not silently rebind it. A caller-constructed model
has no parser digest and is instead completely validated under the schema's
sealed limits. The digest is private metadata and is neither serialized nor
rendered.

`CompiledQuery[T]` is exported but opaque: its fields are unexported and it is
created only by `CompileQueryModel`. It carries the schema identity and sealed
effective-limit digest. Bounded loading, field materialization, evaluation,
sorting, grouping, pagination, and rendering all consume that same normalized
limit set, and execution rejects a schema/digest mismatch. A caller-supplied
boolean or deserialized marker can never bypass compilation.

`QuerySortDirection` is intentionally distinct from the existing exported
`SortDirection int` used by `SortSpec`. Reusing that name for a new string type
would be a package-scope collision; changing the existing type or its `Asc` and
`Desc` constants would break the additive compatibility promise. Compatibility
lowering converts legacy `SortDirection` values into `QuerySortDirection`
values before the canonical compiler runs.

`RegisterQueryOperationCapability` is the only canonical-clause opt-in route.
It accepts an operation already registered through `Operation` or
`OperationWithMetadata` and an unordered, duplicate-free set of `QueryClause`
values. Registration copies and canonicalizes the set into the clause order in
section 5. An empty operation name, unknown operation, empty clause set,
unknown clause, duplicate clause, or duplicate capability registration refuses
atomically before the capability catalog changes. The first compilation,
execution, or `QueryModelSchema` call seals both the field and capability
catalogs; later capability registration returns `predicate_unsupported` and
does not mutate discovery.

Unknown or empty operations, duplicate capability registration, and
registration after sealing return `predicate_unsupported`. Empty, unknown, or
duplicate clause sets return `query_modifier`. All use fixed messages without
enumerating registered operation names or hidden field metadata.

Capability is explicit data, never inferred from an operation name. A host
that wants the owner examples registers `list` with all six clauses and
`count` with `QueryClauseWhere` only. A custom operation may register any
non-empty subset. The same registered set controls canonical clauses and their
legacy aliases. An operation with no capability continues to accept its legacy
syntax unchanged, but any canonical clause or compatibility alias that would
lower into one returns `predicate_unsupported` before field lookup or load.
The check runs at begin-time preflight and again at the production execution
step for every initial, retry, renewal, resume, and recovery attempt. Every
attempt uses the exact sealed `Schema` captured by the opaque request; no
continuation replays registrations into a replacement schema. Request text, a
persisted operation name, an attempt kind, or a freshly constructed schema can
never manufacture a capability or replace the request owner.

## 4. Registered typed fields

The new registration surface is:

```go
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

func (s *Schema[T]) RegisterQueryField(spec QueryFieldSpec[T]) error

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

type QueryAccess interface {
    AllowsVisibility(label string) bool
}

type SnapshotLimits struct {
    MaxRows  uint64
    MaxBytes uint64
}

type QuerySnapshot[T any] struct {
    Items        []T
    DecodedBytes uint64
}

type BoundedSnapshotLoader[T any] func(
    context.Context,
    SnapshotLimits,
) (QuerySnapshot[T], error)

func (s *Schema[T]) SetQuerySnapshotLoader(loader BoundedSnapshotLoader[T])
```

`FieldValue` is the public tagged value containing one `Presence` and, only
when present, one scalar or homogeneous scalar array matching the registered
kind. Null array elements, nested arrays, wrong stored kinds, invalid UTF-8,
and accessor failures are `predicate_data`, never missing.

`Visibility` is a required schema-owned policy label. `Sensitivity` is also
required and is exactly `FieldSensitivityPublic` or
`FieldSensitivitySecret`; its zero value and unknown values are invalid.
`QueryAccess` is the trusted host policy that admits visibility labels for the
current caller. Visibility authorization and sensitivity are independent:
authorization may allow a caller to project, filter, or sort a secret-bearing
field when its operator flags permit that use, but it can never make the field
groupable.

`RegisterQueryField` rejects `FieldSensitivitySecret` with `Groupable: true`
as `query_modifier` with the fixed message `secret-bearing field cannot be
groupable`. This setup-time invariant is checked before the field enters the
catalog; it does not depend on `QueryAccess`, and no later authorization path
can override it. Zero or unknown `Sensitivity` values return `predicate_type`
before catalog or discovery insertion; they are never defaulted to public.
There is no implicit allow-all policy. Projection,
expression, sort, group, and introspection use the same visibility policy.
Unknown and unauthorized field names are indistinguishable to callers.

An opted-in schema has exactly one scalar identity field. Its accessor must
produce a present unique value for every row. Missing identity registration or
duplicate snapshot identities refuses the canonical pipeline. Accessors are
pure, have a fixed declared cost of at least one, and perform no file or network
I/O.

### Operator matrix

| Field kind | Equality | Contains | Regex | Ordered comparisons / `inRange` | `inSet` | `hasElement` | Presence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| string | yes | yes | yes | yes | yes | no | yes |
| identifier | yes | yes | yes | yes | yes | no | yes |
| enum | yes | no | no | no | yes | no | yes |
| int64 | yes | no | no | yes | yes | no | yes |
| boolean | yes | no | no | no | yes | no | yes |
| timestamp | yes | no | no | yes | yes | no | yes |
| homogeneous scalar array | no | no | no | no | no | yes | yes |

Registration may remove operators from a row but cannot add an operator the row
forbids. Arrays are neither sortable nor groupable. Secret-bearing fields are
never groupable regardless of visibility label or caller authorization; a
non-public visibility label remains recommended but is not the enforcement
mechanism.

Strings and identifiers compare and order by exact UTF-8 bytes after stored
value validation. No Unicode normalization, locale collation, or case folding
is implicit. Enums group in lexical canonical-value order. Integers use numeric
order, Booleans use `false < true`, and timestamps use absolute instant order.

## 5. Frozen textual grammar

The new parser consumes the original bytes once. It extends the existing
operation-call and projection grammar with the following postfix clauses:

```ebnf
query         = [ semicolons ] statement
                { semicolons statement } [ semicolons ] ;
semicolons    = ";" { ";" } ;

statement     = operation "(" [ legacyArgs ] ")"
                [ whereClause ] [ sortClause ] [ groupClause ]
                [ skipClause ] [ takeClause ] [ projection ] ;

operation     = legacyIdentifier ;
legacyArgs    = legacyArg { "," legacyArg } ;
legacyArg     = legacyValue | legacyKey "=" legacyValue ;
legacyKey     = legacyIdentifier ;
legacyValue   = legacyIdentifier | legacyString ;
legacyIdentifier = legacyIdentifierStart { legacyIdentifierContinue } ;
legacyIdentifierStart = ASCII_LETTER | DIGIT | "_" ;
legacyIdentifierContinue = legacyIdentifierStart | "-" | "." | "/" ;
projection    = "{" { legacyIdentifier } "}" ;

whereClause   = "where" expression ;
expression    = composite | predicate ;
composite     = "not" "(" expression ")"
              | "satisfiesAll" "(" expression { "," expression } ")"
              | "satisfiesAny" "(" expression { "," expression } ")" ;

predicate     = "equals" "(" field "," scalar ")"
              | "notEquals" "(" field "," scalar ")"
              | "contains" "(" field "," string ")"
              | "matchesRegex" "(" field "," string ")"
              | "lessThan" "(" field "," scalar ")"
              | "lessThanOrEqual" "(" field "," scalar ")"
              | "greaterThan" "(" field "," scalar ")"
              | "greaterThanOrEqual" "(" field "," scalar ")"
              | "inRange" "(" field "," scalar "," scalar ")"
              | "inSet" "(" field "," setLiteral ")"
              | "hasElement" "(" field "," scalar ")"
              | "isNull" "(" field ")"
              | "isMissing" "(" field ")" ;

sortClause    = "sortOrder" "(" sortCriterion { "," sortCriterion } ")" ;
sortCriterion = field ( "ascending" | "descending" ) ;
groupClause   = "groupBy" "(" field ")" ;
skipClause    = "skip" unsignedInteger ;
takeClause    = "take" unsignedInteger ;

setLiteral    = "[" scalar { "," scalar } "]" ;
scalar        = string | signedInteger | "true" | "false"
              | "timestamp" "(" string ")" ;
string        = '"' { UTF8_SCALAR_EXCEPT_QUOTE_BACKSLASH_C0
              | "\\" ( '"' | "\\" | "n" | "t" | OTHER_UTF8_SCALAR ) } '"' ;
signedInteger = [ "-" ] decimalMagnitude ;
unsignedInteger = decimalMagnitude ;
decimalMagnitude = "0" | NONZERO_DIGIT { DIGIT } ;
field         = ( ASCII letter | "_" ) { ASCII letter | digit | "_" } ;
```

The query contains at least one statement. Leading, repeated, and trailing
semicolons remain compatibility syntax; empty input and semicolon-only input
remain invalid. `legacyString` is the quoted-string token of the baseline
parser at commit `c5a6fb45d6e028377d6f7c6e006cd8ce5c0d0830`: it uses the same
four decoded escapes and preserves an unknown backslash escape byte-for-byte.
Whitespace may appear between tokens but never inside an unquoted token.

Expression integers use ASCII decimal digits only. A leading `+`, leading zero
on a multi-digit value, and `-0` are refused as `predicate_syntax`.
`signedInteger` must fit `int64` or returns `predicate_type`;
`unsignedInteger` must fit `uint64` or returns `predicate_limit`. The parser
does not infer numeric types for `legacyValue`: legacy bare `42` remains the
string value `"42"` until the compatibility lowerer interprets the operation's
argument contract.

Clause order is fixed. Each clause occurs at most once. `satisfiesAll`,
`satisfiesAny`, `sortOrder`, and `inSet` require at least one element. There is
no implicit AND, field path, field-to-field comparison, arithmetic, wildcard,
comment, bare null literal, chained comparison, or trailing comma.

String literals reuse the legacy decoder: `\"`, `\\`, `\n`, and `\t` are
decoded; an unknown escape retains the backslash and following byte. Raw input
must be valid UTF-8. Raw C0 controls are refused inside expression literals;
decoded tab/newline remain legal. Regex backslashes therefore render
canonically doubled without a second embedded expression language.

Timestamp text is the strict revision-3 subset: four-digit year `0001..9999`,
valid Gregorian date, uppercase `T`, uppercase `Z` or numeric `+/-HH:MM`
offset, seconds `00..59`, and optional 1..9 fractional digits. Leap seconds,
lowercase `t/z`, `-00:00`, local/date-only values, named zones, `24:00`, invalid
dates, and excess precision are `predicate_type`.

### Positive examples

```text
list() where satisfiesAll(
  equals(type, "task"),
  satisfiesAny(
    contains(name, "predicate"),
    matchesRegex(description, "(?i)group(?:ing|by)")
  ),
  not(equals(status, "done"))
) { id name status }
```

```text
list() where inRange(
  created,
  timestamp("2026-09-01T00:00:00+03:00"),
  timestamp("2026-10-01T00:00:00+03:00")
) sortOrder(updated descending, id ascending) skip 20 take 10 { id updated }
```

```text
list() where not(isMissing(assignee))
  sortOrder(updated descending)
  groupBy(status)
  skip 0 take 5
  { id name assignee }
```

### Refusal examples

| Input | Stable refusal | Reason |
| --- | --- | --- |
| `list() where satisfiesAll()` | `predicate_syntax` | empty composite |
| `list() where matchesRegex(name, "(?=x)")` | `predicate_regex` | unsupported Go/RE2 construct |
| `list() where matchesRegex(status, "x")` | `predicate_operator` | field does not allow regex |
| `list() where equals(created, "2026-09-01")` | `predicate_type` | string is not a timestamp literal |
| `list() where inSet(status, [])` | `predicate_syntax` | empty finite set |
| `list() sortOrder(tags ascending)` | `query_modifier` | arrays are not sortable |
| `list() groupBy(tags)` | `query_modifier` | arrays are not groupable |
| `list() sortOrder(name ascending, name descending)` | `query_modifier` | duplicate criterion |
| `list() skip 100001` | `predicate_limit` | skip bound exceeded |
| `list() take 0` | `query_modifier` | explicit take must be positive |

## 6. Predicate semantics

`inRange(field, lower, upper)` is half-open and equivalent to
`greaterThanOrEqual(field, lower)` AND `lessThan(field, upper)`. Equal or
inverted bounds select no present value; they are not syntax errors.

`inSet` is finite scalar membership. Entries must have one kind compatible
with the field. Duplicates are semantically redundant but count toward limits.
`hasElement` is existential equality against one homogeneous array element.
Empty arrays match no element and are distinct from null and missing arrays.

Presence has three states: present, explicit null, and missing. Ordinary leaf
predicates on null or missing evaluate to `unknown`. `isNull` and `isMissing`
return ordinary true/false values. Only `true` selects a row.

| AND | true | false | unknown |
| --- | --- | --- | --- |
| true | true | false | unknown |
| false | false | false | false |
| unknown | unknown | false | unknown |

| OR | true | false | unknown |
| --- | --- | --- | --- |
| true | true | true | true |
| false | true | false | unknown |
| unknown | true | unknown | unknown |

`not(true)=false`, `not(false)=true`, and `not(unknown)=unknown`.
Preparation validates every referenced field for every candidate row even when
runtime Boolean evaluation could short-circuit.

## 7. Bounded regular expressions

`matchesRegex` is available only for registered string and identifier fields
that explicitly include that operator. It uses Go's `regexp.Compile` once
during query compilation and `Regexp.MatchString` during evaluation. Go's
package accepts RE2 syntax and guarantees linear-time matching; `MatchString`
reports a match anywhere in the subject, so matching is substring search unless
the pattern supplies anchors. See the official
[`regexp` documentation](https://pkg.go.dev/regexp) and
[`regexp/syntax` reference](https://pkg.go.dev/regexp/syntax).

The cumulative decoded regex pattern budget is 1024 bytes per statement.
Patterns also count toward the ordinary 4096-byte-per-literal limit. Inline Go
RE2 flags `i`, `m`, `s`, and `U`, including scoped forms, are allowed. Matching
is case-sensitive unless the pattern enables `i`. Backreferences, look-around,
recursion, and every other construct rejected by Go `regexp` are refused.

Invalid or unsupported syntax returns `predicate_regex` before snapshot load
without echoing the pattern or engine diagnostic. Exceeding regex bytes returns
`predicate_limit`. Matching null or missing produces `unknown`. Work charging
adds `1 + patternBytes + subjectBytes` per attempted match; subject bytes are
also bounded by the stored-value ceiling.

## 8. Sorting, grouping, pagination, and rendering

`sortOrder` accepts at most four explicit scalar criteria. Directions are
caller order. Duplicate fields, invalid directions, invisible fields,
non-sortable fields, and arrays refuse before load. The identity criterion is
appended ascending unless already explicit. Present values follow the typed
orders in section 4. Null and missing sort after all present values in both
directions, with null before missing.

`groupBy` accepts zero or one registered scalar groupable field. It runs after
row sorting. Present groups order by typed ascending key; null and missing are
separate final groups in that order. Group cardinality is bounded before
pagination. Group counts and membership are computed before projection.

The logical JSON result type is:

```go
type GroupKey struct {
    Presence Presence `json:"presence"`
    Value    any      `json:"value,omitempty"`
}

type GroupResult struct {
    Key   GroupKey        `json:"key"`
    Count uint64          `json:"count"`
    Items []map[string]any `json:"items"`
}
```

Present keys include `value`; null and missing keys omit it. The grouping
field's registered `FieldKind` supplies the type context, so `GroupKey` does
not repeat a caller-controlled kind tag. Present values use this exhaustive
wire table:

| Groupable `FieldKind` | JSON `key.value` | Compact logical key cell |
| --- | --- | --- |
| `string` | JSON string containing the validated stored UTF-8 bytes | the same exact canonical JSON string-token bytes |
| `identifier` | JSON string containing the validated identifier bytes | the same exact canonical JSON string-token bytes |
| `enum` | JSON string containing the registered canonical enum value | the same exact canonical JSON string-token bytes |
| `int64` | JSON number in minimal base-10 form, with `-` only for negative values | the same canonical JSON number token |
| `boolean` | JSON Boolean `true` or `false` | the same canonical JSON Boolean token |
| `timestamp` | JSON string containing canonical UTC RFC3339Nano | the same exact canonical JSON string-token bytes |
| null | `{"presence":"null"}`; `value` is absent | empty cell |
| missing | `{"presence":"missing"}`; `value` is absent | empty cell |

JSON numbers are not quoted, and JSON Booleans are not quoted. String,
identifier, enum, and timestamp values are JSON strings rather than bare text.
Their lexical token is exactly the result of Go `encoding/json.Marshal` on the
validated string with the encoder's default HTML escaping enabled. Quote and
backslash use `\"` and `\\`. Backspace, tab, newline, form feed, and carriage
return use `\b`, `\t`, `\n`, `\f`, and `\r`; every other U+0000..U+001F
scalar uses lowercase `\u00xx`. Valid non-ASCII scalars remain their original
UTF-8 bytes except U+2028 and U+2029, which use lowercase `\u2028` and
`\u2029`. Every other valid non-ASCII scalar, including supplementary-plane
scalars, remains its original UTF-8 bytes and is never rewritten as a UTF-16
surrogate-pair escape. The HTML-sensitive `<`, `>`, and `&` characters use
lowercase `\u003c`, `\u003e`, and `\u0026`; `/` is not escaped as `\/`. No
renderer may choose another JSON-equivalent escape spelling.

For timestamps, grouping first compares and coalesces absolute instants, then
renders that instant in UTC using Go's `time.RFC3339Nano` form: uppercase `T`
and `Z`, no fractional part when nanoseconds are zero, otherwise 1..9 digits
with trailing fractional zeroes removed. A source offset is never a group
representative. Thus `2026-09-01T03:04:05.120000000+03:00` and
`2026-09-01T00:04:05.12Z` form one group whose value is exactly
`"2026-09-01T00:04:05.12Z"` in both modes.

Example:

```json
{
  "key": {"presence": "present", "value": "analysis"},
  "count": 3,
  "items": [{"id": "TASK-1"}, {"id": "TASK-2"}, {"id": "TASK-3"}]
}
```

Projection applies only to `items`. The group key remains represented even when
the grouping field is absent from the item projection.

Compact grouped output is a sequence of deterministic blocks separated by one
blank line. Each block starts with
`@group,<presence>,<canonical-json-value-or-empty>,<count>`, followed by the
ordinary compact item header and rows. Null and missing use an empty value
column; presence disambiguates them. An empty selected group page has no blocks.
The header is encoded as four cells by the existing compact CSV encoder. The
third logical cell is the exact canonical JSON token from the table, after
which ordinary CSV quoting is applied once. For example, the logical string
token `"alice"` produces `@group,present,"""alice""",2`, while an integer
produces `@group,present,-7,1` and null produces `@group,null,,1`. JSON and
compact rendering consume one precomputed canonical typed group-key token;
neither mode may re-read an arbitrary source member, independently stringify
the value, or choose a different valid escape spelling. For the logical value
containing, in order, `q`, quote, backslash, backspace, form feed, newline,
carriage return, tab, U+0001, `é`, U+2028, U+2029, `<`, `&`, `>`, `/`, and
U+1F600 (`😀`), the exact JSON token is
`"q\"\\\b\f\n\r\t\u0001é\u2028\u2029\u003c\u0026\u003e/😀"`; after one CSV
framing pass the exact compact header is
`@group,present,"""q\""\\\b\f\n\r\t\u0001é\u2028\u2029\u003c\u0026\u003e/😀""",1`.

`QueryModelSchema(...)["resultShapes"]` publishes the complete eight-row table,
the grouped JSON shape, the four compact header cells, CSV framing, canonical
timestamp rule, the `encoding/json.Marshal` lexical string policy, ordering,
and pagination target. Consumers never infer grouping or key types from
transport accidents.

The group-key boundary has these refusal and unrepresentability invariants:

| Adversary / entry point | Required result before output |
| --- | --- |
| text, external AST, retry, renewal, resume, or recovery requests an array group key | `query_modifier`; no renderer input |
| registration supplies an unknown or unsupported `FieldKind` | atomic `predicate_type`; no catalog entry |
| registration makes a secret-bearing field groupable | atomic `query_modifier`; no catalog entry |
| any request, including retry/renewal/resume/recovery, names an unauthorized group key | `predicate_field` before snapshot load |
| text supplies a second criterion | `predicate_syntax`; the public model has only one `*GroupCriterion` and cannot represent it |
| a provider supplies equal timestamp instants with different offsets | one UTC canonical group key; a member offset cannot reach output |
| a renderer attempts per-mode conversion, stringifies every present key, or chooses a different valid escape spelling | impossible through the shared canonical key token and detected by the raw-byte `18/18` grouped-render gate |

`skip` defaults to zero. Explicit `take` is `1..1000`; omission means no
caller-specified page size, but more than 1000 selected top-level records returns
`result_limit` rather than truncating. `skip` is `0..100000`. In a grouped query,
these controls select groups and never remove items from a selected group.

V1 operation eligibility is the sealed `QueryOperationCapability` catalog.
The standard host registration gives `list` all six `QueryClause` values and
gives `count` only `QueryClauseWhere`; these are fixture registrations, not
special cases in the compiler. Thus `count() where ...` is admitted while its
sort, group, skip, take, and projection clauses refuse before load. A custom
operation is admitted exactly for its registered subset. An unregistered
operation with any canonical clause or lowering alias returns
`predicate_unsupported` rather than silently ignoring it. Retry, resume, and
recovery reuse the sealed catalog and may not manufacture a default capability.
For a where-only capability, admission refuses each of `sortOrder`, `groupBy`,
`skip`, `take`, and projection independently. The same refusal occurs after
lowering `sort_FIELD`, argument-form `skip`, and argument-form `take`; lowering
is not a bypass around capability admission. These clause and alias refusals
happen before field resolution and snapshot load. A where clause that passes
capability admission still resolves fields through `QueryAccess`: unknown and
unauthorized fields share `predicate_field`, the fixed non-enumerating message,
zero loads, and no catalog-bearing details.

## 9. Compatibility and lowering

The new parser accepts all legacy operation calls. For an operation whose
registered capability admits the target clause:

- registered flat filters lower to internal leaf nodes and AND with `Where`;
- current case-insensitive legacy string equality lowers to the private
  `legacyEqualsFold` operator so old accepted values keep their behavior while
  public `equals` remains case-sensitive;
- `sort_FIELD=asc|desc` lowers, in source argument order, to `SortCriterion`;
- argument-form `skip` and `take` lower to the corresponding page controls;
- duplicates or conflicts between a compatibility alias and a canonical clause
  return `query_modifier`;
- `groupBy` has no legacy argument alias in v1.

Lowered nodes execute in the same compiler and evaluator. There is no second
flat-filter evaluator for equivalent predicates. Existing operations without a
`QueryOperationCapability` continue to use the legacy APIs unchanged only when
the parsed statement contains no canonical clause and no compatibility alias
that lowers into one. Capability absence is not full capability and never
selects a fetch-all/post-filter fallback.

The parser operates once. Local execution uses the parsed model directly.
Confirmation guards, dry-run rewriting, and transports inspect the model rather
than splitting raw text. `RenderQueryModel` is used only when a textual transport
is unavoidable and must preserve expression grouping and typed literal kinds.

Existing public APIs and legacy helper behavior remain source- and
behavior-compatible. The new module API is therefore additive and targets
`agentquery/v1.7.0` after both implementation slices and independent review.
Release work must re-check the authoritative remote tag inventory immediately
before publication; an occupied name advances to the next unused minor and is
never retagged. Consumers that opt into the new canonical pipeline must treat
the new bounds, deterministic identity tie-breaker, and typed refusals as an
explicit migration. A prerelease such as `agentquery/v1.7.0-rc.1` may be used
for integration, never as a substitute for the reviewed stable tag.

## 10. Authorization and stable errors

Authorization precedes field resolution and data loading. Every branch,
projection, sort criterion, and group criterion is checked even when runtime
evaluation would not visit it. Unknown and unauthorized fields return the same
code and fixed message and expose neither the name nor the field catalog.

Text errors carry a one-based statement index and a safe byte position with
zero-based offset and one-based line/column. Externally constructed ASTs carry
`position: null`. Values, regex patterns, accessor errors, paths, and hidden
catalogs are never echoed.

The typed envelope is:

```json
{
  "error": {
    "code": "predicate_field",
    "message": "field is not available",
    "statement": 1,
    "position": {"offset": 18, "line": 1, "column": 19}
  }
}
```

Stable codes are:

| Code | Meaning |
| --- | --- |
| `predicate_syntax` | malformed text or external AST, including empty composites/cycles |
| `predicate_unsupported` | operation/provider has no compatible query-model capability |
| `predicate_field` | unknown or unauthorized field |
| `predicate_operator` | operator is not valid for the registered field |
| `predicate_type` | literal kind/value is invalid for the field |
| `predicate_regex` | Go/RE2 syntax or inline flags are invalid/unsupported |
| `query_modifier` | duplicate/conflicting/invalid sort, group, skip, or take control |
| `predicate_limit` | source, grammar, control, snapshot, value, or cardinality ceiling |
| `predicate_cost` | request work budget exhausted |
| `predicate_data` | snapshot/accessor/stored-value/identity failure |
| `result_limit` | top-level record or serialized-response bound exceeded |
| `query_cancelled` | context cancellation or deadline at a typed checkpoint |

Existing no-model parse and execution errors keep their current exported codes
and envelopes.

### 10.1 Request-wide response budget and error reserve

The response contract carries forward the compatible revision-3 accounting
rules and makes them normative for this repository. Let `M` be the sealed
`MaxResponseBytes`, `R` the sealed `ErrorFramingReserveBytes`, and `S = M - R`.
`S` is the success allowance. Successful statement records, their delimiters,
and the enclosing response framing may consume at most `S`; success never
borrows the reserve. Exactly one terminal redacted typed error record, including
the delimiter and framing delta needed to append it after an accepted success
prefix, may consume at most `R`. The complete response is therefore at most
`M`. All arithmetic is unsigned-overflow checked before addition.

One `QueryResponseRequest` owns one response ledger. It is created only by
`BeginQueryModelResponse`, initialized from the same sealed `QueryLimits`
snapshot and digest used by compilation and execution, and shared by every
statement and dispatch on that opaque request. It records the selected mode,
committed success-prefix bytes, current statement index, initial-dispatch state,
last returned state, terminal error state, unpublished cumulative response, and
publication state. The request object is the production continuation carrier:
the public request methods dispatch initial, retry, renewal, resume, and recovery
attempts through the same internal renderer, buffer, and ledger. A continuation
on another request object, a continuation label before initial dispatch or
without the immediately preceding production dispatch having returned
`QueryResponsePaused`, a second initial label, a different model/access policy,
a replacement schema, a mode switch, or any dispatch after publication refuses
before serialization. `QueryResponsePaused` is produced only when the public
method has committed exactly one complete statement and the preflighted model
contains another statement; `QueryResponseTerminal` is produced after the last
success or any refusal. No caller-supplied context value, digest, byte count,
attempt label, sink, previously returned bytes, or test seam can create or
replace the ledger, buffer, or last returned state.

The one-shot Schema methods begin a request, perform its initial dispatch, and
drive the same private next-statement step until terminal before publishing.
Calling a one-shot method again is explicitly a genuinely new request; it is not
a continuation and carries no claim about prior delivery. Retry, renewal,
resume, recovery, re-rendering, and later batch statements on one request object
reuse charged usage and may not rebind defaults or schema registrations. The
response-accounting test first charges the ledger through the public initial
dispatch of a real two-statement model, requires the observed return value to be
`QueryResponsePaused`, exercises each continuation through the public request
method, requires the continuation to return `QueryResponseTerminal`, and then
consumes the matching publication exactly once. Directly pre-seeding an internal
byte count, setting a private paused bit, bypassing the public return state, or
inspecting the private buffer is not evidence.

Every statement is serialized into a scratch writer capped at the remaining
success allowance. The writer includes the bytes needed for the complete valid
response if that statement were the last success. `result_limit` latches at the
first attempted byte for which the complete candidate success response would
exceed `S`, whether that byte belongs to the result body, a statement delimiter,
or closing framing. The entire scratch statement is then discarded. No byte of
that statement becomes observable, no later statement runs, and the terminal
`result_limit` record names that statement. A successful statement becomes part
of the unpublished cumulative response only after its complete serialization
fits and is committed atomically. Errors raised before or during serialization
use the same terminal path and likewise expose no partial statement.

JSON publication is exactly one minified top-level array. Each committed
statement contributes one complete result element; a terminal typed error
envelope, when present, is the final element, and the closing bracket is written
only by publication. Compact publication is one sequence of complete statement
records separated by one blank line, followed when needed by the terminal
`@error` record. Native publication returns the corresponding ordered statement
record slice and uses the exact canonical JSON array above for accounting. Thus
an accepted prefix plus a later refusal is one valid response in every mode,
never a closed response followed by a suffix.

`Schema.QueryModelJSONASTWithMode` counts the exact published JSON or compact
bytes, including batch delimiters and top-level framing. `Schema.QueryModelAST`
uses the exact canonical JSON encoding of the native value as its accounting
representation before its one publication; its count must equal the JSON-mode
count for the same model. The native entry is not an unmetered bypass. A request
cannot switch modes after its first statement, and JSON and compact may differ
only because their actual encoded byte sequences differ, never because they use
different limits, reserve rules, statement boundaries, latching points, or
publication ownership.

The error encoder accepts only a registry row below, statement index, and safe
position. Values, patterns, paths, provider diagnostics, partial results, and
hidden catalog data cannot enter it. The message list is exhaustive; an
unregistered code/message pair is an internal publication refusal rather than a
dynamic-message fallback.

| Code | Permitted fixed redacted message |
| --- | --- |
| `predicate_syntax` | `query syntax is invalid` |
| `predicate_unsupported` | `query model is not supported` |
| `predicate_field` | `field is not available` |
| `predicate_operator` | `operator is not available for field` |
| `predicate_type` | `query value has invalid type` |
| `predicate_regex` | `regular expression is invalid` |
| `query_modifier` | `query modifier is invalid` |
| `predicate_limit` | `query limit exceeded` |
| `predicate_cost` | `query work limit exceeded` |
| `predicate_data` | `query data is invalid` |
| `result_limit` | `query result limit exceeded` |
| `query_cancelled` | `query was cancelled` |

Setup-only errors such as `secret-bearing field cannot be groupable`, `query
limit exceeds default`, and `query limit is below supported minimum` are returned
by configuration calls before a response request exists. They are not permitted
terminal response messages and cannot consume this reserve.

The maximum statement index is 16. The maximum safe position is offset 65,536
and one-based line/column 65,537; `position: null` and absent request-wide
statement positions are smaller. JSON uses the minified typed envelope shown
above. Compact uses exactly
`@error,<code>,<JSON-string-message>,<statement-or-empty>,<offset-or-empty>,<line-or-empty>,<column-or-empty>\n`.
The message token uses the same canonical JSON string encoder as grouped compact
keys. Relative to a committed success prefix, either mode adds at most two bytes
of separator/closing framing. Exhaustively encoding every permitted registry
row at the maximum numeric widths gives a largest JSON envelope of 158 bytes
(`predicate_operator`) and compact record of 85 bytes, hence maximum real
incremental sizes of 160 and 87 bytes.
The machine resource reproduces these sizes from the independent registry rather
than trusting implementation constants.

The bounded writer measures those exact incremental bytes relative to the
already committed success prefix. The record must fit both `R` and the remaining
whole-response capacity before publication. An accounting failure, a second
error, an unregistered row, or an error record that would exceed either bound is
an internal publication refusal with no fallback payload; it must never publish
an over-cap envelope, reinterpret the failure as success, truncate a typed
envelope, or retry with reset usage. Because 160 is below the supported minimum
reserve of 480 bytes, one real permitted terminal error is always emit-capable.
The separate synthetic 480/481 bounded-writer vector still attacks the
publication gate itself. A narrowing mutant that lengthens one permitted real
message until its encoded incremental size is 481 must fail the registry-derived
production-entry test.

Earlier successful batch records may remain visible when a later statement
fails, but only as the already committed atomic prefix followed by the one
terminal typed error record. The prefix remains charged. The failing statement
is absent in full, `committedSuccessBytes + incrementalErrorBytes <= M`, and the
entire sequence becomes visible only through the request's single publication.

| Revision-3 clause | Repository decision |
| --- | --- |
| success allowance is response cap minus error/framing reserve | carried forward exactly as `S = M - R` |
| response accounting is request-wide and latching | carried forward and bound to the sealed limit digest |
| one bounded redacted typed error remains emit-capable | carried forward; exact incremental wire bytes consume `R` |
| statement serialization is atomic | carried forward for native, JSON, and compact production entries |
| a later batch refusal retains charged earlier successes | carried forward; no partial failing statement |
| implementation-specific buffering or a per-renderer counter | superseded by one unpublished mode-bound request buffer, one ledger, one publication, and capped scratch writer |
| revision-3 reserve sized for as many as 16 typed errors | superseded by exactly one terminal typed error per request; the exhaustive real envelope maximum is 160 bytes |

The refusal inventory is exhaustive for this surface:

| Adversary / entry point | Required refusal |
| --- | --- |
| renderer ignores `R` | `S+1` returns `result_limit`; success cannot borrow the reserve |
| renderer subtracts `R` twice | exact `S` succeeds; the reserve is withheld once, not twice |
| native, JSON, or compact reads a different limit or latches at a different byte | cross-entry/vector mismatch; no output under the divergent rule |
| serializer appends bytes before the statement fits | `result_limit`; committed prefix unchanged and failing statement absent |
| accounting or bounded-error encoding fails | publication refusal; no unmetered fallback or truncated envelope |
| retry, renewal, resume, or recovery resets usage, changes mode/inputs/schema, accepts caller-minted ledger/state, or runs without an observed production pause | the opaque request method refuses or retains the charged ledger; only a preceding public `QueryResponsePaused` return admits the next step |
| continuation closes or returns a response independently, or runs after publication | refusal; dispatch exposes zero response bytes and exactly one matching publish call owns the complete response |
| error record exceeds `R` or makes the response exceed `M` | publication refusal; over-cap bytes are never emitted |

## 11. Normative limits

Hosts may lower, but never raise, these defaults and must expose effective values
through schema introspection. `DefaultQueryLimits()` returns the table exactly.
Normalization is field-wise: an all-zero `QueryLimits{}` means all defaults;
in a partial value, each zero field inherits its default and each non-zero field
must be less than or equal to its default. Non-zero values have a minimum of 1,
except `MaxResponseBytes`, whose minimum is 8,192, and
`ErrorFramingReserveBytes`, whose minimum is 480 so one redacted typed error can
always be emitted. A value above its default returns `predicate_limit` with the
fixed message `query limit exceeds default`; a value below a supported minimum
returns `predicate_limit` with `query limit is below supported minimum`. The
error does not expose configured values or identify hidden fields. Normalization
is atomic: failure leaves the schema's previous limits unchanged.

| Limit | Default | Boundary rule |
| --- | ---: | --- |
| source bytes | 65,536 | valid UTF-8; 65,537 refuses |
| statements per request | 16 | 17 refuses before execution |
| expression depth | 16 | root is depth 1; 17 refuses |
| expression nodes per statement | 128 | every composite and leaf counts |
| decoded literal bytes | 4,096 each | before normalization/deduplication |
| decoded regex bytes | 1,024 cumulative per statement | also literal bytes |
| finite-set entries | 100 per set | duplicates count |
| explicit sort criteria | 4 | automatic identity excluded |
| snapshot rows | 100,000 | bounded provider, not post-load only |
| snapshot typed bytes | 67,108,864 | 64 MiB |
| stored scalar bytes | 65,536 each | invalid UTF-8 is data failure |
| stored array entries | 1,024 each | 1,025 refuses |
| group cardinality | 10,000 | 10,001 refuses before pagination |
| skip | 100,000 | inclusive maximum |
| take | 1,000 | explicit minimum 1 |
| top-level results without explicit take | 1,000 | refusal, never truncation |
| request work units | 10,000,000 | shared across statements |
| serialized response bytes | 4,194,304 | whole response; success allowance is this value minus the reserve |
| error/framing reserve | 8,192 | one terminal typed error and incremental framing inside the response cap |

After field-wise normalization,
`ErrorFramingReserveBytes <= MaxResponseBytes` is a derived invariant rather
than an independently reachable refusal: the reserve range is `480..8192` and
the response range is `8192..4194304`, so every individually valid normalized
configuration satisfies the relation. Implementations must not carry a dead
cross-field rejection branch or claim it as executed coverage. The effective
digest is SHA-256 over the ASCII contract name
`agentquery.composable-query.v1`, one NUL byte, then all 19 normalized `uint64`
fields in `QueryLimits` declaration order as unsigned big-endian bytes. This
digest binds parser output, compiler output, bounded loader inputs, evaluation
counters, grouped/ungrouped result limits, rendering, and discovery to one
configuration. It is never accepted from the caller as proof.

`SetQuerySnapshotLoader` receives `SnapshotLimits` copied from the sealed
`MaxSnapshotRows` and `MaxSnapshotTypedBytes`; the provider must refuse before
allocating beyond either value. All other execution-only ceilings are read
from the same sealed configuration, never from package defaults. Retrying,
renewing, resuming, recovering, re-rendering, or executing another batch
statement in the same request does not reset or replace the digest, response
ledger, or other request-wide counters. Section 10.1 defines the response
ledger and exact `result_limit` latch.

The effective-limit regression publishes a `10/10` executed matrix: zero
defaults, partial-zero lowering, above-default atomic refusal, below-minimum
atomic refusal, post-seal atomic refusal, parsed-model digest mismatch, bounded
loader `N`/`N+1`, and sealed-limit witnesses for sorting, grouping, and
pagination. The sorting witness uses two reverse-score rows with one-unit `id`
and `score` accessors: four units reach the first scalar comparison and the
fifth attempted unit refuses in sorting. The grouping witness lowers cardinality
to one and refuses a second distinct status. The pagination witness lowers the
top-level no-`take` result bound to one, refuses two rows, and admits `take 1`.
Each witness runs through `Schema.QueryModelAST`; a mutant that reads a package
default in that stage must turn the expected refusal into an unexpected
success.

Limits are checked at `N` and `N+1`; tests must also narrow each configurable
gate to `N-1` and prove the `N` fixture then refuses. Deleting a gate is not
sufficient evidence. Work charging retains the revision-3 rules, with the regex
addition from section 7. Counters are request-wide, overflow-safe, and never
reset by retrying a later batch statement.

The canonical path requires a bounded snapshot provider that accepts the row
and byte ceilings before allocation. A provider that cannot make that guarantee
cannot register the capability. A post-load `len(items)` check is not a memory
bound.

## 12. Schema discovery

`Schema.QueryModelSchema(ctx, access)` is the access-bearing production entry
for query-model discovery. It returns the value of the `queryModel` metadata
object. An authenticated transport may place that object under the
`queryModel` key of its schema response; the legacy built-in `schema()` handler
is unchanged and must not synthesize query-model metadata because it receives
no `QueryAccess`. The returned object has exactly these top-level keys:

- `version`, `grammarRevision`, `operations`, `composites`, `predicates`,
  `fields`, `limits`, `modifiers`, `resultShapes`, `errorCodes`, and `examples`;
- `operations` is sorted by operation name and carries the canonical ordered
  clause list from each sealed `QueryOperationCapability`;
- `fields` includes only fields for which `access.AllowsVisibility` is true;
  every sortable/groupable/identity list is derived from that same filtered
  list, never from the unfiltered catalog.

The remaining metadata includes:

- `version: 1` and `grammarRevision: agentquery.composable-query.v1`;
- composite and leaf names from the grammar;
- effective field kinds, allowed operators, visibility-filtered sortable and
  groupable fields, and the identity field;
- effective limits;
- canonical modifier syntax and the compatibility aliases;
- grouped/ungrouped JSON and compact result descriptors, including the exact
  eight-row typed group-key encoding table from section 8;
- the stable error-code registry;
- the three positive examples and refusal examples above.

Capability and schema reads fail closed. A nil `QueryAccess`, no registered
query-operation capability, or malformed catalog is an error and returns no
partial metadata. A missing operation capability is
`predicate_unsupported`; unknown and unauthorized fields remain
`predicate_field` with the fixed non-enumerating message. Failed or malformed
reads are never absence and never permission to fetch all rows for local
post-filtering. Normal fixture access publishes the 11 public fields;
restricted fixture access publishes all 12 fields, while the secret field still
cannot be groupable.

## 13. Production slices

The revision-5 loop response split architecture rework by catalog surface:
`TASK-260923-37ss7m` owns operation capability and authorized discovery,
`TASK-260923-2vs2fb` owns canonical typed group-key encoding, and
`TASK-260923-naqctx` owns response/error-reserve accounting. They are serialized
before the two runtime slices. No research or generic harness task is justified.

### Slice 1 — `TASK-260923-3hhfe2`

Implement the model, functional parser/renderer, typed field registration,
full-query compilation, three-valued evaluator, bounded regex, compatibility
filter lowering, introspection, and a real `Schema.QueryModelAST` list path.
The smallest runnable vertical slice is a nested `satisfiesAll` query containing
`satisfiesAny`, `not`, and `matchesRegex`, executed through the public Schema
entry point over a bounded fixture snapshot. It must refuse an invalid regex,
an empty composite, a hidden field in an unreachable branch, and a cyclic
external AST before loading rows. It also owns limit normalization and sealing:
the production entry must exercise a lowered snapshot-row limit at `N` and
`N+1`, reject above-default and below-minimum configurations atomically, reject
a post-seal update and parsed-model digest mismatch, and publish its rows of the
`10/10` matrix described in section 14.

This slice parses and renders the full frozen statement grammar because its
public AST and renderer are the compatibility boundary consumed by slice 2.
Execution of sort/group/page may return `predicate_unsupported` until slice 2;
predicate behavior cannot be deferred to a helper-only test.

### Slice 2 — `TASK-260923-s86r49`

Implement row sorting, identity tie-breaking, grouping, top-level pagination,
projection, JSON/compact grouped rendering, and modifier introspection through
the same `Schema.QueryModelAST` entry. It consumes the accepted slice-1 model
without introducing another AST or evaluator.

The runnable vertical slice is the grouped owner example: filter missing
assignees, sort rows by updated plus identity, group by status, page groups,
project items, and render both modes. It must refuse duplicate sort fields,
array/hidden grouping, group cardinality `N+1`, and paging bounds. Field
registration is part of this slice's real schema path and must reject an
authorized secret-bearing field that self-asserts `Groupable: true`, plus zero
and unknown sensitivity values before catalog/discovery insertion. It owns the
sorting, grouping, and pagination rows of the effective-limit matrix. It also
owns the section 10.1 response ledger at the native, JSON, and compact
production entries, including late-batch atomic refusal and bounded typed error
publication; no separate renderer or generic oracle is introduced.

## 14. Frozen-contract implementation gate

Before slice 1 enters development, the vector resource is the
`grammar_frozen` precondition. `TestQueryModelFrozenContractCompleteness`
derives the required grammar productions and Go declarations from its five
normative registries and reports the exact `156/156` coverage ratio; an
implementation-owned declaration table is not an acceptable expected set.

`TestQueryModelFrozenPublicSurfaceNoPackageScopeCollision` intersects every
new package-scope name in the vector resource with the immutable 115-name
legacy package-scope baseline frozen there. That baseline was source-derived
with Go's parser from non-test `agentquery/*.go` at commit `c5a6fb45...` and
package tree `a00d35c...`; it is not recomputed from the post-implementation
package. The exact-declaration half of
`TestQueryModelFrozenContractCompleteness` separately checks the implemented
surface against the type/field/signature/value registries. The required
narrowing mutant replaces `QuerySortDirection` in the candidate set with the
legacy `SortDirection`; the collision test must fail and report that identifier.
Allowlisting implemented candidate names is forbidden.

`TestQueryModelLegacyBatchGrammarCompatibility` drives `ParseQueryModel` with
the machine vectors for `list()`, `get(T1);`, `get(T1);;;get(T2)`, and
`;;get(T1)`. Its required narrowing mutant replaces `semicolons` with one exact
`;` and removes both optional edge occurrences; the trailing, repeated, and
leading compatibility cases must fail under the mutant while the production
grammar accepts all four. This is the owner-leaf regression for revision-2 F2,
not a new research or harness prerequisite.

`TestQueryModelEffectiveLimitBinding` configures a schema with
`MaxSnapshotRows: 2` and every other field zero, obtains the normalized limits
through `Schema.QueryLimits`, parses with that value, and drives
`Schema.QueryModelAST`. A two-row bounded snapshot succeeds, a three-row
provider refuses before allocating or returning rows, an above-default value
and a below-minimum value refuse atomically, a post-seal setter refuses without
mutation, and a model parsed under defaults is rejected for digest mismatch
before load. The same named test executes every row of the machine resource's
`10/10` limit-state matrix. Its production-entry stage witnesses lower
`MaxWorkUnits` for sorting, `MaxGroupCardinality` for grouping, and
`MaxTopLevelResultsWithoutTake` for pagination. It reports all nine bound
stages: parser, compiler, bounded loader, evaluation, sorting, grouping,
pagination, rendering, and discovery. Required mutants narrow the snapshot
limit, substitute package defaults in each of the three downstream stages,
admit a below-minimum response bound, and admit a post-seal update. Every mutant
must fail the named test. A default-only, parser-only, or six-of-nine-stage
implementation cannot satisfy this regression.

`TestQueryModelResponseBudgetAccounting` drives `Schema.QueryModelAST`,
`Schema.QueryModelJSONASTWithMode`, `Schema.BeginQueryModelResponse`, both
public request dispatch methods, and both single-publication methods. It derives
all cases from the machine
resource's `responseBudgetAccounting` and `typedErrorEnvelopeRegistry`
registries and publishes `6/6` surfaces, `13/13` production cases, and `8/8`
refusal rows. Every case lowers both
`MaxResponseBytes` and `ErrorFramingReserveBytes` through `SetQueryLimits` and
parses with `Schema.QueryLimits()`. The success boundary accepts an exact `S`
candidate and refuses `S+1`; native and JSON counts are equal. Compact repeats
its own exact `S`/`S+1` boundary. Whole-response cases accept a terminal error
whose targeted bounded-writer boundary fixture leaves the complete response at
`M` and refuse a one-byte-wider frame before publication. The fixture is
injected below the public entry only to force the boundary; the real production
publication gate makes the decision, so this is not a helper-only oracle. A
two-statement case commits statement one, latches `result_limit` while staging
statement two, preserves the charged prefix, emits one bounded error, and
exposes zero bytes of statement two.
The real-envelope case independently enumerates all 12 permitted code/message
rows, encodes maximum statement/position widths in JSON and compact mode, adds
the maximum framing delta, and reproduces 160/87-byte maxima below the 480-byte
minimum reserve.

The initial continuation control proves the same `S` boundary through the
one-shot wrapper. Each retry, renewal, resume, and recovery case obtains a fresh
opaque request for a real two-statement model. Its public initial dispatch
executes exactly statement one, charges `S`, and must observably return
`QueryResponsePaused`; the test asserts that state before calling the same
request's public continuation method. The continuation executes exactly
statement two, refuses its first attempted success byte under the original
ledger, and returns `QueryResponseTerminal` with `result_limit`. Dispatch
signatures expose state and error but no response value; the matching publish
method is called once after terminal and must yield one parseable cumulative
response, with no duplicated prefix or bytes after closing framing, at or below
`M`. All `5/5` lifecycle paths use the same sealed schema and refuse their next
attempted byte with the original ledger. Tests may not pre-seed committed usage,
set a private pause bit, inspect the private buffer, or call the internal
renderer. A continuation label on a fresh request or without an observed public
`QueryResponsePaused` return, repeated initial, cross-request state, replacement
schema, mode change, model/access replacement, premature or repeated
publication, wrong publication family, or continuation after publication is
refused before bytes become observable.

Required narrowing mutants (a) do not subtract the reserve, (b) subtract it
twice, (c) read `DefaultQueryLimits()` in rendering, (d) maintain a compact-only
counter, (e) stream one byte of the failing statement before latching, (f) give
real retry/renewal/resume/recovery dispatches a fresh ledger, (g) publish an
error frame at `M+1`, (h) lengthen one permitted real fixed message until its
incremental envelope is 481 bytes, (i) preserve the charged ledger but close
and return an independently framed response from a continuation, and (j) set a
private paused bit only through the test seam while production initial dispatch
returns `QueryResponseTerminal`. Each
must fail the named test through a production entry. Helper-only byte counters,
pre-seeded usage, fixture-declared pause state, default-only limits, delete-only
mutants, a surface ratio
below `6/6`, a case ratio below `13/13`, a real-envelope row ratio below
`12/12`, a mode ratio below `2/2`, or a refusal ratio below `8/8` do not satisfy
the gate.

`TestQueryModelSecretGroupRegistrationRefusal` calls the real
`Schema.RegisterQueryField` entry with `Sensitivity:
FieldSensitivitySecret`, `Visibility: "restricted"`, and `Groupable: true`,
while its query access fixture authorizes `restricted`. Registration must
return `query_modifier`, the field must not enter discovery, and no snapshot
load may occur. The same production entry is then called with zero and unknown
`Sensitivity`; each returns `predicate_type` atomically with no catalog or
discovery insertion. The test publishes `3/3` executed registration cases.
Required mutants (a) reject secret-plus-groupable only when `Visibility == ""`
and (b) default zero or unknown sensitivity to public; each must fail the named
test. This attacks both the self-minted groupability bypass and the Go-zero-value
bypass instead of relying on an unauthorized query.

`TestQueryModelFixtureValidity` validates the machine fixture before executing
behavior vectors. All `12/12` registered fields must carry explicit
`visibility` and `sensitivity`. The normal access policy admits `public` and
denies `restricted`; all `32/32` `Schema.QueryModelAST` vectors and the one
`Schema.CompileQueryModel` capability vector resolve to that fixture policy
unless they carry an explicit override. The one
`Schema.QueryModelSchema` vector executes all `3/3` explicit access cases.
Registration-only vectors declare their access context explicitly. Removing
one visibility label must report `11/12`, and removing the fixture binding must
lower the combined `33/33`; either fails before any vector runs, so missing authorization
setup cannot become implicit allow-all evidence.

`TestQueryModelOperationCapabilityAndDiscovery` drives the production
`Schema.RegisterQueryOperationCapability`, `Schema.CompileQueryModel`,
`Schema.QueryModelAST`, and `Schema.QueryModelSchema` entries. It executes all
`5/5` surfaces and `22/22` executable cases in the machine resource's
`operationCapabilityCoverage`. The case inventory is `2/2` `list` cases, `12/12`
`count` cases, `1/1` custom opt-in, `4/4` unregistered initial/retry/resume/
recovery, and `3/3` normal/restricted/nil-access discovery.

The full-list case executes `Schema.QueryModelAST`, performs exactly one bounded
snapshot load, and returns a result whose selected rows and shape independently
witness `where`, `sortOrder`, `groupBy`, `skip`, `take`, and projection. The
count matrix admits canonical `where`; refuses every other canonical clause and
the three owned lowering aliases before field resolution/load; and proves the
unknown/unauthorized-field equivalence after admission without a snapshot load.
Two isolated counterfactual schemas prove that the conventional names carry no
implicit policy: `list` registered with `where` only refuses `sortOrder` with
zero field resolutions and zero loads, while `count` registered with `where`
and `sortOrder` admits `sortOrder` and performs one bounded load. These cases
preserve the conventional fixture cases rather than replacing them.
Every unregistered lifecycle attempt returns the same
`predicate_unsupported` class with zero field resolutions, zero loads, and no
catalog-bearing output. Discovery reports `11/11` normal-visible and `12/12`
restricted-visible fixture fields without making the secret field groupable.

Required narrowing mutants (a) use the exact hybrid name branch that hard-codes
`list` as full and `count` as where-only while consulting registered metadata
only for other operation names; both counterfactual standard-name cases must
fail under it, (b) treat an absent capability as full capability, (c)
derive discovery from the unfiltered field catalog, (d) refuse only sort and
projection while admitting count group/page modifiers, (e) check canonical
clauses but bypass capability admission for lowering aliases, and (f)
manufacture capability only on retry/resume/recovery. Each mutant must fail
this named test. The test obtains results and metadata only through the named
production entries; calling a private formatter, compiler helper, or legacy
`introspect()` helper is not evidence.

`TestQueryModelTypedGroupKeyEncoding` drives
`Schema.QueryModelJSONASTWithMode` with isolated registered fields and bounded
snapshots for string, identifier, enum, int64, Boolean, timestamp, null, and
missing group keys. The eight required variants plus one escape-sensitive
string case each run once in JSON and once in compact mode. The machine
inventory derives nine cases times two declared modes, publishing an exact
`18/18` executed/required ratio and `1/1` lexical-escape ratio. The test compares
native JSON value types, raw JSON token bytes before decoding, and exact compact
`@group` header bytes after one CSV framing pass. The escape-sensitive case
covers quote, backslash, short and generic C0 escapes, ordinary non-ASCII UTF-8,
supplementary-plane UTF-8, U+2028/U+2029, HTML-sensitive characters, and an
unescaped solidus. The timestamp
case supplies two equal instants with different offsets and requires one group,
count two, and the same UTC RFC3339Nano value in both modes. The discovery case
also reproduces all `8/8` encoding-table rows and the lexical string policy
`1/1` through `Schema.QueryModelSchema`.

Required narrowing mutants (a) stringify every present JSON key, which must
fail the int64 and Boolean cases, (b) choose the first timestamp member as the
representative, which must fail the equal-instant/different-offset case, and
(c) let compact rendering independently format a typed value instead of using
the shared canonical JSON token, which must fail the cross-mode exact-header
cases, (d) use default HTML escaping in JSON mode but `SetEscapeHTML(false)` in
compact mode, (e) escape `/` as `\/` only in compact mode, and (f) encode
U+1F600 as `\ud83d\ude00` only in compact mode. The raw-token and
once-framed-header assertions must fail every mode-split mutant even though the
mutated tokens decode to the same logical string. A helper-only formatter test,
parsed-value-only comparison, a mode ratio below `18/18`, a lexical ratio below
`1/1`, or discovery that omits any table row or the lexical policy cannot
satisfy this regression.

No product or architecture choice remains unresolved in this revision, so no
`UNRESOLVED_QUESTIONS.md` entry is required. Implementation discoveries that
would alter grammar, exported identifiers, limits, errors, or result shapes
must return this task to architecture review rather than silently widening the
frozen contract.

`TASK-260908-3hhvl2` remains the serialized integration, documentation,
cross-entry validation, independent review, and immutable release task.

## 15. Traceability and open questions

| Owner requirement | Frozen section |
| --- | --- |
| nested `not` / `satisfiesAll` / `satisfiesAny` | 3, 5, 6 |
| typed leaves and presence | 4, 5, 6 |
| bounded regex | 7, 11 |
| deterministic pipeline | 2, 8 |
| `groupBy` shape and ordering | 8 |
| bounded `skip` / `take` | 8, 11 |
| compatibility and one evaluator | 3, 9 |
| authorization and refusals | 10 |
| operation capability and authorized schema discovery | 3, 8, 9, 12, 14 |
| request-wide response and error-reserve accounting | 10.1, 11, 14 |
| next two production slices | 13 |

No product or architecture question remains open for these two slices, so this
task does not create `UNRESOLVED_QUESTIONS.md`. A later consumer may register a
different logical field catalog, but that is configuration under this contract,
not an unresolved grammar decision.
