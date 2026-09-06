# TASK-260907-3f11x1: make-scoped-grammar-disclose-positional-arguments-and-pin-completeness-against-the-parser

## Description
In agentquery/grammar.go the scoped GrammarSyntax() text says arguments are comma-separated key=value only; the parser (parser.go parseArg) accepts bare positional values too, and the operation examples rely on it. The completeness claim of grammarExamples is hand-maintained and circular: a narrowing mutant that removes op(positional) from grammarExamples AND the positional clause from Grammar.Arguments survives go test ./... -count=1 (measured by the consumer reviewer in an rsync copy). What is wanted: (1) GrammarSyntax() and Grammar.Arguments truthfully disclose positional arguments (first positional is the element/identifier argument; key=value for the rest). (2) An independently pinned statement-form contract: a test derives the accepted argument forms from the parser (drive parseArg / the parser through a fixture per form: positional, key=value, quoted, batch, projection) and asserts each derived form is named in the grammar prose AND has an example that parses; the test must fail when the positional form is removed from both prose and examples. (3) Existing API unchanged otherwise; go build, go vet, gofmt -l, go test ./... -count=1 green in the foreground; results note lists the mutant re-run (exit 1 now). Work in the Story worktree the spawn provisions, leave it uncommitted, publish the Change Request, handoff with task-board handoff <ID> --role developer. No tag, no push.

## Scope
(define task scope)

## Acceptance Criteria
1. GrammarSyntax() and Grammar.Arguments disclose bare positional arguments truthfully. 2. A parser-derived completeness test fails when the positional form is removed from both the grammar prose and the examples (the consumer reviewer's narrowing mutant now exits 1). 3. Module gates green in the foreground; no other public API change.
