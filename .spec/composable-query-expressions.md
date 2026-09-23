# Composable Query Expressions

Status: product baseline awaiting architecture acceptance
Story: `STORY-260908-3ieod5`

## Problem

Flat `key=value` filters cannot express nested Boolean conditions, regex
matching, or reusable typed predicate trees. Sorting and `skip`/`take` exist as
separate helpers, but there is no single query model that defines how filtering,
sorting, grouping, and pagination compose.

The feature must provide one typed query-expression model rather than adding a
new operation for every filtering combination.

## Required Outcome

A caller can construct and execute nested logical predicates, then apply a
deterministic result pipeline:

1. authorize and compile the complete query;
2. load one consistent input snapshot;
3. filter with the compiled predicate tree;
4. apply `sortOrder` with a stable identity tie-breaker;
5. optionally apply `groupBy`;
6. apply bounded `skip` and `take` to the top-level result;
7. project and render the result.

When `groupBy` is absent, the top-level result contains rows. When `groupBy` is
present, the top-level result contains groups and pagination selects groups.
Items inside each group retain the deterministic row order from step 4.

## Predicate Model

### Composite nodes

The public expression model must expose these concepts directly:

- `not(expression)` negates one child;
- `satisfiesAll(expression...)` is logical AND;
- `satisfiesAny(expression...)` is logical OR.

Composite nodes may contain leaf predicates or other composite nodes to the
configured depth and node limits. Empty `satisfiesAll` and `satisfiesAny`
expressions are invalid rather than implicit constants.

The exact Go identifiers and textual DSL spelling are architecture decisions,
but the public API and schema introspection must preserve these three concepts.

Example intent:

```text
satisfiesAll(
  equals(type, "task"),
  satisfiesAny(
    contains(name, "predicate"),
    matchesRegex(description, "(?i)group(?:ing|by)")
  ),
  not(equals(status, "done"))
)
```

### Leaf predicates

The v1 model must cover:

- equality and inequality;
- string or identifier containment;
- bounded regex matching;
- ordered comparison for ordered scalar types;
- half-open ranges, equivalent to `value >= lower AND value < upper`;
- finite-set membership;
- array element membership;
- explicit null and missing presence tests.

Leaf predicates operate only on registered typed fields. A consumer may
register a logical field whose accessor reads nested domain data, but the query
language does not perform unrestricted object traversal or reflection.

### Typed semantics

Registered fields declare their kind, allowed operators, presence behavior,
accessor cost, visibility, and sort/group eligibility. At minimum, the shared
model supports string/identifier, enum, signed integer, Boolean, timestamp, and
homogeneous scalar-array kinds.

Null and missing are distinct from a present value and from each other. Ordinary
comparisons on null or missing yield an unknown truth value. Boolean composition
uses three-valued logic: false dominates AND, true dominates OR, and negating
unknown remains unknown. Only true selects a row.

String equality, containment, and ordering are case-sensitive and do not apply
Unicode normalization unless a registered consumer field explicitly defines a
different typed accessor contract. Timestamp comparisons use absolute instants
with an explicit offset; half-open time windows are the standard range form.

### Regex matching

`matchesRegex` performs substring search unless the pattern supplies anchors.
The implementation must use Go's RE2-compatible `regexp` engine or an
equivalent linear-time engine; backtracking engines are out of scope.

Regex patterns are compiled once during query compilation. Invalid syntax,
unsupported constructs, excessive pattern bytes, excessive expression depth,
or an operation/field that does not allow regex must fail closed with stable
typed errors before loading rows. Inline RE2 flags are allowed within the same
bounds. Regex matching is case-sensitive unless the pattern opts into an
allowed inline flag.

## Query Modifiers

### `sortOrder`

`sortOrder` is an ordered list of registered scalar criteria and directions.
Multiple keys are applied in caller order. Duplicate keys, unsupported fields,
and invalid directions are typed errors.

The engine appends a unique identity key in ascending order unless the caller
already selected it. This makes pagination deterministic for a fixed snapshot.
Arrays are not sortable in v1.

Existing `sort_FIELD=asc|desc` arguments may remain compatibility sugar, but
they must lower into the same query model.

### `groupBy`

V1 accepts zero or one registered grouping criterion. A grouped result contains
deterministically ordered group records with this logical shape:

```json
{
  "key": {"presence": "present", "value": "analysis"},
  "count": 3,
  "items": []
}
```

Present keys use the field's typed equality and ascending order. Explicit null
and missing form separate groups after present keys, in that order. Arrays and
secret-bearing fields are not groupable in v1. Grouping must not reveal an
unauthorized field through group keys, counts, ordering, or errors.

Projection applies to group items. The group key is always represented even if
the grouped field is absent from the item projection. Compact and JSON output
must describe the grouped shape through schema metadata rather than relying on
undocumented transport behavior.

### `skip` and `take`

`skip` defaults to zero. Omitted `take` means no caller-specified page size, but
the configured result bound still applies. Explicit `take` must be positive.
The accepted initial hard bounds are `skip <= 100000` and `take <= 1000` unless
the architecture records a compatible reason to change them.

Pagination applies to the final top-level collection: rows for an ungrouped
query, groups for a grouped query. It never changes the item membership inside
a selected group.

## Compatibility

- Existing no-expression queries keep their current behavior.
- Compatible flat filters lower to the shared predicate representation; there
  must not be two independent evaluators for equivalent predicates.
- Existing sorting and pagination APIs may remain as aliases, but schema
  introspection identifies their canonical query-model fields.
- Parsing occurs once. Transports, confirmation guards, and dry-run rewriting
  operate on the parsed AST and never split raw query text.
- The initial implementation belongs in the shared `agentquery` module.
  Consumer-specific task-board adapters are a separate downstream concern.

## Safety and Refusals

The compiler validates the entire expression, including branches that runtime
short-circuiting would not evaluate. Unknown and unauthorized fields are
indistinguishable to callers.

The implementation must enforce bounded source bytes, statement count,
expression depth, node count, literal bytes, regex bytes, set entries, snapshot
rows, stored value bytes, evaluation work, response bytes, sort keys, grouping
cardinality, `skip`, and `take`. Exact defaults and stable error codes are frozen
by the architecture before implementation.

Errors must identify the failing statement and safe source position when the
request came from text, without echoing secret values or hidden field catalogs.

## Public Discovery

Schema introspection must expose:

- supported composite and leaf predicate kinds;
- field kinds and their permitted operators;
- sortable and groupable fields;
- modifier syntax and bounds;
- grouped and ungrouped result shapes;
- stable typed error codes;
- positive nested examples and refusal examples.

## Acceptance Examples

The accepted architecture must provide executable equivalents of these cases:

```text
# Nested Boolean composition with regex
list where satisfiesAll(
  equals(type, "task"),
  satisfiesAny(contains(name, "query"), matchesRegex(description, "(?i)predicate")),
  not(equals(status, "done"))
)

# Half-open timestamp window, sorted and paginated
list where satisfiesAll(
  greaterThanOrEqual(created, timestamp("2026-09-01T00:00:00+03:00")),
  lessThan(created, timestamp("2026-10-01T00:00:00+03:00"))
) sortOrder(updated descending, id ascending) skip 20 take 10

# Grouped query; skip/take select groups
list where not(isMissing(assignee))
  sortOrder(updated descending)
  groupBy(status)
  skip 0 take 5
```

The spelling above is product-level pseudocode. The architecture freezes one
unambiguous textual grammar plus the corresponding Go API and renderer.

## Superseded Restrictions

The earlier external revision-3 research contract remains useful evidence for
typed values, presence semantics, authorization, resource bounds, sorting, and
pagination. It is not the product authority for this Story. Its explicit
exclusion of regex and omission of `groupBy` are superseded by the refreshed
owner requirements recorded on 2026-09-23.

## Delivery Slices

1. `TASK-260923-37ss7m` freezes the architecture and revises this spec.
2. `TASK-260923-3hhfe2` implements the shared predicate AST and evaluator.
3. `TASK-260923-s86r49` implements sorting, grouping, and pagination composition.
4. `TASK-260908-3hhvl2` integrates, documents, validates, reviews, and releases.
