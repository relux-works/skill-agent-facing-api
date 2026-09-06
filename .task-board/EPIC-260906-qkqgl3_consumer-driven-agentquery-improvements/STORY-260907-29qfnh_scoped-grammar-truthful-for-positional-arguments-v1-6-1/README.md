# STORY-260907-29qfnh: scoped-grammar-truthful-for-positional-arguments-v1-6-1

## Description
Patch release after agentquery/v1.6.0. Consumer review (skill-project-management BUG-260903-12avv2 review-verdict-rev2, finding F1) measured that GrammarSyntax() says arguments are comma-separated key=value only while the parser accepts bare positional values (parseArg: ident = value | value) and every operation example uses one (get(TASK-XX) { overview }); and that the grammar completeness gate is defeatable: removing op(positional) from grammarExamples and the positional clause from Grammar.Arguments leaves go test ./... green. Deliverable: v1.6.1 tag on main.

## Scope
(define story scope)

## Acceptance Criteria
(define acceptance criteria)
