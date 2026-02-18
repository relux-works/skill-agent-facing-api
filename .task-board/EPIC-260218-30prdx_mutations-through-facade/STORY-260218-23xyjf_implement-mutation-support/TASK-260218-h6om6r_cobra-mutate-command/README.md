# TASK-260218-h6om6r: cobra-mutate-command

## Description
Phase 3: Cobra integration — MutateCommand.

EXTEND: agentquery/cobraext/command.go
- MutateCommand[T]() factory — 'm' subcommand with --format (required), --dry-run, --confirm flags
- needsConfirm() helper — parses mutation name from input, checks MutationMetadata.Destructive
- injectDryRun() helper — appends dry_run=true to the query string args
- Update AddCommands() — conditionally add 'm' command only if schema.HasMutations()

TESTS: agentquery/cobraext/command_test.go
- Execute mutation via m command, verify output
- Test --dry-run flag injection
- Test --confirm required for destructive mutations
- Test AddCommands with and without mutations (backward compat)

Design: .research/260218_mutation_design_proposal.md section 5

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
