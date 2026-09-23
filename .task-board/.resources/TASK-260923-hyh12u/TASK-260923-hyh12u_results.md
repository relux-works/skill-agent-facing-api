# TASK-260923-hyh12u — Documentation revision results, revision 2

## Candidate identity

The composed candidate is the repository delta from base
`c5a6fb45d6e028377d6f7c6e006cd8ce5c0d0830` across the four paths listed
below. The captured binary patch has Git blob ID
`05f33b58878889e7f27a9581b519e6bb670f686a`.

| Path | Candidate Git blob ID |
| --- | --- |
| `.research/260923_mcp-token-economics-evidence.md` | `bcc30ff59cd2287bafff1414089534cb023f732d` |
| `LOGBOOK.md` | `103261492dbfeb5f33dafa4e7665bd6bcd71421e` |
| `README.md` | `ef8a7a9cfd6478b3244010429e24ed31bbfc054b` |
| `articles/field-alias-compression-study.md` | `e08d229d9c35449576b0fbad32a38c94310e7de5` |

## Evidence and editorial contract

- The accepted `mcp-token-economics-evidence.md` research note is the only
  factual expansion source.
- `developer-writer-core.md` supplies the eight editorial invariants; it is a
  writing contract, not an additional factual source.
- Revision 1's reviewer verdict supplies the three bounded corrections in this
  revision: composed-delta whitespace, adjacent index consistency, and exact
  break-even notation.

## Changed sections

- Reframed the article around the schema-once header tension and stated the
  bounded question before background.
- Kept historical measurements, model estimates, assumptions, implementation
  facts, reasoning, and unknown MCP economics visibly separate.
- Replaced universal MCP economics with a substantive scoped comparison:
  verified DSL batching and schema-once behavior, protocol/SDK/host-dependent
  failure modes, missing benchmark evidence, and interoperability cases where
  MCP can be preferable.
- Closed with a conditional design decision: reject aliases for this formatter,
  use MCP when interoperability requires it, and measure an aligned workload
  before making transport-token claims.
- Corrected the model boundary: 17 data queries exactly break even for the
  stated 85/5 inputs; net-positive savings begin at 18.
- Updated the sole README article-index row to distinguish historical fixture
  evidence from the conditional simulator and unknown MCP token economics.
- Removed the two trailing-space defects from the accepted research note and
  narrowed the logbook caveat to the still-stale README overview and SKILL.md.

## Validation

All commands ran directly as standalone processes. Logs are in the worktree's
`.temp/TASK-260923-hyh12u/` directory.

| Check | Exit | Evidence |
| --- | ---: | --- |
| `python3 .temp/TASK-260923-hyh12u/validate_revision1_regressions.py` | 0 | Named regression checks passed: composed-document whitespace, exact 17/18 bounds, README evidence classification, and local Markdown targets. |
| The same validator with `--narrow-net-positive-bound` | 1 | Expected-red narrowing mutant changed the in-memory boundary from 18 to 19; `test_break_even_and_net_positive_bounds` failed while the other three tests passed. |
| `git diff --check c5a6fb45d6e028377d6f7c6e006cd8ce5c0d0830` | 0 | The complete base-to-candidate delta has no whitespace errors. |
| Python syntax check for the three documented research scripts | 0 | `generate.py`, `measure.py`, and `simulate.py` compile with a task-local bytecode cache. |
| `python3 .research/session-simulator/simulate.py` | 0 | Reproduced 16 compact scenarios, four positive cases, `(20,20)=-5`, and `(50,50)=115`; tracked outputs remained unchanged. |
| `python3 .research/synthetic-payloads/measure.py` | 1 | Expected-red: `ModuleNotFoundError: No module named 'tiktoken'`; no dependency was installed and historical tokenizer counts were not re-attested. |
| `go test ./... -count=1` in `agentquery/` | 0 | Main package and Cobra extension passed. |
| `go vet ./...` in `agentquery/` | 0 | Passed. |
| `go test ./... -count=1` in `example/` | 0 | Passed; the package has no test files. |
| `go vet ./...` in `example/` | 0 | Passed. |
| HTTP fetch of each of the three official MCP references | 0 / 0 / 0 | Ruby protocol versions, TypeScript request body, and MCP server overview were reachable; bounded source-content patterns were present. |

The regression suite exercises four local obligations derived from the three
revision-1 findings, `4/4`; its deliberate blind spot is external HTTP content, which is validated
by separate fetch and source-content checks rather than inferred from local
link syntax.

## Publication caveats

- The 5-token and 44.5% to 46.6% values remain checked-in historical fixture
  measurements, not tokenizer results independently reproduced in this run.
- The session result is a model conditional on hard-coded constants, query mix,
  and eviction behavior; it is not evidence about a typical agent runtime.
- No repository-local MCP adapter, definitions, prompt snapshot, host trace, or
  aligned workload establishes a transport-wide token winner.
- The broader README overview, SKILL.md, and comparison reference still repeat
  stale fixed MCP-overhead, no-batching, or break-even claims. They remain
  outside this article-focused revision and need a separate correction.
