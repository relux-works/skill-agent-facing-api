# TASK-260906-1auqyl — review verdict, Change Request rev 1

- Verdict: **accepted**
- Reviewer run: `RUN-260906-f6aa5a`
- Change Request: `CR-TASK-260906-1auqyl-1` revision `1`
- Base OID: `656ad0a73dbffc92e732ed95c39a7ae3197f28f1`
- Candidate tree OID: `43349d7f86160623b02f9dd63f85b6a7e457855b`
- repeat-of: none

Every claim below was re-measured by this run, in an isolated copy at
`/tmp/aq-review` outside the story worktree, not read from the producer note.

## 1. Candidate integrity — reconstructed, not trusted

The exported patch resource hashes to the declared value and independently
rebuilds the candidate tree byte for byte:

```
$ shasum -a 256 TASK-260906-1auqyl_change-request_rev1.patch
da8a42afa424fc46ee0e88f1693de8ccf8d4ba22e1cb22baddfec8d0dfd2059e
declared:  da8a42afa424fc46ee0e88f1693de8ccf8d4ba22e1cb22baddfec8d0dfd2059e

$ git checkout 656ad0a && git apply --3way <patch> && git add -A && git write-tree
43349d7f86160623b02f9dd63f85b6a7e457855b
declared candidate OID: 43349d7f86160623b02f9dd63f85b6a7e457855b
```

The resource, the worktree and the declared tree OID are one delta, not three
variants. Every hunk of all 9 changed paths was read.

## 2. Gates — rerun in the foreground by this run (AC 2)

Module `agentquery/` in the story worktree:

```
$ go build ./...      -> exit 0, no output
$ go vet ./...        -> exit 0, no output
$ gofmt -l .          -> no output (clean)
$ go test ./... -count=1
ok  github.com/relux-works/skill-agent-facing-api/agentquery          0.627s
ok  github.com/relux-works/skill-agent-facing-api/agentquery/cobraext 0.755s
   (-v: 391 --- PASS, 0 --- FAIL)
```

Consumer module `example/` built against the worktree library through its
`replace` directive: `go build` OK, `go vet ./...` OK, `gofmt -l .` clean.

## 3. API surface — measured additive, not asserted (AC 3)

`go doc -all` diffed at three points. `agentquery/v1.5.5` and base `656ad0a`
produce an identical document, so the base is the released surface. base ->
candidate produces **only `>` lines: zero removals, zero modifications**.
`agentquery/cobraext` diff is empty.

Added surface, complete:

| Symbol | Kind |
| --- | --- |
| `type Grammar struct` (8 fields) | new exported type |
| `func DSLGrammar() Grammar` | new exported func |
| `func GrammarSyntax() string` | new exported func |
| `func NewUnknownOperationError(name string, pos Pos, known []string) *ParseError` | new exported func |
| `func (e *ParseError) IsUnknownOperation() bool` | new exported method |
| `const UnknownOperationHint` | new exported const |
| `ParseError.Operation`, `ParseError.OperationPos` | new exported fields |
| `ParseError.KnownOperations`, `ParseError.Hint` | new exported fields |

The last three rows go beyond the five symbols AC 3 enumerates. The producer
note declares them explicitly rather than hiding them under the AC list; a
superset that is named is additive and correct, not a deviation.

Behavioural note on an existing symbol: `(*ParseError).Error()` keeps its
signature but now appends `; known operations: ...; <hint>` to unknown-operation
messages. Any downstream exact-string match on the old text changes. That is the
point of the change and is minor-version appropriate for `v1.6.0`; the suite
pins that *known*-operation errors keep their exact prior text.

## 4. Attacking the gates, not reading them

### 4a. Producer mutants re-measured independently

The producer's harness mutates files in place inside the story worktree. It was
re-pointed at `/tmp/aq-review/agentquery` and rerun there. All 9 mutants (M1,
M2, M3, M4, M4b, M5, M6, M7, M8) reproduce as KILLED with the same named tests.
No survivors. The table is not delete-only: M1, M2, M3, M4, M7 and M8 keep the
gate present and weaken it to admit one member of its class. M6 is the correct
token-preserving shape — `schema()` keeps the `grammar` key and empties its
value — and it is killed behaviourally, not by a static check.

### 4b. Reviewer-authored narrowing mutants — not declared by the producer

Six additional narrowing mutants written by this run, executed against the
behavioural suite in the isolated copy (`/tmp/aq-review/mutants2.py`):

| Mutant | Class it wrongly admits | Result |
| --- | --- | --- |
| R1 unknown-op-wins fires only when the operation is at offset 0 | unknown op in the 2nd statement of a batch | KILLED — `TestUnknownOperationWinsOverArgumentPosition` |
| R2 known-operation list built with the built-in `schema` filtered out | an incomplete recovery list | KILLED — `TestUnknownOperationWinsOverArgumentPosition` |
| R3 tokenizer attribution skipped when the error carries `Got` | every real tokenizer failure | KILLED — both attribution tests |
| R4 `Error()` omits the known list whenever `Got` is set | the entire unknown-op class | KILLED — `TestUnknownOperationWinsOverArgumentPosition` |
| R5 parser-stage unknown-op verdict only for argument-less statements | `items(x=1)` | KILLED — `TestUnknownOperationWinsOverArgumentPosition` |
| R6 grammar keeps its prose, drops `Escapes` | a grammar that under-advertises the tokenizer | KILLED — `TestGrammarEscapesRoundTrip`, `TestSchemaIntrospectionCarriesGrammar` |

Survivors: none. The gates hold under attacks the producer did not anticipate.

### 4c. Production call site — verified, not inferred

`Schema.Query` -> `Schema.Parse` -> `Parse(input, s.parserConfig())` is the same
entry point the tests drive; the guidance is not a helper called from nowhere.
Driven end to end through the built `taskdemo` CLI:

```
$ taskdemo q 'items(status in [a,b]) { id }' --format json
Error: parse error at 1:1: unknown operation "items" (got "items"); known
operations: count, create, delete, distinct, get, list, schema, summary, update;
see schema() for the contract and schema(operation=NAME) for one signature

$ taskdemo q 'list(status in [a,b])' --format json
Error: parse error at 1:16: unexpected character "[" (got "[")   # known op keeps positional text
```

`schema() --format json` carries the full `grammar` block with `statement`,
`batch`, `arguments`, `values`, `escapes`, `tokens`, `examples`. This is exactly
the CLI-boundary bound the producer declared as untested; this run exercised it
manually and it holds. The bound itself stays true and stays declared.

### 4d. Bypass-path probes

Eleven malformed inputs driven through `Schema.Parse` (leading `;;`, leading
newlines, no space after `;`, a `;` hidden inside a quoted string, unterminated
parens / string / projection, no parens at all, spaced parens, uppercase). The
unknown-operation verdict wins in 10 of 11, including through a quoted-string
`;` — the token walk is not fooled by a separator inside a literal.

## 5. Stated bounds (not defects)

1. **Missing batch separator degrades the guidance.** `list() { id } items(x in [q])`
   reports `unexpected character "["` attributed to `list`, with no unknown-op
   verdict, because `atStart` only resets on `;`. The input is not a well-formed
   batch, and attributing to the statement actually being parsed is defensible.
   Named so a future change does not discover it as a surprise.
2. **`IsUnknownOperation()` does not survive a JSON round-trip with an empty
   known list.** `knownOperations` is `omitempty`, so an unknown-op verdict from
   a schema with zero operations decodes with `KnownOperations == nil` and the
   predicate returns false. Unreachable in practice — `schema` is always
   registered — so the list is never empty.
3. **Reverse grammar drift is pinned at token level only.** `len(Tokens) ==
   int(tokenEOF)+1` fails if the tokenizer gains a token, but dropping an entry
   from `grammarExamples` fails nothing. Structural claims are pinned; prose and
   example completeness are not.

## 6. Deviations from the exported patch — both legitimate

- `agentquery/types.go` (+10/-10, whitespace only). Verified independently:
  `gofmt -l .` on a clean `656ad0a` checkout already reported `types.go`. The
  drift pre-dates this task; AC 2 requires a green `gofmt`, so fixing it here is
  correct. `go doc -all` confirms zero surface impact.
- `LOGBOOK.md` (new, untracked, repo root). Documentation, outside both Go
  modules, affects no gate. Landing it is the orchestrator's call.

Both are declared in the producer note. A pre-existing failure reported as a
pre-existing failure is the correct handling, not a concealed scope creep.

## 7. AC coverage

| AC | Verified by this run | Result |
| --- | --- | --- |
| 1 delta applies to main, hunks read, deviations named | patch -> tree OID reconstruction; full hunk read; §6 | PASS |
| 2 build / vet / gofmt / test in the foreground, quoted | §2 | PASS |
| 3 API surface listed exactly, additive | `go doc -all` three-way diff, §3 | PASS |
| 4 drift test derived from parser; unknown-op + unparsable args reports known list and `schema()` pointer | §4a–4d, 15 mutants, 11 probes, CLI drive | PASS |
| 5 results note attached, uncommitted, no tag/push | `git log 656ad0a..HEAD` empty; no `agentquery/v1.6*` tag; resource present | PASS |

5 of 5 AC rows verified. The producer's behavioural ratio (5 of 5 named tests
with production call sites) was re-checked against the test file and is accurate.

## 8. Verdict

**Accepted.** The change is additive, the gates are attacked rather than
described, 15 narrowing mutants (9 producer + 6 reviewer-authored) die against
the behavioural suite with no survivors, and the guidance is reachable from the
real CLI entry point. Landing on `main` and tagging `agentquery/v1.6.0` remain
the orchestrator's step; nothing was committed, tagged or pushed by this run.
