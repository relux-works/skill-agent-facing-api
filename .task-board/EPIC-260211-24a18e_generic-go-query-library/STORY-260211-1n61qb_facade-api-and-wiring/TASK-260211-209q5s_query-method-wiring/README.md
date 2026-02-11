# TASK-260211-209q5s: query-method-wiring

## Description
Implement Schema[T].Query(input string) (any, error). Wire: parse input via parser -> for each statement: find handler, build FieldSelector from projection, call loader for Items, invoke handler with OperationContext -> return result. QueryJSON returns []byte.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
