# TASK-260218-2nj2eu: schema-introspection-mutations

## Description
Phase 2: Extend schema() introspection to include mutations.

EXTEND: agentquery/schema.go — introspect() method
- Add 'mutations' key: sorted list of mutation names (only if any mutations registered)
- Add 'mutationMetadata' key: map of mutation name to MutationMetadata (only if any have metadata)
- Follow existing pattern for operationMetadata

TESTS: extend existing schema tests or add new ones in mutation_test.go
- Register mutations with metadata, call schema(), verify JSON includes mutations and mutationMetadata sections
- Verify mutations without metadata still appear in mutations list
- Verify schema output is unchanged when no mutations registered (backward compat)

Design: .research/260218_mutation_design_proposal.md section 3

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
