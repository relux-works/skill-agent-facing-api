# Flight Logbook

> Institutional memory. Concise, factual, high-signal.
> Newest entries first. One block per insight.

## 2026-09-06

### 1520 — gofmt drift already on main at 656ad0a
- FINDING: `gofmt -l agentquery` reported `agentquery/types.go` at base commit `656ad0a`, before any of this task's changes. Comment-alignment drift in `ParameterDef`, `SortDirection` and `MutationContext`.
- SCOPE: `agentquery/types.go:57`, `agentquery/types.go:88`, `agentquery/types.go:118`.
- DECISION: fixed with `gofmt -w` under TASK-260906-1auqyl so the module's gofmt gate is green. Whitespace only; the exported API is unchanged.
- NOTE: no formatting gate runs in CI (no CI exists), which is how the drift reached main.

### 1515 — DSL error-guidance delta adopted into agentquery
- MILESTONE: the BUG-260903-12avv2 delta (7 files, 469/37) applied onto `656ad0a` and validated in this repo. Adds `Grammar`/`DSLGrammar()`/`GrammarSyntax()` to `schema()`, and `NewUnknownOperationError`/`IsUnknownOperation`/`ParseError.Operation`/`OperationPos`.
- FINDING: the exported board patch is byte-identical to the producer worktree state, so the resource and the worktree were one delta, not two variants.
- DECISION: unknown operation outranks a positional argument error. `attributeTokenizeError` (`agentquery/parser.go:49`) walks the tokens emitted before a tokenizer failure so `items(status in [a,b])` reports the unknown operation and the known-operation list instead of `unexpected character "["`.
- FINDING: the grammar text is derived from the tokenizer token table and escape map rather than restated, so it cannot drift. Pinned by `agentquery/grammar_test.go`; 9 narrowing mutants, 9 killed, no survivors.
- STATUS: handed to review; landing on main and tagging `agentquery/v1.6.0` is the orchestrator's step.
