# STORY-260213-e59yqs: schema-introspection-pagination

## Description
Add OperationMetadata to schema registration and expose it in schema() introspection. Agent calls schema() once per session and learns all available operations, their parameters (name, type, optional, default, description), and usage examples. No external docs needed. Implementation: new OperationMetadata/ParameterDef types, new OperationWithMetadata() registration method, updated introspect() output. Backwards compatible — existing Operation() works unchanged. See .research/260213_pagination-discoverability.md

## Scope
(define story scope)

## Acceptance Criteria
(define acceptance criteria)
