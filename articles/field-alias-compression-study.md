# Field Name Aliases in Schema-Once Output: Do They Save Tokens?

**A compact formatter has already paid to say a field name once. Is an alias dictionary worth another schema lookup to save a few tokens?**

*February 2026; evidence boundaries revised September 2026.*

---

## The question and its boundary

A task-list formatter can return a header once and values beneath it:

~~~text
id,name,status
T-209,Schema migration,development
T-210,Release notes,to-review
~~~

This is the familiar baseline: field names identify the columns once, while the
rows carry only values. Replacing the header with aliases such as
<code>i,n,s</code> can make that one line shorter, but it also requires an
agent to learn and retain the dictionary.

This article asks a deliberately bounded question: **for this repository's
schema-once compact formatter, do field aliases justify their discovery and
comprehension cost?** It covers the checked-in alias fixtures, the associated
session model, and the current Go implementation of compact output and DSL
batching. It does not establish a token winner between the DSL and MCP, and it
does not generalize the results to arbitrary tokenizers, hosts, or transports.

The evidence types are kept distinct throughout:

| Label | What it means here |
| --- | --- |
| **Verified implementation fact** | Current source and executable tests establish a behavior of this Go implementation. |
| **Checked-in historical measurement** | A fixture, script, and recorded result exist, but this revision did not rerun the tokenizer. |
| **Model estimate** | A simulator result depends on stated constants and eviction assumptions; it is not a runtime observation. |
| **Reasoning** | A conditional design recommendation rather than a numeric result. |
| **Unknown** | The repository has no aligned artifact that could support the claim. |

The detailed classification and MCP claim dispositions are in the accepted
[MCP token-economics evidence map](../.research/260923_mcp-token-economics-evidence.md).

## Study 1: What one abbreviated header saves

**Evidence: checked-in historical measurement.** The fixtures use task-tracker
payloads at 5, 20, 100, and 500 items, each with eight fields:
<code>id</code>, <code>name</code>, <code>status</code>,
<code>assignee</code>, <code>description</code>,
<code>priority</code>, <code>created</code>, and <code>updated</code>.
The checked-in [measurement script](../.research/synthetic-payloads/measure.py)
tokenizes fixed JSON, compact-full, and compact-alias fixtures with
<code>tiktoken</code> and <code>cl100k_base</code>. These are historical fixture
results, not tokenizer counts re-attested by this revision.

| Items | JSON | Compact-full | Compact-alias |
| ---: | ---: | ---: | ---: |
| 5 | 485 | 269 | 264 |
| 20 | 1,957 | 1,055 | 1,050 |
| 100 | 9,836 | 5,283 | 5,278 |
| 500 | 48,933 | 26,144 | 26,139 |

The same recorded data makes the contrast explicit:

| Transition | 5 items | 20 items | 100 items | 500 items |
| --- | ---: | ---: | ---: | ---: |
| JSON to compact-full | -44.5% | -46.1% | -46.3% | -46.6% |
| Compact-full to compact-alias | -1.86% | -0.47% | -0.09% | -0.02% |
| **Absolute alias saving** | **5 tok** | **5 tok** | **5 tok** | **5 tok** |

The concrete example explains the fixed result. The full header

~~~text
id,name,status,assignee,description,priority,created,updated
~~~

is emitted once; the abbreviated version

~~~text
i,n,s,a,d,p,c,u
~~~

changes only that declaration. The value rows do not repeat either set of field
names. The historical fixture counts therefore show a 5-token header delta at
every tested payload size; they do not prove the same percentage or token count
for another tokenizer or formatter.

The per-item values below are another view of those same historical fixtures,
not a new measurement:

| Items | JSON tok/item | Compact-full tok/item | Compact-alias tok/item |
| ---: | ---: | ---: | ---: |
| 5 | 97.0 | 53.8 | 52.8 |
| 20 | 97.8 | 52.8 | 52.5 |
| 100 | 98.4 | 52.8 | 52.8 |
| 500 | 97.9 | 52.3 | 52.3 |

## Study 2: What a remembered dictionary costs

**Evidence: checked-in historical benchmark record.** The comprehension
materials under [.research/comprehension-tests](../.research/comprehension-tests/)
compare full names with aliases at three levels: 5, 15, and 30 fields. Each
level has 12 data items and 10 questions spanning lookup, filtering,
cross-reference, aggregation, and multi-field reasoning. The earlier benchmark
reported the following result when an explicit dictionary was supplied:

| Level | Fields | Abbreviated | Full | Delta |
| --- | ---: | :---: | :---: | :---: |
| 1 | 5 | 10/10 (100%) | 10/10 (100%) | 0% |
| 2 | 15 | 10/10 (100%) | 10/10 (100%) | 0% |
| 3 | 30 | 10/10 (100%) | 10/10 (100%) | 0% |

This result is bounded. The tests and answers were produced in the same
pipeline, so the useful signal is the reported delta, not a general claim about
model accuracy. The benchmark was not rerun for this revision.

The growing task-list example also exposes the operational risk. Once a header
contains aliases such as <code>s</code>, <code>sc</code>,
<code>sp</code>, and <code>st</code>, an agent answering a question about
T-209 must first recover the dictionary, then find the row and its columns.
The source materials identify five failure modes:

1. No dictionary leaves an alias such as <code>c</code> ambiguous.
2. Domain priors can conflict with a tool's chosen alias.
3. Different tools can reuse the same alias for different fields.
4. Partial context eviction can leave an incomplete dictionary.
5. Large aggregations add attention pressure even when the dictionary is present.

An explicit dictionary prevents the first failure in the historical benchmark,
but it creates the discovery and retention requirement evaluated next.

## Study 3: The alias session model

**Evidence: model estimate, not a runtime measurement.** The
[session simulator](../.research/session-simulator/simulate.py) models an
agent that refreshes its alias dictionary after every <em>K</em> turns. It
hard-codes the historical 85-token schema roundtrip and 5-token compact-header
saving, plus a query mix and eviction schedule. Those inputs are assumptions of
the model; they are not observations of Codex, Claude, MCP, or another agent
host.

| Model input | Value used by the simulator | Evidence boundary |
| --- | ---: | --- |
| Schema roundtrip | 85 tok | Historical measurement reused as a model input |
| Compact alias saving per query | 5 tok | Historical fixture result reused as a model input |
| Context eviction and query mix | Scenario-specific | Assumption |

With those inputs, the simulator produces these estimates:

| Session queries | Eviction K | Schema calls | Schema cost | Alias savings | Net |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 10 | 10 | 1 | 85 | 40 | -45 |
| 10 | never | 1 | 85 | 40 | -45 |
| 20 | 10 | 2 | 170 | 80 | -90 |
| 20 | 20 | 1 | 85 | 80 | -5 |
| 50 | 10 | 5 | 425 | 200 | -225 |
| 50 | 50 | 1 | 85 | 200 | +115 |
| 100 | 10 | 10 | 850 | 400 | -450 |
| 100 | 20 | 5 | 425 | 400 | -25 |
| 100 | 50 | 2 | 170 | 400 | +230 |
| 100 | never | 1 | 85 | 400 | +315 |

For its compact-format cases, the simulator evaluates 16 cases and reports four
positive outcomes. For example, its compact scenario for 20 queries with K=20
returns -5, while 50 queries with K=50 returns +115. That result supports only
this conditional statement: aliases can be positive when the model's discovery
cost is amortized over sufficiently many remembered queries. It does not measure
a typical agent session or a transport comparison.

The simple all-data-query amortization check for the model's 85-token and
5-token inputs is:

~~~text
break_even_queries >= schema_cost / savings_per_query
net_positive_queries > schema_cost / savings_per_query
~~~

At those inputs, 17 such data queries exactly amortize one schema roundtrip;
net-positive savings begin at 18. These are derived model values, not
host-independent thresholds.

### Batching is a separate implementation capability

The alias model should not be turned into a claim that one transport universally
beats another. Still, batching changes the local execution shape in a way the
current implementation can verify.

**Situation:** an agent needs the status of T-209, T-210, and T-211.
**Expected behavior:** this DSL accepts semicolon-separated statements and
returns the results in source order.

~~~text
get(T-209) { status }; get(T-210) { status }; get(T-211) { status }
~~~

The parser and executor implement this multi-statement behavior, and the test
suite covers three batched statements and compact rendering. This shows a
capability of this Go DSL. It does not supply a per-call token total, prove a
particular host framing cost, or establish anything about an MCP server's
batching behavior.

## Discussion: choose the structural optimization first

The historical fixtures support a narrow structural insight: a schema-once
header removes repeated field names from the rows. Aliases can only shorten
that one declaration. The simulator then models whether learning the aliases
pays for that fixed saving under its stated assumptions.

| Priority | Optimization | Evidence in this repository | Boundary |
| ---: | --- | --- | --- |
| 1 | Field projection | Implementation selects requested fields | No percentage is claimed here |
| 2 | Compact schema-once output | Historical fixtures report 44.5% to 46.6% versus JSON | Fixture and tokenizer specific |
| 3 | Semicolon batching | Verified implementation behavior | External call savings depend on host and transport |
| 4 | Presets | Implementation feature for common selections | No token saving is asserted here |
| 5 | Field aliases | Historical fixtures show a fixed 5-token compact-header delta | Dictionary discovery is modeled, not newly measured |

### MCP comparison: scoped conclusion

The accepted evidence map separates implementation facts from transport
economics:

| Question | What the evidence supports | What remains unknown |
| --- | --- | --- |
| Can this DSL batch requests? | Yes. The [parser](../agentquery/parser.go#L318-L369) accepts semicolon-separated statements and [QueryAST](../agentquery/query.go#L30-L60) executes a batch. | The token saving for a particular host or transport. |
| Is compact output schema-once? | Yes. [FormatCompact](../agentquery/format.go#L9-L62) writes a field header before its rows. | A universal percentage across tokenizers and payloads. |
| Is MCP more or less token-efficient here? | No repository-local result answers this. | There is no MCP adapter, tool-definition fixture, prompt snapshot, host trace, or aligned workload. |

MCP batching and discovery costs must be bounded by protocol version, SDK,
server design, and host behavior. The official
[Ruby SDK protocol-version reference](https://ruby.sdk.modelcontextprotocol.io/protocol-versions/)
records that protocol version 2025-06-18 removed JSON-RPC batching, while the
official [TypeScript SDK request-body reference](https://ts.sdk.modelcontextprotocol.io/v2/api/@modelcontextprotocol/server/server/requestBody.html)
documents a current maximum of 100 messages in a JSON-RPC batch array. Neither
source guarantees that an agent host exposes or sends a batch, and neither
measures prompt-token cost for this repository.

**Reasoning: MCP can still be the better interface when interoperability is the
requirement rather than a proven local token minimum.** A remote service that
must expose model-controlled tools to several MCP-capable hosts, or an existing
MCP deployment that already meets the integration requirement, can justify MCP.
The [MCP server overview](https://modelcontextprotocol.io/specification/draft/server/index)
supports that tool-interoperability rationale. It is not evidence of a token
win. The failure mode is treating any of those integration benefits as a
measured break-even point; a real comparison needs a server/host pair, protocol
and SDK version, discovery transcript, tokenizer, and aligned workload.

## Reproducing and extending the evidence

The checked-in artifacts make the historical alias result inspectable:

| Artifact | Purpose |
| --- | --- |
| [Synthetic payload generator](../.research/synthetic-payloads/generate.py) | Generates the four fixed payload scales and three formats |
| [Tokenizer measurement script](../.research/synthetic-payloads/measure.py) | Counts the fixtures with <code>tiktoken</code> and <code>cl100k_base</code> |
| [Comprehension materials](../.research/comprehension-tests/) | Holds the three alias/full test levels |
| [Session simulator](../.research/session-simulator/simulate.py) | Evaluates the documented 16 model scenarios |

Run the tokenizer measurement only in an isolated environment where
<code>tiktoken</code> is installed:

~~~bash
python3 .research/synthetic-payloads/generate.py
python3 .research/synthetic-payloads/measure.py
~~~

Run the model separately so its output is not presented as a measurement:

~~~bash
python3 .research/session-simulator/simulate.py
~~~

To make an MCP token claim, add a distinct, reproducible benchmark with the
exact server, host, protocol/SDK version, tool definitions, prompt snapshot,
tokenizer, and workload. Do not reuse the historical 346-element comparison as
if those artifacts were present here.

## Conclusion: the decision for this formatter

**Do not add field aliases to this compact formatter on the current evidence.**
The checked-in fixtures show that aliases are a one-header optimization, and
the session model turns that small fixed gain into a benefit only under its
explicit retention assumptions. For this formatter, aliases are a one-header
optimization; no repository-local MCP benchmark establishes a transport-wide
token winner.

The concrete next action is therefore conditional: prioritize field projection,
compact schema-once output, and the existing DSL batching capability for this
formatter; choose MCP when interoperability requires it; and measure an aligned
host/server workload before making any transport-token claim.

## References

1. [MCP Token-Economics Evidence Map](../.research/260923_mcp-token-economics-evidence.md), repository research note, September 2026.
2. [MCP Ruby SDK protocol versions](https://ruby.sdk.modelcontextprotocol.io/protocol-versions/), official protocol-version reference.
3. [MCP TypeScript SDK request body](https://ts.sdk.modelcontextprotocol.io/v2/api/@modelcontextprotocol/server/server/requestBody.html), official request-body reference.
4. [MCP server overview](https://modelcontextprotocol.io/specification/draft/server/index), official specification.
