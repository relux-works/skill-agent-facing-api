# 2026-09-24 — Layer-ladder measurement: where the token saving actually comes from

Reproducible, offline, no model calls. Everything below is a `cl100k_base`
token count of a checked-in fixture produced by the shipped `agentquery` code
paths, or an explicitly labelled synthetic contract.

Artifacts: `.research/layer-ladder/` — `RESULTS.md` (generated report),
`results.json` (machine-readable), `fixtures/` (rendered payloads + manifest),
`mcp/` (derived MCP contracts), `mutants/` (narrowing-mutant harness).

## Why this exists

The prior evidence recovery (`TASK-260924-rjkzmk`) established that the
remembered "70–90% projection saving" had no fixture, denominator or tokenizer
behind it, and that the repository's MCP numbers were byte-ratio guesses. It
recommended publishing nothing about projection.

That left the article with a hole: projection is the pattern's headline feature
and we could not say anything measured about it. This task fills the hole the
only way that survives review — by measuring one concrete scenario end to end,
one rung at a time, with the denominator named at every step.

## The scenario

An agent assembles a sprint stand-up roll-call. For every task on the sprint
page it needs `id`, `status`, `assignee`, `priority`. It does not need `name`,
`description`, `created` or `updated`.

- Data: the existing checked-in `.research/synthetic-payloads/json-{5,20,100,500}.txt`
  fixtures, unchanged. Reusing them keeps this comparable with the 2026-02-12
  alias study and avoids a second generator seed.
- Record contract: 8 fields. Scenario needs 4 of 8.
- Production query: `list() { id status assignee priority }`.
- Row set is identical in every layer. Only the field set and the serialization
  change.

## The ladder

| Layer | What it is | Produced by | Class |
|---|---|---|---|
| L0 | all 8 fields, pretty JSON | `json.MarshalIndent` over the real query result | measurement-constructed baseline |
| L1 | all 8 fields, minified JSON | `Schema.QueryJSONWithMode(HumanReadable)` | production output |
| L2 | 4 fields, pretty JSON | `json.MarshalIndent` over the real query result | measurement-constructed baseline |
| L3 | 4 fields, minified JSON | `Schema.QueryJSONWithMode(HumanReadable)` | production output |
| L4 | 4 fields, header+value compact | `Schema.QueryJSONWithMode(LLMReadable)` → `FormatCompact` | production output |
| L5 | L4 with one-character header aliases | research overlay | **not production** — agentquery ships no aliases |

L0 and L2 are labelled baselines because `agentquery`'s own `--format json`
already emits minified JSON. A pretty-printed full record is what a typical REST
endpoint hands you, which is why it is the ladder's base — but it is not this
library's output and the article must not imply otherwise.

## Result

Raw `cl100k_base` token counts:

| Scale | L0 | L1 | L2 | L3 | L4 | L5 |
|---:|---:|---:|---:|---:|---:|---:|
| 5 | 485 | 346 | 186 | 107 | 65 | 63 |
| 20 | 1,957 | 1,398 | 742 | 423 | 239 | 237 |
| 100 | 9,836 | 7,037 | 3,688 | 2,089 | 1,154 | 1,152 |
| 500 | 48,933 | 34,934 | 18,433 | 10,434 | 5,748 | 5,746 |

Incremental savings, each denominated in the rung below it, across all four
scales:

| Step | What it isolates | Denominator | Range over scales |
|---|---|---|---|
| L0→L1 | minification, full record | pretty full JSON | 28.46–28.66% |
| L1→L3 | projection, 4 of 8 fields | minified full JSON | **69.08–70.31%** |
| L0→L2 | projection, 4 of 8 fields | pretty full JSON | 61.65–62.51% |
| L2→L3 | minification, projected record | pretty projected JSON | 42.47–43.40% |
| L3→L4 | JSON → header+value compact | minified projected JSON | **39.25–44.91%** |
| L4→L5 | one-character header aliases | compact | 0.03–3.08% (**exactly 2 tokens**) |
| L0→L4 | the whole stack | pretty full JSON | **86.60–88.27%** |

### The ordering trap

Two orderings reach the same payload and the same cumulative saving, and split
the credit completely differently:

| Ordering | First step | Second step | Cumulative after two steps |
|---|---|---|---|
| minify → project (scale 100) | L0→L1 = 28.46% | L1→L3 = 70.31% | 78.76% |
| project → minify (scale 100) | L0→L2 = 62.51% | L2→L3 = 43.36% | 78.76% |

Projection is worth 62.51% or 70.31% of its denominator depending purely on
whether minification happened first. Neither number is wrong; a number quoted
without its denominator is. This is the single most important methodological
point for the article.

Which is why the percentage now has its own gate. `reconcile_ladder` closes the
*token* arithmetic and nothing else: a column denominated in the ladder base
instead of the previous rung leaves every token count correct, closes the ladder
exactly, and still publishes the wrong number under a header that claims the
previous rung. Substituting the base denominator at scale 5 turns L1→L3 from
69.08% into 49.28% and L3→L4 from 39.25% into 8.66% — plausible figures, no
failing check (verified: the pre-fix suite ran 32 tests green and `measure.py
--check` exited 0 with that substitution in place). `reconcile_percentages` is what ties each published percentage to
the rung its header names, and mutant M10 plants exactly that substitution in
`build_ladder` to prove the tie holds.

### What the "70–90%" memory probably was

It is close to two real quantities in this scenario, and to neither exactly:

- the projection-only step off a minified full record: 69.08–70.31%
- the whole stack off a pretty full record: 86.60–88.27%

Both are specific to this scenario's 4-of-8-field projection, this fixture set
and `cl100k_base`. A record with a different field-size distribution moves both
numbers. Publish them as "in this scenario", never as a general rate.

### Aliases, again

The header alias overlay saves **exactly 2 tokens** at every scale here — the
4-column header `id,status,assignee,priority` → `i,s,a,p`. The 2026-02-12 study
measured exactly 5 tokens for its 8-column header. Same structural result: the
header is paid once, so the saving is a constant, and its percentage collapses
as the payload grows. Against the 21-token legend an agent must hold to read an
aliased header, aliases break even at 11 queries and never at any scale recover
enough to matter. The 2026-02-12 recommendation stands.

## Amortization

A per-response saving is not a session saving until it has paid off the one-time
contract the agent had to read **to make that particular move**. The one-time
cost of a transition is what its destination rung requires and its source rung
did not. An artifact both rungs require is sunk on both sides and cancels.

Session constants, measured:

- Real `schema()` response for this scenario: **535 tokens** (2,338 bytes).
- Real `schema()` response for the shipped example CLI: **1,154 tokens**.
- Alias legend: **21 tokens**.

Every rung of this ladder — L0 through L5 — is `agentquery` output. L1 and L3
are `QueryJSONWithMode(HumanReadable)`; L4 is `QueryJSONWithMode(LLMReadable)`.
The same session, the same schema, the same introspection. An agent that has
read `schema()` once pays nothing extra to write a projected query instead of a
full one, and nothing extra to ask for the compact format instead of JSON. So
the 535 is sunk on both sides of both of those transitions:

| Comparison | Transition | Charged | Sunk on both sides | One-time cost | Saving/query @20 rows | Break-even |
|---|---|---|---|---:|---:|---:|
| projection (L1→L3) | L1→L3 | nothing | `schema()` | 0 | 975 | 1 query |
| compact (L3→L4) | L3→L4 | nothing | `schema()` | 0 | 184 | 1 query |
| aliases (L4→L5) | L4→L5 | alias legend | `schema()` | 21 | 2 | 11 queries |

Only the alias row has a real incremental cost: the 21-token legend is something
L4 genuinely does not need, and at 2 tokens saved per query it takes 11 queries
to repay and never becomes material.

**Correction against the previous revision of this note.** It charged the full
535-token `schema()` roundtrip to the projection and compact rows, giving
break-evens of 3 and 13 queries and a "Net @10 = −115" for compact at 5 rows.
Those were wrong: they priced an introspection the agent had already paid for on
the other side of the same transition. The true incremental figures are 0, 1
query, and +420. `measure.py` now derives each row's one-time cost from its
transition and `validate_amortization` refuses any row that charges an artifact
its source rung already held, so the error cannot be reintroduced silently.

The framing built on top was wrong for the same reason. "Break-even is a
function of payload size, so 'the DSL pays for itself immediately' is only true
above a certain response size" is a claim about **DSL adoption versus a non-DSL
baseline** — and this ladder has no non-DSL rung. L1 is not a REST response; it
is `agentquery` output. **This measurement cannot price DSL adoption at all.**
Whether one `schema()` roundtrip is worth paying against a plain REST client
that needs no contract is a different comparison against a baseline nothing here
measures, and the article must not use these numbers to answer it.

What does survive: `schema()` is paid **only if the agent introspects**. A skill
file that documents the query grammar in the agent's prompt moves that cost
elsewhere rather than removing it.

## MCP comparison — synthetic contract, explicitly not a host measurement

Protocol revision `2026-07-28`, shapes taken from the primary specification:
<https://modelcontextprotocol.io/specification/2026-07-28/server/tools>.
No SDK was executed, no server was run, no host transcript was captured.

**Revision freshness is probed, not asserted.** `mcp/spec-probe.json` records a
dated primary-source read: on 2026-09-24 `/specification/latest` returned a 307
to `/specification/2026-07-28`, and `server/tools` pages resolve 200 for
`2024-11-05`, `2025-03-26`, `2025-06-18`, `2025-11-25` and `2026-07-28`. The
probe carries a control — revision `2099-01-01`, which cannot exist, returns
404 — so a site outage that 404s everything cannot be read as "that revision was
withdrawn". `measure.py` refuses any contract pinned to a revision other than
the probed latest one, and refuses a probe whose control resolved or whose
control read failed. The previous revision of this note pinned `2025-06-18`,
four revisions behind current; that is what the gate now prevents.

The tool contracts are **derived from** the real `schema()` output
(`mcp/build_contract.py`), so both sides expose the same operations, the same
parameters and the same descriptions. Neither side gets a strawman.

### The session-constant comparison has no single number

A `Tool` requires only `name` and `inputSchema` under both `2025-06-18` and
`2026-07-28` (read from `schema/<revision>/schema.ts`); `description`,
`outputSchema`, `annotations`, `_meta` and — new in `2026-07-28` — `icons` are
all optional. So a *minimal* derived contract is byte-identical under either
revision, which is exactly why the minimal figure is a **floor** and not a
typical current-revision server.

What `2026-07-28` materially expands is that optional surface. On the revision's
own `server/tools` page the occurrence counts move `outputSchema` 4→15,
`structuredContent` 6→15, `_meta` 0→9, `icons` 22→31, `annotations` 22→33. So
the contract is now emitted in two profiles: `minimal`, and `structured`, which
adds the `outputSchema` a server publishes to return structured content —
derived from the same `schema()` field list, so it is still not a strawman.

| Contract | Tools | Profile | MCP `tools/list` result | agentquery `schema()` | MCP − DSL | MCP cheaper? |
|---|---:|---|---:|---:|---:|---|
| scenario (1 read op + 2 built-ins) | 3 | minimal | 471 tokens | 535 tokens | −64 | yes |
| scenario (1 read op + 2 built-ins) | 3 | structured | 676 tokens | 535 tokens | +141 | no |
| example CLI (6 reads + 3 mutations) | 9 | minimal | 1,490 tokens | 1,154 tokens | +336 | no |
| example CLI (6 reads + 3 mutations) | 9 | structured | 1,774 tokens | 1,154 tokens | +620 | no |

**The direction of the scenario comparison inverts between profiles.** The
previous revision of this note quoted the 471-vs-535 margin — 64 tokens, 12% —
as evidence that MCP's tool definitions are cheaper than `schema()`. That margin
does not survive contact with the current revision's structured-output surface:
add the `outputSchema` a `2026-07-28` server would publish and the same contract
costs 676 against 535. Neither number settles the question; a claim in either
direction has to name its profile.

### The envelope is not a fixed cost — it depends on the payload's quotes

An MCP tool result embeds the payload as a JSON **string**, so every `"` in the
payload is escaped and charged a second time. Pricing only one payload format
would have produced a false "the envelope adds a fixed amount" claim.

| Scale | Payload layer | `"` in payload | Payload tokens | MCP − DSL |
|---:|---|---:|---:|---:|
| 5 | L4 compact | 0 | 65 | +51 |
| 5 | L3 minified JSON | 80 | 107 | +59 |
| 5 | L1 full minified JSON | 160 | 346 | +69 |
| 500 | L4 compact | 0 | 5,748 | +51 |
| 500 | L3 minified JSON | 8,000 | 10,434 | +1,049 |
| 500 | L1 full minified JSON | 16,000 | 34,934 | +2,049 |

Measured range: **+51 to +2,049 tokens per call**. The quote-free compact
payload pays exactly +51 at every scale — that is the bare envelope. Everything
above it is escaping, and it tracks the quote count, not the payload size: the
500-row compact payload is 53× the size of the 5-row JSON payload and pays *less*
framing excess (0 vs 8 tokens).

Subtracting the bare envelope, the excess is 8/18, 38/78, 198/398 and 998/1,998
tokens for L3/L1 at the four scales — L1 carries exactly twice L3's quotes, and
the excess ratio tightens toward 2 as the payload grows (2.25 → 2.00). It works
out to roughly one extra token per eight payload quotes, which is an
observation, not a law.

**This is a real argument for compact output that only appears inside MCP**:
a JSON payload pays a framing surcharge there that it does not pay when a CLI
writes it to stdout.

Three further conclusions, and three things the article must not say:

1. **MCP carries field projection fine.** `fields` is an ordinary tool argument
   in the derived `inputSchema`. Projection is a server design choice, not a
   protocol capability. The current `SKILL.md` framing ("Why not MCP?") should
   not imply otherwise.
2. **The tool-definition cost is not automatically larger than the DSL's — and
   it is not automatically smaller either.** For the 3-tool scenario contract
   MCP's `tools/list` is cheaper than agentquery's `schema()` on the minimal
   profile (471 vs 535) and dearer on the structured one (676 vs 535). What the
   measurement does rule out, on every profile and both contracts, is the
   repository's existing "~2,000–3,000 tokens of dead weight" claim: the largest
   figure measured here is 1,774 tokens for a nine-tool contract, and the
   scenario contract is a quarter of that. That claim is unsupported by anything
   measured here or anywhere else in this repo. The article should say the cost
   is comparable and profile-dependent, not that either side wins.
3. **The real asymmetry is when the cost is paid, not how big it is.** A host
   injects `tools/list` unconditionally at session start; `schema()` is paid
   only if the agent asks. That is a host-behaviour difference, and this
   measurement does not quantify it.

Limitations, restated because they are load-bearing: no host framing, no
tool-use scaffolding, no system-prompt text, no server implementation, wire JSON
only, one tokenizer. Two further bounds on the session-constant table: the
`minimal` profile is a floor that no real current-revision server would publish,
and the `structured` profile is itself a lower bound on a fully populated
`2026-07-28` contract, since it counts no `icons`, no `_meta` and no richer
`annotations`. The revision pin is fresh as of the dated probe, not for all
time; re-running `mcp/build_contract.py` against a newer probe is what keeps it
honest.

## Adapter evidence: where projection is pushed down and where it is not

Both repositories are public and both blob references below were resolved
against the public commit via the GitHub API.

**`relux-works/skill-jira-management` — projection pushed down to the backend.**
Commit `5c9b31fddb0495d2eaee32564d153fc6955a35b2`.
`APIFieldsFromSelector` maps the DSL's selected fields onto Jira REST field
names, and both `get` and `list` hand the result to the API as the `fields`
query parameter. The backend returns only those fields; the wrapper is not
discarding data it already paid to fetch.

- <https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go#L39-L54> — `APIFieldsFromSelector`
- <https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go#L300> — `get` pushdown
- <https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go#L359-L360> — `list` pushdown
- <https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/jira/issues.go#L12-L27> — `q.Set("fields", ...)`

**`relux-works/skill-confluence-management` — projection applied locally only.**
Public commit `10e342b1c60e52233cc4ef662e70e6b6dcc8011b`.
`getPageV2` sends only `body-format`; `getPageV1` sends a **fixed** expand list
`version,space,ancestors,metadata.labels`, unrelated to what the agent
projected. The DSL projection still shrinks the agent's context, but the wrapper
fetched the full payload regardless.

- <https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/confluence/pages.go#L19-L26> — v2, `body-format` only
- <https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/confluence/pages.go#L28-L45> — v1, fixed expand list

This is the conclusion the article should draw: **field projection at the agent
boundary always saves agent context; it saves backend work only when the adapter
pushes the selection into the upstream request.** Jira shows the first shape,
Confluence the second. Neither is a defect — Confluence's v2 API has no
equivalent field parameter for these calls — but they are different claims and
must not be merged into "projection makes it faster".

## What is still not measured

- No REST server, no HTTP transfer, no latency, no backend cost. Everything here
  is a serialized payload's token count.
- No host was measured, for MCP or for the DSL. There is no number here for what
  a real agent session actually costs.
- One tokenizer (`cl100k_base`). Other tokenizers will move every percentage.
- One field-size distribution. The scenario keeps 4 short fields and drops one
  long free-text `description`; a projection that drops four short fields and
  keeps the description would produce a much smaller saving.
- Filtering is deliberately out of the ladder. Filtering changes the row set,
  and the ladder's whole premise is that the row set does not change.

## Reproducing

```bash
cd .research/layer-ladder
go run ./cmd/genfixtures            # renders fixtures through the real code paths
/usr/bin/python3 mcp/build_contract.py
/usr/bin/python3 measure.py         # writes results.json and RESULTS.md
go test ./... -count=1
/usr/bin/python3 -m unittest discover -s tests
/usr/bin/python3 mutants/run.py     # 17 narrowing mutants, all must be killed
```

`/usr/bin/python3` is the interpreter that carries `tiktoken` 0.14.0 on this
workstation; Homebrew's `python3` does not.
