# Flight Logbook

> Institutional memory. Concise, factual, high-signal.
> Newest entries first. One block per insight.

## 2026-09-23

### 1205 — Article now separates alias economics from MCP economics
- FINDING: the revised [field-alias article](articles/field-alias-compression-study.md) labels fixture-backed token counts as historical measurements, session output as a model estimate, and the current formatter/batching behavior as implementation evidence.
- DECISION: retain the field-alias NO-GO only for this schema-once formatter. State no MCP token winner because the repository has no adapter, tool-definition fixture, prompt snapshot, host trace, or aligned workload.
- EVIDENCE: `.research/260923_mcp-token-economics-evidence.md`; current `go test ./...` runs passed in `agentquery/` and `example/`; the tokenizer command was expected-red because `tiktoken` is not installed. No package or benchmark fixture was added.
- CAVEAT: the README article-index summary now carries the same evidence boundary. The broader README overview and SKILL.md retain stale fixed MCP-overhead, no-batching, and break-even claims outside this article-focused slice; correct them in a separate documentation task.

### 1045 — MCP token comparison needs a bounded correction
- FINDING: `references/comparison-example.md` asserts an `internal/fields`-backed MCP implementation, fixed 2,200-token definition cost, no batching, and a ~293-query break-even; this checkout has no MCP adapter or `internal/fields` package. A Go-source search finds only `assets/field-selector.go:4`, which explicitly describes a future MCP server.
- DECISION: revise `articles/field-alias-compression-study.md` as the canonical publication source. Keep proven DSL batching/schema-once formatter facts, but mark MCP token economics unknown without an aligned server, host, protocol/version, definitions, and tokenizer transcript.
- EVIDENCE: `.research/260923_mcp-token-economics-evidence.md`; `go test ./...` from `agentquery/` exited 0. Official MCP SDK documentation shows batching/version behavior is protocol- and SDK-dependent, so the current universal no-batching wording is stale.

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
