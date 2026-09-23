# TASK-260923-hyh12u review verdict — revision 1

## Verdict

**Changes requested** → `to-dev`.

`repeat-of: none`

The editorial structure, scoped MCP comparison, conditional recommendation,
local links, official-source links, Go validation, and simulator arithmetic are
substantively sound. Revision 1 is not acceptable because the composed Change
Request fails its claimed documentation gate and leaves the adjacent article
index inconsistent with the revised evidence classification.

## Acceptance-criteria coverage

Coverage: **7/7 AC rows exercised; 5/7 satisfied**.

| AC row | Driving review check | Result |
| --- | --- | --- |
| Substantive MCP comparison | Article lines 215-243 checked against the accepted evidence map and three official MCP sources | Pass |
| Measurements, estimates, assumptions, and limitations visibly distinct | Article evidence table and Studies 1-3 inspected; simulator assertions rerun | Pass |
| Conditional recommendation | Article lines 235-243 and 275-287 inspected | Pass |
| Eight developer-writer invariants | Manual invariant-by-invariant editorial audit of the frozen article | Pass |
| Links and commands valid | Local link resolver, HTTP status/source checks, Python syntax check, Go commands | Pass |
| Repository documentation checks and adjacent consistency | Exact base-to-candidate `git diff --check`; README Articles row compared with revised article | **Fail** |
| Task-scoped results resource accurately records evidence/checks/caveats | Producer outcome materialized and compared with exact candidate validation | **Fail** |

## Findings

### F1 — The claimed whitespace gate did not test the composed CR delta

Shape: **bypass path around the check**. The producer outcome says
`git diff --check` exited 0, but the exact reviewable delta fails:

```text
git diff --check c5a6fb45d6e028377d6f7c6e006cd8ce5c0d0830 d0cbeee2c8ed7e898978f9ca4698033aaa5a5131
.research/260923_mcp-token-economics-evidence.md:3: trailing whitespace.
.research/260923_mcp-token-economics-evidence.md:4: trailing whitespace.
```

The unqualified worktree check covered only the uncommitted article/logbook
slice and skipped the already-checkpointed research-note part of the composed
candidate. Fix the two whitespace errors, rerun the explicit base-to-candidate
command, and update the results resource so its statement names the frozen
candidate identity it actually checks.

### F2 — The adjacent README article index contradicts the revised article

`README.md:166` still says schema discovery “costs 85 tokens per roundtrip” and
that aliases are “a net loss in 75% of scenarios” without identifying the first
as a historical model input and the second as simulator output conditional on
hard-coded query-mix and eviction assumptions. The revised article correctly
labels those facts at lines 132-178. Because this is the repository's sole
Articles index entry for the publication, leaving it stale fails the explicit
index-consistency/Docs-consistent criteria.

Update only this adjacent Articles summary with the bounded conclusion from the
accepted evidence note. The broader stale MCP claims in the README overview,
SKILL, and comparison reference remain outside this article-focused rework and
are already recorded as a separate caveat.

### F3 — Break-even notation and prose disagree

At article lines 170-178, the formula uses
`queries_between_evictions > schema_cost / savings_per_query`, which requires
18 whole queries for a positive balance when the inputs are 85 and 5, while the
next sentence says 17 queries are required to amortize the cost. State the two
bounds precisely: break-even is 17 (`>=`), and net-positive begins at 18 (`>`).

## Independent evidence

- Candidate identity: all three changed worktree blobs match tree
  `d0cbeee2c8ed7e898978f9ca4698033aaa5a5131`.
- `go vet ./...`, `gofmt -l`, and `go test ./... -count=1` passed in
  `agentquery/`; `go vet ./...` and `go test ./... -count=1` passed in
  `example/`.
- Python syntax checks passed for the three documented scripts.
- Simulator assertions reproduced 16 compact cases, 4 positive cases,
  `(20,20)=-5`, and `(50,50)=115` without rewriting repository artifacts.
- All local Markdown targets checked in the article, evidence map, and README
  exist.
- The three official MCP URLs returned HTTP 200; their bodies support the
  article's version-bound batching, `MAX_BATCH_SIZE = 100`, and
  model-controlled-tool statements.

No repository source file was modified during review.
