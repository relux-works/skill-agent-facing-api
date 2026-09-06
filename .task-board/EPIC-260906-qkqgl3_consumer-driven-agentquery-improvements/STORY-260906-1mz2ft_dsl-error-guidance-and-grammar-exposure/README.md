# STORY-260906-1mz2ft: dsl-error-guidance-and-grammar-exposure

## Description
Expose the DSL grammar through the schema surface and make parse errors for an unknown operation carry the known-operation list and a schema() pointer, so an agent recovers from a DSL mistake without reading skill docs outside the API. Consumer driver: skill-project-management BUG-260903-12avv2 (task-board half already written against this API) and BUG-260906-2itnl6 (follow-on: unknown-field did-you-mean, bare element-ID hint). Deliverable: reviewed change on main plus a tagged release agentquery/v1.6.0 (additive API).

## Scope
(define story scope)

## Acceptance Criteria
(define acceptance criteria)
