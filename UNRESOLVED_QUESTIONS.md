# Unresolved Questions

## Composable Query Expressions

The architecture task `TASK-260923-37ss7m` must resolve these before either
implementation task starts:

1. What exact textual DSL maps to the product concepts `not`, `satisfiesAll`,
   `satisfiesAny`, and the leaf predicates while preserving legacy queries?
2. What exact exported Go types and constructor names represent the public
   expression tree without breaking unkeyed literals of existing AST types?
3. Which stable typed error codes and numeric resource defaults extend the
   already researched predicate limits for regex compilation and group
   cardinality?
4. What JSON and compact transport encoding represents grouped results while
   preserving field projection and schema discovery?
