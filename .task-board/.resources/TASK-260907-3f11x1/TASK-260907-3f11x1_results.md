# TASK-260907-3f11x1 developer results

## Scope delivered

- Updated `agentquery/grammar.go` so `DSLGrammar().Arguments` and
  `GrammarSyntax()` disclose the first bare positional element/identifier
  argument and direct remaining arguments to `key=value` form.
- Removed the contradictory `key=value only` wording.
- Added `TestGrammarCoversParserAcceptedStatementForms`, whose independent
  fixtures drive the public `Parse` entry point and derive five accepted forms
  from the resulting AST before checking grammar prose and parsing examples.
- No exported signatures or non-grammar behavior changed.

## Acceptance coverage

Coverage: **3 of 3 AC rows driven**.

| AC row | Production call site | Named test / evidence |
| --- | --- | --- |
| 1. Grammar disclosures | `Schema.Query("schema()") -> Schema.introspect() -> DSLGrammar()` and exported `GrammarSyntax()` | `TestSchemaIntrospectionCarriesGrammar` |
| 2. Parser-derived completeness | `Parse() -> parser.parseStatementBody() -> parser.parseArgs() -> parser.parseArg()` | `TestGrammarCoversParserAcceptedStatementForms`; narrowing mutant exits 1 |
| 3. Module gates / unchanged API | Go module entry points and consumer `example` module | Foreground commands below, all exit 0 |

Statement-form coverage: **5 of 5 parser-fixture forms driven**: positional,
`key=value`, quoted, batch, and projection. The quoted-form classifier is
bounded to decoded values containing whitespace or DSL separators, because the
AST intentionally does not retain lexical quote tokens.

## Mutant evidence

| Mutant | What it narrows the gate to | Named failing test | Result / stated bound |
| --- | --- | --- | --- |
| Reviewer narrowing | Removed `op(positional)` from `grammarExamples` and removed positional disclosure from `Grammar.Arguments` together | `TestGrammarCoversParserAcceptedStatementForms/positional` | `go test ./... -count=1` exit **1**: `grammar prose for parser-accepted positional form ... want marker "bare positional"` and `grammar has no parsing example of parser-accepted positional form` |
| Token-preserving prose mutation | Kept the token `positional` but replaced the first-bare-positional element/identifier contract with the weaker `positional values are accepted` | `TestGrammarCoversParserAcceptedStatementForms/positional` | `go test ./... -count=1` exit **1**: `want marker "bare positional"` |

Surviving mutants: **none**.

## Foreground validation

Commands ran after restoring the exact implementation from both mutants.

| Command | Directory | Exit code | Result |
| --- | --- | ---: | --- |
| `go build ./...` | `agentquery/` | 0 | pass |
| `go vet ./...` | `agentquery/` | 0 | pass |
| `gofmt -l .` | `agentquery/` | 0 | pass; empty output |
| `go test ./... -count=1` | `agentquery/` | 0 | pass |
| `go build ./...` | `example/` | 0 | pass |
| `go test ./... -count=1` | `example/` | 0 | pass; no test files |
| `git diff --check` | repository root | 0 | pass |

The candidate remains uncommitted as required. Changed tracked files are only
`agentquery/grammar.go` and `agentquery/grammar_test.go`.
