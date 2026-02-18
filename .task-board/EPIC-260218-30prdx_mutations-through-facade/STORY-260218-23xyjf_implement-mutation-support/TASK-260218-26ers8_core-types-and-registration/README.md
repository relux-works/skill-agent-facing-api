# TASK-260218-26ers8: core-types-and-registration

## Description
Phase 1: Core mutation types and Schema registration.

NEW FILE: agentquery/mutation.go
- Mutation() method on Schema — registers handler, wraps into operations via wrapMutation()
- MutationWithMetadata() — registers handler + metadata
- wrapMutation() — adapts MutationHandler to OperationHandler, builds ArgMap, handles dry_run flag, runs framework validation, wraps result in MutationResult
- validateArgs() — checks required params and enum constraints from metadata
- HasMutations() — returns true if any mutations registered

EXTEND: agentquery/types.go
- MutationHandler[T] func type
- MutationContext[T] struct (Mutation, Statement, Args, ArgMap, Items, DryRun)
- MutationResult struct (Ok, Result, Errors)
- MutationError struct (Field, Message, Code)
- MutationMetadata struct (Description, Parameters, Examples, Destructive, Idempotent)
- New error codes: ErrConflict, ErrForbidden, ErrPrecondition, ErrRequired, ErrInvalidValue
- Extend ParameterDef with Enum []string and Required bool

EXTEND: agentquery/schema.go
- Add mutations map[string]MutationHandler[T] and mutationMetadata map[string]MutationMetadata to Schema struct
- Initialize in NewSchema()

TESTS: agentquery/mutation_test.go
- Register mutations, call Query(), verify MutationResult shape (ok/result/errors)
- Test framework validation (required params, enum)
- Test dry_run flag
- Test batch with mutations
- Test mixed batch (query + mutation)
- Test mutation error isolation in batch

Design: .research/260218_mutation_design_proposal.md sections 1, 2, 4

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
