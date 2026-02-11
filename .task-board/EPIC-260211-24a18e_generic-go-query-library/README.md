# EPIC-260211-24a18e: generic-go-query-library

## Description
Go library (importable package, not CLI-embedded) implementing the agent-facing-api pattern generically. User defines their domain model, registers fields/presets/operations, and gets a working DSL parser + field projection + scoped grep + JSON output. Like a mini-GraphQL/Apollo but for CLI tools — zero HTTP, zero schema files, just Go types and a query string.

## Scope
(define epic scope)

## Acceptance Criteria
- Importable Go package (go get), not a standalone binary
- User defines domain model as Go struct
- User registers fields (name → accessor function) and presets (name → field list)
- User registers operations (get, list, summary, custom) with handlers
- DSL parser parses query strings and dispatches to registered handlers
- Field projection applied automatically via Selector
- Scoped grep included as a separate util package
- Batch queries (semicolon-separated) work out of the box
- JSON output always, no text mode
- Zero external dependencies beyond stdlib
- Works as Cobra subcommand (helper to wire into existing CLI)
- Example project demonstrating full integration
