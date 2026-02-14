# EPIC-260213-3fcjlo: predicate-filtering

## Description
First-class predicate/filtering support in agentquery. Currently every consumer (board-cli, example) manually implements filter logic in operation handlers — parsing args, building predicates, applying FilterItems. Move filtering into the library: register filterable fields on Schema, auto-extract filter params from args, apply predicates before the operation handler sees data. Operation handlers get pre-filtered items instead of doing it themselves.

## Scope
(define epic scope)

## Acceptance Criteria
(define acceptance criteria)
