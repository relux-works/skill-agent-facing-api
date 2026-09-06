# TASK-260906-1auqyl — adopt the DSL error-guidance delta into agentquery

Workspace: `.temp/STORY-260906-1mz2ft/worktree` (branch `task-board/story/STORY-260906-1mz2ft`, base `656ad0a`).
Work is left **uncommitted** for the handoff snapshot. No tag, no push from this run.

## 1. Adoption of the delta (AC 1)

Source of truth used: the consumer board resource
`file:///Users/alexis/src/relux-works/skill-project-management/.task-board/.resources/BUG-260903-12avv2/BUG-260903-12avv2_agentquery.patch`.

Before applying, the exported patch was checked against the producer worktree
`/Users/alexis/src/relux-works/skill-project-management/.temp/BUG-260903-12avv2/agentquery-worktree`
(base `656ad0a`, same as ours):

```
worktree-export bytes: 23264
IDENTICAL to exported patch
```

so the resource and the worktree state are the same delta, not two variants.

Applied with `git apply --3way`:

```
Applied patch to 'agentquery/error.go' cleanly.
Falling back to direct application...     # agentquery/grammar.go (new file)
Falling back to direct application...     # agentquery/grammar_test.go (new file)
Applied patch to 'agentquery/parser.go' cleanly.
Applied patch to 'agentquery/query.go' cleanly.
Applied patch to 'agentquery/render.go' cleanly.
Applied patch to 'agentquery/schema.go' cleanly.
exit=0
```

Resulting 7-file delta, matching the described 469/37:

```
 agentquery/error.go        |  71 +++++++++++++--
 agentquery/grammar.go      | 100 +++++++++++++++++++++
 agentquery/grammar_test.go | 212 +++++++++++++++++++++++++++++++++++++++++++++
 agentquery/parser.go       |  92 ++++++++++++++++----
 agentquery/query.go        |   7 +-
 agentquery/render.go       |  13 +--
 agentquery/schema.go       |  11 +++
 7 files changed, 469 insertions(+), 37 deletions(-)
```

Every hunk was read. Re-diffed after all work (mutant harness included) against the
exported patch, ignoring `index` lines: **7-file delta IDENTICAL to exported patch**.
No hunk of the delta was modified here.

### Deviation from the exported patch: one extra file

`agentquery/types.go` (+10/-10, whitespace only) is **added to the change** and is
not part of the exported patch. Reason: `gofmt -l .` on base `656ad0a` already
reported `types.go` — a pre-existing comment-alignment drift in `ParameterDef`,
`SortDirection` and `MutationContext`, untouched by the delta. AC 2 requires
`gofmt -l` to pass, so the file was run through `gofmt -w`. It is formatting only:
no identifier, type, tag or comment text changed, and the exported API diff below
is unaffected.

### Deviation from the exported patch: a second extra file

`LOGBOOK.md` (new, repo root, untracked) is added. The board's handoff guard refuses
`to-review` while the "recorded in logbook" checklist item is unchecked, and this repo
had no `LOGBOOK.md`; there is no `task-board logbook` command. Two entries were written:
the pre-existing gofmt drift, and the adoption itself. Documentation only, outside the
Go modules, so it does not affect any gate. If the orchestrator prefers a release commit
without it, drop `LOGBOOK.md` before landing — nothing else depends on it.

Total working tree: **8 tracked files, 479 insertions, 47 deletions**, plus the untracked
`LOGBOOK.md`.

### Review notes on the delta (no change made)

- `Grammar.Batch` and `Grammar.Projection` contain em dashes. The repo's recent
  em-dash style pass (`f79d21b`, `56d0a3a`) was README-scoped, and em dashes are
  used throughout `agentquery/*.go` comments, so this is consistent with the
  codebase. Flagged for the reviewer, not changed.
- `IsUnknownOperation` is a method on `*ParseError`, not a package-level function.
  It is nil-safe (`e != nil &&`).
- `attributeTokenizeError` returns the original `*ParseError` untouched when the
  error is not a `*ParseError` or when no statement identifier precedes the failure
  offset — the fallback fires on genuine absence of attribution, not on a read failure.

## 2. Gates (AC 2) — all foreground, real exit codes

Module: `agentquery/` (`cd .temp/STORY-260906-1mz2ft/worktree/agentquery`).

```
=== go build ./... ===
exit=0
=== go vet ./... ===
exit=0
=== gofmt -l . ===
exit=0 (no filenames listed = clean)
=== go test ./... -count=1 ===
ok  	github.com/relux-works/skill-agent-facing-api/agentquery	0.856s
ok  	github.com/relux-works/skill-agent-facing-api/agentquery/cobraext	0.333s
exit=0
```

Additionally, the `example/` module (separate module, `replace` → `../agentquery`):

```
$ cd example && go build ./...
example build exit=0
```

End-to-end smoke through the real CLI entry point (`cobraext` `q` command):

```
$ taskdemo q 'items(status in [a,b]) { id }' --format json
Error: parse error at 1:1: unknown operation "items" (got "items"); known operations: count, create, delete, distinct, get, list, schema, summary, update; see schema() for the contract and schema(operation=NAME) for one signature
exit=1
```

`taskdemo q 'schema()' --format json` returns the `grammar` block with `statement`,
`batch`, `arguments`, `values`, `escapes` (`\"  \\  \n  \t`), `projection`,
`tokens` (10 entries, tokenizer order) and 7 `examples`.

## 3. Added public API surface (AC 3)

Measured, not asserted: `go doc -all` for `agentquery` and `agentquery/cobraext`
at `HEAD` (656ad0a) vs the applied tree, then diffed.

Added:

| Symbol | Kind |
| --- | --- |
| `type Grammar struct{ Statement, Batch, Arguments, Values string; Escapes []string; Projection string; Tokens, Examples []string }` | new exported type |
| `func DSLGrammar() Grammar` | new exported func |
| `func GrammarSyntax() string` | new exported func |
| `func NewUnknownOperationError(name string, pos Pos, known []string) *ParseError` | new exported func |
| `func (e *ParseError) IsUnknownOperation() bool` | new exported method |
| `const UnknownOperationHint = "see schema() for the contract and schema(operation=NAME) for one signature"` | new exported const |
| `ParseError.Operation string`, `ParseError.OperationPos *Pos` | new exported fields |
| `ParseError.KnownOperations []string`, `ParseError.Hint string` | new exported fields (beyond the five the AC names) |

Additive confirmed: the `go doc -all` diff for `agentquery` contains **only `>` lines**
— no exported signature was changed or removed. The `agentquery/cobraext` API diff is
**empty**. Unexported additions (`stringEscapes`, `stringUnescapes`, `grammarExamples`,
`attributeTokenizeError`, `parser.parseStatementBody`, `Schema.operationNames`) are not
part of the surface.

Runtime-observable additions outside the type surface: `schema()` introspection gains a
`grammar` key (`agentquery/schema.go:269`); no existing key changed.

## 4. Test evidence (AC 4)

`grammar_test.go` derives the advertised grammar from the parser rather than
restating it:

- `TestGrammarExamplesParse` — parses every advertised example; asserts each
  tokenizer token type is named exactly once and that `len(Tokens) == int(tokenEOF)+1`;
  re-tokenizes each punctuation token name to prove the name matches an emitted token.
- `TestGrammarEscapesRoundTrip` — `len(Escapes) == len(stringEscapes)`; each escape
  parses to its decoded byte through the real tokenizer and renders back through
  `Render`; an unlisted escape (`\q`) must stay verbatim so the list cannot lie.
- `TestSchemaIntrospectionCarriesGrammar` — `schema()` carries a non-empty `Grammar`,
  it JSON-marshals with the documented keys, and `GrammarSyntax()` names the batch
  separator and all four escapes.
- `TestUnknownOperationWinsOverArgumentPosition` — four inputs including
  `items(status in [a,b]) { id }` (tokenizer-stage failure) and
  `list(); items(status in [a,b])` (second statement in a batch): each must report
  `unknown operation "items"` at the operation's column, carry
  `KnownOperations == [get list schema]` and the `schema(operation=NAME)` pointer, and
  must **not** regress to `expected ')'` / `unexpected character`. Also covers the
  `ValidateAST` path for a permissively parsed AST.
- `TestPermissiveParseAttributesErrorsToOperation` — a permissive parse attributes the
  error to `items` at column 13 without claiming an unknown operation, and a *known*
  operation with the same broken arguments keeps its exact positional message at both
  the tokenizer and the parser stage.

### AC coverage ratio

AC 4 is the only behavioral AC row; it decomposes into 5 named behaviors.
**5 of 5 driven by named committed tests**, with production call sites:

| Behavior | Named test | Production call site |
| --- | --- | --- |
| grammar derived from tokenizer token table (drift) | `TestGrammarExamplesParse` | `DSLGrammar()` grammar.go:67, reached from `Schema.introspect()` schema.go:269 |
| escapes derived from tokenizer, round-trip through renderer | `TestGrammarEscapesRoundTrip` | `stringEscapes[next]` parser.go:232; `stringUnescapes[value[i]]` render.go:45 |
| `schema()` carries grammar; `GrammarSyntax()` one-liner | `TestSchemaIntrospectionCarriesGrammar` | schema.go:269; grammar.go:97 |
| unknown op wins over unparsable arguments, carries known list + hint | `TestUnknownOperationWinsOverArgumentPosition` | `attributeTokenizeError` parser.go:34/84; `parseStatement` parser.go:399; `ValidateAST` query.go:71 |
| permissive parse attributes to operation; known op keeps positional text | `TestPermissiveParseAttributesErrorsToOperation` | parser.go:34; parser.go:411 |

ACs 1, 2, 3 and 5 are process/gate rows evidenced by the command output quoted above,
not by tests.

**Stated bound:** the unknown-operation message rendered through the Cobra `q` command
was verified by the manual `taskdemo` smoke above, not by a committed test. The
`cobraext` suite covers the command wiring but does not assert this message text. If a
future change strips the hint at the CLI boundary only, the library tests still pass.

### Negative / narrowing mutants

Harness: `.temp/TASK-260906-1auqyl/mutants.py`, log
`.temp/TASK-260906-1auqyl/TASK-260906-1auqyl_mutants.log`. Each mutant keeps the gate
present and weakens it to admit one member of the class it must reject; the executor is
the behavioral suite `go test ./... -count=1`, not a static checker. Baseline green
before each; tree restored after each.

| Mutant | Narrows the gate to | Failing named test |
| --- | --- | --- |
| M1 `atStart = false` on `;` in `attributeTokenizeError` | unknown-op-wins only for the first statement of a batch | `TestUnknownOperationWinsOverArgumentPosition`, `TestPermissiveParseAttributesErrorsToOperation` |
| M2 attribute only when `parseErr.Expected == ""` | attribution only for errors without an `expected` field | `TestPermissiveParseAttributesErrorsToOperation` |
| M3 `ValidateAST` returns the old bare `ParseError` | unknown-op verdict without known list or hint | `TestUnknownOperationWinsOverArgumentPosition` |
| M4 `Hint: ""` in `NewUnknownOperationError` | error names the op and known list but drops the `schema()` pointer | `TestUnknownOperationWinsOverArgumentPosition` |
| M4b drop `sort.Strings(sorted)` | known list present but unordered | `TestUnknownOperationWinsOverArgumentPosition` |
| M5 tokenizer stops emitting `;` while `tokenTypeName` still returns `"';'"` | token-name-preserving behavioral break (attacks the name-inspecting gate) | `TestGrammarExamplesParse` + 23 others incl. `TestParse`, `TestQuery_BatchTwoStatements` |
| M6 `"grammar": Grammar{}` | key present in `schema()` JSON but empty (token-preserving) | `TestSchemaIntrospectionCarriesGrammar` |
| M7 `stringEscapes['n'] = 'x'` | `\n` still advertised, decodes to the wrong byte | `TestParse` |
| M8 renderer stops escaping `\t` | one escape decodes but does not round-trip | `TestGrammarEscapesRoundTrip` |

**No survivors.** 9 of 9 mutants killed by at least one named test. M5 and M6 satisfy
the token-preserving requirement: both leave the searched-for string (`"';'"`,
`"grammar"`) in place and change behavior, and both are executed by the behavioral suite.

## 5. Handoff (AC 5)

Left uncommitted in the story worktree; artifacts attached as task-scoped resources.
Landing on main and tagging `agentquery/v1.6.0` remain the orchestrator's step.
