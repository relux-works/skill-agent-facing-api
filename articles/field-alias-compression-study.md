# How Far to Optimize an Agent's Context

**One request, one row set, measured rung by rung: minification, field
selection, compact representation, the same two choices over MCP, and the
abbreviated header where the layers stop paying.**

---

An agent runs a sprint stand-up roll-call: who owns each task, what state is it
in, how urgent is it. One list call answers that. The tracker returns complete
task records, eight fields each, and every one of those fields lands in the
model's context:

~~~json
[
  {
    "assignee": "heidi",
    "created": "2025-11-24",
    "description": "The our caching layer has issues when token expiration. We should rewrite it to improve reliability.",
    "id": "TASK-0001",
    "name": "Implement metrics dashboard endpoint",
    "priority": "critical",
    "status": "in-progress",
    "updated": "2025-11-27"
  },
  {
    "assignee": "rosa",
    "created": "2025-04-22",
    "description": "The the worker pool has issues when failover events. We should decouple it to support horizontal scaling.",
    "id": "TASK-0002",
    "name": "Extract input sanitization filters",
    "priority": "critical",
    "status": "open",
    "updated": "2025-04-29"
  }
]
~~~

Three more records follow in the same shape. At five rows the whole response is
1,732 bytes and 485 tokens under `cl100k_base`. The roll-call needed four fields
per record: `id`, `status`, `assignee`, `priority`. The other four, including the
free-text `description`, were paid for and discarded. The record bodies are
synthetic filler; only their size and shape matter here.

## The question and its boundary

That gap is where context optimization starts. This article asks where it
stops: **for an agent-facing read layer, which optimization layers repay their
complexity, and which one is small enough that the machinery around it costs
more than the tokens it saves?**

It walks one fixed scenario up a ladder of six representations of the *same rows*
— only the field set and the serialization change — and reports each rung against
the rung below it. It then locates the practical boundary in two adapters built
on this repository's library.

Every number comes from the offline fixtures in
[`.research/layer-ladder`](../.research/layer-ladder/), counted with `tiktoken`
0.14.0 and the `cl100k_base` encoding. The
[measurement note](../.research/260924_layer-ladder-measurement.md) records the
scenario, the reproduction commands, and the limits; the
[generated report](../.research/layer-ladder/RESULTS.md) and
[`results.json`](../.research/layer-ladder/results.json) carry every exact value
and the denominator of each one. Percentages below are rounded to whole points.
No model was called, no MCP host was started, and no production API was touched.

Evidence labels are kept distinct throughout:

| Label | What it means here |
| --- | --- |
| **Measurement** | A checked-in fixture, script, and recorded count from this slice. |
| **Arithmetic** | A calculation over measured values under stated assumptions; not a runtime observation. |
| **Implementation fact** | Current source in a named public repository establishes the behavior. |
| **Unknown** | This slice has no aligned artifact that could support the claim. |

**Measurement.** The six rungs, over identical rows, at four scales:

| Rung | What it is | 5 rows | 20 rows | 100 rows | 500 rows |
| --- | --- | ---: | ---: | ---: | ---: |
| L0 | pretty JSON, all eight fields | 485 | 1,957 | 9,836 | 48,933 |
| L1 | minified JSON, all eight fields | 346 | 1,398 | 7,037 | 34,934 |
| L2 | pretty JSON, four fields | 186 | 742 | 3,688 | 18,433 |
| L3 | minified JSON, four fields | 107 | 423 | 2,089 | 10,434 |
| L4 | header plus value rows, four fields | 65 | 239 | 1,154 | 5,748 |
| L5 | L4 with one-character header aliases | 63 | 237 | 1,152 | 5,746 |

L1, L3, and L4 are production output: `QueryJSONWithMode` in `HumanReadable` and
`LLMReadable` mode. L0 and L2 are pretty-printed baselines built for the
comparison. L5 is a research overlay with no production implementation.

## Layer one: let the caller name the fields

The caller already knows which fields it needs. A query layer that accepts that
selection returns exactly those fields. This is the idea GraphQL made familiar,
reduced to a single line a shell can pass:

~~~text
list() { id status assignee priority }
~~~

**Situation:** the agent wants the sprint board with just enough context to run
the roll-call. **Expected behavior:** the response carries the four named fields
for every record and omits the rest.

~~~json
[{"assignee":"heidi","id":"TASK-0001","priority":"critical","status":"in-progress"},{"assignee":"rosa","id":"TASK-0002","priority":"critical","status":"open"}]
~~~

**Measurement.** Holding the serialization constant and changing only the field
set, the same rows cost:

| Projection step | Denominator | 5 rows | 20 rows | 100 rows | 500 rows |
| --- | --- | ---: | ---: | ---: | ---: |
| L1 to L3, minified | minified full record | 69% | 70% | 70% | 70% |
| L0 to L2, pretty | pretty full record | 62% | 62% | 63% | 62% |

At 100 rows that is 7,037 tokens down to 2,089 on the minified side, and 9,836
down to 3,688 on the pretty side. Exact values: 69.08 to 70.31 percent and 61.65
to 62.51 percent across the four scales.

Selection is the largest single reduction in this measurement, and it is the one
layer whose benefit grows with the data: every description, timestamp, and name
that is never requested is a cost that never appears.

It is also the only layer here that changes what the response contains. The
measurement checks the projected payload as an exact subset of the selected
fields and makes no preservation claim about the four dropped fields, which is
why both rows above name their denominator.

**Stated bound.** This projection drops one long free-text `description` and
keeps four short fields. A projection that kept the description and dropped four
short fields would save far less. These are percentages for this scenario, not a
rate to quote anywhere else.

## The denominator decides the credit

Two orderings reach the same payload. Minify first, then project; or project
first, then minify. The cumulative saving is identical and the attribution is
not:

| Ordering | First step | Second step | Cumulative |
| --- | ---: | ---: | ---: |
| minify, then project | L0 to L1: 28% | L1 to L3: 70% | 79% |
| project, then minify | L0 to L2: 63% | L2 to L3: 43% | 79% |

**Arithmetic over measured values, at 100 rows.** Projection is worth 63 percent
or 70 percent depending purely on what happened before it. Neither number is
wrong. A number quoted without its denominator is.

This is worth stating plainly because the industry folklore around agent output
compression lives here. The familiar "70 to 90 percent" is close to two real
quantities in this scenario and exactly equal to neither: projection off a
minified full record is 69 to 70 percent, and the whole stack from pretty full
JSON down to projected compact output is 87 to 88 percent. Both are specific to
this scenario's four-of-eight projection, this fixture set, and this tokenizer.

## Layer two: say each field name once

Selection removes fields. The remaining cost is how the surviving fields are
written. In minified JSON each of the four field names is repeated once per
record, so 100 records carry 400 key strings for four distinct names. A header
plus value rows says each name once:

~~~text
id,status,assignee,priority
TASK-0001,in-progress,heidi,critical
TASK-0002,open,rosa,critical
TASK-0003,review,ivan,critical
TASK-0004,review,bob,medium
TASK-0005,open,heidi,low
~~~

The rows carry the same four fields as the projected JSON, and the measurement
decodes the compact form back to the fixture records to confirm it.

**Measurement.** With the field set held constant at the scenario's four fields,
L3 to L4:

| Scale | Minified projected JSON | Header plus value rows | Reduction |
| ---: | ---: | ---: | ---: |
| 5 | 107 | 65 | 39% |
| 20 | 423 | 239 | 43% |
| 100 | 2,089 | 1,154 | 45% |
| 500 | 10,434 | 5,748 | 45% |

This is the row the previous revision of this article could not report. Its
fixture had no projected compact payload, so it compared formats at one field
set and projections at one format and refused to combine them. This ladder holds
the field set fixed across L3 and L4, so the comparison is a format change and
nothing else. Exact values: 39.25 to 44.91 percent.

Representation has a limit that selection does not: the saving scales with the
number of field names and the punctuation around them, not with the amount of
data behind them. Wide records with short values gain the most. A response
dominated by long free text gains the least, because the text is the payload
either way. That is visible in the ladder itself: minification of the full
record, which still carries the descriptions, is worth 28 to 29 percent, while
minification of the projected record, where the long field is already gone, is
worth 42 to 43 percent.

**Measurement.** Taking the whole stack together, L0 to L4 — pretty full JSON
down to projected compact output — removes 87 to 88 percent of the tokens. At
100 rows: 9,836 down to 1,154.

## What each move costs once

A per-response saving is not a session saving until it has paid off whatever the
agent had to read **to make that particular move**. The one-time cost of a
transition is what the destination needs and the source did not; an artifact both
rungs need is sunk on both sides and cancels.

Every rung of this ladder is `agentquery` output. An agent that has read
`schema()` once pays nothing extra to write a projected query instead of a full
one, and nothing extra to ask for the compact format instead of JSON.

**Measurement and arithmetic.** Session constants: the real `schema()` response
for this scenario is 535 tokens, the one for the shipped example CLI is 1,154,
and the alias legend is 21.

| Move | Charged | Sunk on both sides | One-time | Break-even |
| --- | --- | --- | ---: | ---: |
| projection, L1 to L3 | nothing | `schema()` | 0 | 1 query |
| compact, L3 to L4 | nothing | `schema()` | 0 | 1 query |
| aliases, L4 to L5 | alias legend | `schema()` | 21 | 11 queries |

Only the alias row has a real incremental cost. The other two are free choices
inside a session that already uses the query layer.

**Correction to the previous revision.** This article previously charged the
whole `schema()` roundtrip against the projection and compact rows, which gave
break-evens of three and thirteen queries and a negative net for compact at five
rows and ten queries. Those figures priced an introspection the agent had
already paid for on the other side of the same transition, and they are
withdrawn. The measurement now derives each row's one-time cost from its
transition, and a gate refuses any row that charges an artifact its source rung
already held.

**Stated bound, and a framing this ladder cannot support.** There is no non-DSL
rung here. L1 is `agentquery` output, not a REST response. So this measurement
cannot price *adoption* of the query layer at all: whether one `schema()`
roundtrip is worth paying against a plain client that needs no contract is a
different comparison against a baseline nothing here measures. What survives is
narrower and still useful — inside a session that already uses the query layer,
projection and compact output cost nothing extra to choose. And `schema()` is
paid only if the agent introspects; documenting the grammar in a skill file moves
that cost rather than deleting it.

## Layer three: the same two decisions over MCP

Neither layer is a property of a private transport. A tool exposed over the
[Model Context Protocol](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)
takes the same selection argument and returns the same representation, because
`fields` is an ordinary tool argument in the input schema. **Projection is a
server design choice, not a protocol capability**, and nothing in this slice says
MCP is unable to project or to emit compact output.

**Stated bound, stated first because it governs everything below.** The contracts
compared here are *constructed* from the real `schema()` output rather than
captured from a running server. No host was started, no SDK was executed, no
transcript was recorded. The results carry this as `evidence_class:
synthetic-contract` and `host_measured: false`. The protocol revision is probed
rather than asserted: on 2026-09-24 `/specification/latest` redirected to
`2026-07-28`, with a control revision that cannot exist returning 404 so a site
outage could not be read as a withdrawn revision.

**Measurement.** The session-constant comparison — what the agent reads before
any call — has no profile-independent answer, so both profiles are published.
`minimal` carries only what the `Tool` interface requires plus what `schema()`
already publishes. `structured` adds the `outputSchema` the current revision's
structured-content surface expects, derived from the same field list.

| Contract | Tools | Profile | MCP `tools/list` | `schema()` | Difference |
| --- | ---: | --- | ---: | ---: | ---: |
| scenario | 3 | minimal | 471 | 535 | -64 |
| scenario | 3 | structured | 676 | 535 | +141 |
| example CLI | 9 | minimal | 1,490 | 1,154 | +336 |
| example CLI | 9 | structured | 1,774 | 1,154 | +620 |

**The direction of this comparison is not stable.** For the scenario contract it
flips between profiles, so neither side wins on tool-definition cost and any
claim in either direction has to name its profile. What is ruled out on every
profile measured here is the folk figure of two to three thousand tokens of dead
weight per session for a tool surface of this size: the largest figure measured
anywhere in this slice is 1,774.

**Arithmetic.** The real asymmetry is *when* the cost is paid, not how large it
is. A host injects `tools/list` unconditionally at session start; `schema()` is
paid only if the agent asks. That is a host-behavior difference, and this
measurement does not quantify it. How a particular host caches tool definitions,
where it places them, and what it charges for them is **unknown** here.

**Measurement.** There is one argument for compact output that exists only inside
MCP. A tool result embeds the payload as a JSON **string**, so every `"` in the
payload is escaped and charged a second time. Measured against the equivalent
call through the query layer, per payload format:

| Payload | `"` at 5 rows | `"` at 500 rows | MCP minus DSL at 5 rows | at 500 rows |
| --- | ---: | ---: | ---: | ---: |
| L4, compact | 0 | 0 | +51 | +51 |
| L3, minified projected JSON | 80 | 8,000 | +59 | +1,049 |
| L1, full minified JSON | 160 | 16,000 | +69 | +2,049 |

The quote-free compact payload pays exactly +51 at every scale: that is the bare
envelope. Everything above it tracks the **quote count, not the payload size**.
The 500-row compact payload is fifty-three times the size of the 5-row JSON
payload and pays less framing excess. A JSON payload pays a surcharge inside MCP
that it does not pay when a command-line tool writes it to standard output.

## Layer four: abbreviating the header, and why it stops there

After selection and representation, one visible target remains. The header line
still spells out four field names. Replacing them with single characters makes
that line shorter:

~~~text
i,s,a,p
TASK-0001,in-progress,heidi,critical
TASK-0002,open,rosa,critical
~~~

**Measurement.** Every value row is byte-identical to the previous section. Only
the header changed.

| Scale | Full-name header | Aliased header | Saved |
| ---: | ---: | ---: | --- |
| 5 | 65 | 63 | 2 tokens (3.1%) |
| 20 | 239 | 237 | 2 tokens (0.8%) |
| 100 | 1,154 | 1,152 | 2 tokens (0.2%) |
| 500 | 5,748 | 5,746 | 2 tokens (0.03%) |

Two tokens, once per response, independent of how many rows follow. The reason is
structural: the previous layer already moved the field names out of the rows, so
the only text left to abbreviate is a single line, and its percentage collapses
as the payload grows. The 2026-02-12 study of this repository measured the same
shape on a wider header: five tokens for eight columns, also constant.

Those two tokens are not free. An agent that meets `s` and `p` in a header has to
learn what they mean, and the legend that teaches it is 21 tokens.

**Arithmetic.** Against that legend, aliases break even at the eleventh query and
never recover enough to matter: ten queries at five rows is a net of -1 token,
and a hundred queries is +179 — against a payload that is already 5,748 tokens at
500 rows.

The failure modes are worse than the arithmetic. A partly evicted legend leaves a
header that still parses and now means something else. Two tools can assign the
same letter to different fields. A schema carrying `s`, `sc`, `sp`, and `st`
forces a lookup on every read. Each of those turns a two-token saving into a
correctness question, which is a poor trade at any session length.

**Decision for this formatter: do not add field aliases.** That was the
2026-02-12 recommendation and this ladder does not move it. The useful result of
this layer is not its balance sheet but the boundary it marks. Once a format
names each field once, abbreviating that name is the last fraction of a percent
of the problem and the first place where an optimization costs more in ambiguity
than it returns in tokens. `agentquery` ships no alias support, and L5 has no
production implementation.

## Where the boundary sits in a real adapter

The layers above describe what the caller sees. They say nothing about what the
backend did, and that distinction decides where the selection belongs.

**Implementation fact.** When the source API accepts a field selection, the
adapter passes it through and the backend genuinely does less work. The Jira
adapter maps selected fields onto Jira REST field names in
[`APIFieldsFromSelector`](https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go#L39-L54),
both the [`get`](https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go#L300)
and [`list`](https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go#L359-L360)
handlers hand that list to the client, and
[`GetIssue`](https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/jira/issues.go#L12-L27)
puts it in the request:

~~~go
q := url.Values{}
if len(fields) > 0 {
    q.Set("fields", strings.Join(fields, ","))
}
~~~

The selection reaches the origin server, and the response is smaller on the wire
before the adapter sees it.

**Implementation fact.** The Confluence adapter shows the other shape, and one
partial exception inside it. Its
[`get` handler](https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/query/schema.go#L247-L256)
derives a single boolean from the projection and passes it down:

~~~go
includeBody := containsField(ctx.Statement.Fields, "body")
page, err := client.GetPage(pageID, includeBody)
~~~

That boolean is the only part of the selection that reaches the API. In
[`pages.go`](https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/confluence/pages.go#L19-L45)
it decides whether the v2 call sends `body-format` at all and whether the v1 call
appends `body.storage` to its otherwise **fixed** expand list
`version,space,ancestors,metadata.labels`. The rest of the projection never
leaves the wrapper. The
[`list` handler](https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/query/schema.go#L275-L303)
has no equivalent at all: it fetches the pages the space returns and applies the
selector afterwards. The agent still receives only the selected fields and the
context saving is real; the backend did nearly the same work it would have done
without the selection.

That difference is the honest boundary of a wrapper. **Field projection at the
agent boundary always saves agent context. It saves backend work only where the
adapter pushes the selection into the upstream request.** Neither shape is a
defect — Confluence's v2 API offers no equivalent field parameter for these calls
— but they are different claims and must not be merged into "projection makes it
faster". [Named presets](https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/query/schema.go#L117-L120)
shorten the common selections without changing that fact.

**Implementation fact.** In this repository, both layers are small.
[`FormatCompact`](../agentquery/format.go#L20) writes the header and then the
rows, [`FieldSelector.Apply`](../agentquery/selector.go#L17) applies the
projection, [`QueryJSONWithMode`](../agentquery/schema.go#L223) selects between
them, and the [parser](../agentquery/parser.go#L318) accepts several statements
separated by semicolons so a batch of lookups is one call:

~~~text
get(TASK-0001) { status }; get(TASK-0002) { status }; get(TASK-0003) { status }
~~~

Whether that batch saves tokens depends on the host's per-call framing, which
this measurement does not observe. That number is **unknown** here.

## Reproducing the measurement

~~~bash
cd .research/layer-ladder
go run ./cmd/genfixtures
/usr/bin/python3 mcp/build_contract.py
/usr/bin/python3 measure.py
go test ./... -count=1
/usr/bin/python3 -m unittest discover -s tests
/usr/bin/python3 mutants/run.py
~~~

| Artifact | Purpose |
| --- | --- |
| [`scenario.go`](../.research/layer-ladder/scenario.go) | The scenario, the fixture loading, and the `agentquery` schema it registers |
| [`ladder.go`](../.research/layer-ladder/ladder.go) | Layer contract, renderers, decoders, and the preservation gates |
| [`cmd/genfixtures`](../.research/layer-ladder/cmd/genfixtures/main.go) | Renders every layer through the real `agentquery` entry points |
| [`measure.py`](../.research/layer-ladder/measure.py) | Tokenizes, validates, computes the ladders and amortization, writes the report |
| [`mcp/build_contract.py`](../.research/layer-ladder/mcp/build_contract.py) | Derives the MCP tool contracts from the real `schema()` output, in both profiles |
| [`mcp/spec-probe.json`](../.research/layer-ladder/mcp/spec-probe.json) | Dated primary-source read of the current protocol revision, with an absent-revision control |
| [`mutants/run.py`](../.research/layer-ladder/mutants/run.py) | Narrows each gate with a known violation and reports survivors |
| [`results.json`](../.research/layer-ladder/results.json) | Raw counts, both ladder orderings, amortization, and the MCP tables |
| [`RESULTS.md`](../.research/layer-ladder/RESULTS.md) | The generated report |
| [`260924_layer-ladder-measurement.md`](../.research/260924_layer-ladder-measurement.md) | The recorded note with scenario, corrections, and bounds |

## What this measurement does not show

1. One scenario, one field-size distribution, one tokenizer. The projection drops
   one long free-text field and keeps four short ones; a projection that did the
   reverse would save far less. Read every percentage as "in this scenario".
2. No REST server, no HTTP transfer, no latency, no backend cost. Everything here
   is a serialized payload's token count.
3. No host was measured, for MCP or for the query layer. Host framing, tool-use
   scaffolding, and system-prompt text are all uncounted.
4. The MCP contract is constructed from real `schema()` output, not captured from
   a running server. The `minimal` profile is a floor no real current-revision
   server would publish, and `structured` is itself a lower bound on a fully
   populated contract.
5. The revision pin is fresh as of the dated probe, not for all time.
6. Filtering is deliberately outside the ladder: it changes the row set, and the
   ladder's premise is that the row set does not change.
7. L5 aliases have no production implementation.

## Conclusion

Put the selection in the source API when you own it. A field argument that
reaches the origin query is the only version of this optimization that makes the
backend do less work, and it is also the version that scales with the data rather
than with the schema. Make the compact representation the default for responses
an agent reads: at a fixed field set it removes 39 to 45 percent of what the
projected JSON cost, and inside a session that already uses the query layer it
costs nothing extra to choose. Offer both over whichever transport your callers
need, MCP included, since neither layer is a property of the transport.

Quote every percentage with its denominator. The same projection in this
scenario is worth 63 percent or 70 percent depending only on whether the baseline
was already minified, and a figure that travels without its denominator will be
read against the wrong one.

When the API is somebody else's, or while a native selection is still ahead of
you, a wrapper is the right answer with one honest caveat: it protects the
model's context and it promises nothing about the load on the origin server. Say
which of the two you are delivering.

And stop before the header. Two tokens, constant at every scale, against a
21-token legend an agent has to hold and a class of decoding failures that a
correct format does not have: that is what an optimization looks like when it has
run out of structure to remove. When the remaining target is a single line, the
work left to do is somewhere else.

## References

1. [Layer-ladder measurement note](../.research/260924_layer-ladder-measurement.md), repository research note, September 2026.
2. [Generated layer-ladder report](../.research/layer-ladder/RESULTS.md), repository measurement artifact, September 2026.
3. [MCP Tools specification, revision 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28/server/tools), official specification.
4. [Jira Management query schema](https://github.com/relux-works/skill-jira-management/blob/5c9b31fddb0495d2eaee32564d153fc6955a35b2/internal/query/schema.go), public adapter source.
5. [Confluence Management page client](https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/confluence/pages.go), public adapter source.
6. [Confluence Management query schema](https://github.com/relux-works/skill-confluence-management/blob/10e342b1c60e52233cc4ef662e70e6b6dcc8011b/internal/query/schema.go), public adapter source.
