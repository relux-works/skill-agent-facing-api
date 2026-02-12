# Field Name Aliases in Schema-Once Output: Do They Save Tokens?

**A three-part empirical study on the marginal value of field name abbreviation when compact tabular format already eliminates key repetition.**

*February 2026*

---

## Abstract

We investigated whether registering short aliases for field names (e.g., `status` -> `s`, `assignee` -> `a`) would meaningfully reduce token consumption in agent-facing CLI output that already uses a schema-once compact format. Three independent studies measured: (1) raw token savings from abbreviation across payload scales, (2) LLM comprehension impact at 5, 15, and 30 alias complexity levels, and (3) session-level token economics including schema discovery roundtrip overhead.

**Finding: aliases are not worth implementing.** In compact tabular (schema-once) format, field names appear exactly once in the header row. Abbreviating the header saves a fixed 5 tokens regardless of whether the payload contains 5 or 500 items — a 0.02% to 1.86% marginal reduction. Meanwhile, the alias dictionary requires a `schema()` introspection roundtrip costing 85 tokens per call, producing a net token loss in 75% of simulated session scenarios. The compact format already solves the problem aliases target (repeated key names), making aliases architecturally redundant.

---

## 1. Introduction

### The Optimization Landscape

When AI agents consume structured data from CLI tools, token efficiency directly impacts cost and context window utilization. A well-optimized agent-facing query layer implements several techniques to minimize output tokens:

1. **Field projection** — return only requested fields (~variable savings)
2. **Format switch** — compact tabular output instead of JSON (~46% savings)
3. **Batching** — multiple queries per tool call (~80 tokens saved per avoided call)
4. **Presets** — named field bundles reduce query input tokens

After implementing these four optimizations, a natural question arises: can we squeeze out more tokens by abbreviating field names themselves?

### The Hypothesis

Field name aliases (`id` -> `i`, `name` -> `n`, `status` -> `s`) should reduce output tokens because shorter strings produce fewer tokens. The alias mapping would be stored in the schema and exposed via a `schema()` introspection call. The agent learns the dictionary once, then reads abbreviated output for the remainder of the session.

### The Concern

Three potential costs could negate the savings:

1. **Token savings might be trivial** if field names are already short or appear infrequently in the output format
2. **LLM comprehension might degrade** when reading cryptic abbreviated headers
3. **Schema roundtrip overhead** (the cost of calling `schema()` to learn the alias dictionary) might exceed the per-query savings, especially if context compression forces repeated re-learning

We designed three independent studies to test each concern.

---

## 2. Study 1: Token Savings Measurement

### Methodology

Generated synthetic task tracker payloads at four scales (5, 20, 100, 500 items) with 8 fields per item: `id`, `name`, `status`, `assignee`, `description`, `priority`, `created`, `updated`.

Three format variants per scale:

- **JSON** — standard pretty-printed JSON array with indentation
- **Compact-full** — CSV-style with full field names as header, comma-separated data rows below
- **Compact-alias** — same CSV-style with 1-character abbreviated header (`i,n,s,a,d,p,c,u`)

Token counts measured with `tiktoken` using `cl100k_base` encoding (compatible with GPT-4 and Claude tokenizers). Random seed fixed at 42 for reproducibility. Payloads used realistic data: task names, assignee names, ISO date strings, multi-word descriptions.

### Results

#### Raw Token Counts

| Items | JSON | Compact-Full | Compact-Alias |
|------:|-----:|-------------:|--------------:|
| 5 | 485 | 269 | 264 |
| 20 | 1,957 | 1,055 | 1,050 |
| 100 | 9,836 | 5,283 | 5,278 |
| 500 | 48,933 | 26,144 | 26,139 |

#### Savings Breakdown

| Transition | 5 items | 20 items | 100 items | 500 items |
|-----------|--------:|--------:|---------:|---------:|
| JSON -> Compact-Full | -44.5% | -46.1% | -46.3% | -46.6% |
| Compact-Full -> Compact-Alias | -1.86% | -0.47% | -0.09% | -0.02% |
| **Absolute alias savings** | **5 tok** | **5 tok** | **5 tok** | **5 tok** |

#### The Structural Explanation

The alias savings are constant at 5 tokens because in CSV-style output, field names appear **exactly once** — in the header row. The full header:

```
id,name,status,assignee,description,priority,created,updated
```

is 14 tokens. The abbreviated header:

```
i,n,s,a,d,p,c,u
```

is 9 tokens. The difference (5 tokens) is fixed regardless of how many data rows follow. All data rows are identical in both variants — they contain values, not keys.

This is the fundamental structural property of schema-once formats: field names are declared once, then never repeated. Abbreviating a one-time declaration yields a one-time saving. As payload size grows, the marginal benefit of that saving approaches zero.

#### Per-Item Amortization

| Items | JSON tok/item | Compact tok/item | Alias tok/item |
|------:|--------------:|-----------------:|---------------:|
| 5 | 97.0 | 53.8 | 52.8 |
| 20 | 97.8 | 52.8 | 52.5 |
| 100 | 98.4 | 52.8 | 52.8 |
| 500 | 97.9 | 52.3 | 52.3 |

JSON costs ~98 tokens per item (keys repeat per object). Compact costs ~53 tokens per item (keys declared once). Aliases save an additional 1 token per item at 5 items, converging to 0 per item at scale.

---

## 3. Study 2: LLM Comprehension Impact

### Methodology

Designed a three-level comprehension benchmark:

| Level | Fields | Alias Style | Collision Risk |
|-------|-------:|-------------|----------------|
| 1 | 5 | Single-char (`i,n,s,a,d`) | Low |
| 2 | 15 | 1-2 char (`i,n,s,a,d,p,t,cr,up,dl,e,tg,bl,cm,pr`) | Medium |
| 3 | 30 | 1-2 char with deliberate near-collisions (`s,sc,sp,st,sr` / `c,cl,cr,cm`) | High |

Each level includes 12 data items and 10 questions spanning five types: direct lookup, filtering, cross-reference, aggregation, and multi-field reasoning. Each level was tested in two conditions:

- **Abbreviated (A):** explicit alias dictionary provided, data uses abbreviated headers
- **Full (F):** identical data with full field names, no dictionary needed

Evaluated by Claude Opus 4.6 with pre-computed ground truth verified independently by positional field counting. Self-evaluation caveat: both tests and answers were produced in the same pipeline; the meaningful signal is the *delta* between conditions, not absolute accuracy.

### Results

| Level | Fields | Abbreviated | Full | Delta |
|-------|-------:|:-----------:|:----:|:-----:|
| 1 | 5 | 10/10 (100%) | 10/10 (100%) | 0% |
| 2 | 15 | 10/10 (100%) | 10/10 (100%) | 0% |
| 3 | 30 | 10/10 (100%) | 10/10 (100%) | 0% |

**Zero accuracy degradation across all levels** when an explicit dictionary is provided.

### Qualitative Friction

Despite perfect accuracy, aliases introduced measurable cognitive overhead at Level 3. Collision clusters required repeated dictionary lookups:

- **The "s-family":** `s`=status, `sc`=scope, `sp`=sprint, `st`=story-points, `sr`=source
- **The "c-family":** `c`=category, `cl`=closed, `cm`=comments, `cr`=created
- **The "r-family":** `r`=reporter, `rv`=reviewer, `rn`=rank

**Processing path comparison** (Level 3, Q2: "What sprint and scope does T-209 belong to?"):

| Step | Full Names | Abbreviated |
|------|-----------|-------------|
| 1 | Find T-209 row | Lookup: sp=sprint, sc=scope (not s, not st) |
| 2 | Read "sprint" column -> S-15 | Find T-209 row |
| 3 | Read "scope" column -> backend | Locate `sp` column (16th) -> S-15 |
| 4 | — | Locate `sc` column (15th) -> backend |

The abbreviated path adds one dictionary lookup per aliased field. Trivial for a single question. Over thousands of agent queries, this compounds.

### Failure Modes

Five failure modes were identified for production deployment:

1. **No dictionary provided** — catastrophic. `c` could mean category, comments, created, or closed. The Columbo study (EMNLP 2025) measured 10.54% NL2SQL accuracy drop and 40.5% relation detection drop with undocumented abbreviations.

2. **Domain-specific ambiguity** — `s` defaults to "status" in developer tools but could mean "story", "sprint", "severity" in other contexts. Domain priors may override dictionary lookup.

3. **Multi-schema contexts** — agent works with multiple tools using different alias dictionaries. `p`=priority in Tool A, `p`=parent in Tool B. Cross-contamination is likely.

4. **Partial context eviction** — the worst failure mode. After context compression, the agent remembers some aliases but not others, silently applying incorrect mappings.

5. **Aggregation at scale** — counting or filtering over 100+ rows while holding alias mappings strains attention, though this is a general LLM limitation orthogonal to aliasing.

---

## 4. Study 3: Session-Level Token Economics

### Measured Constants

All token counts measured with `tiktoken` cl100k_base encoding on real CLI output from an agentquery-based task tracker:

| Metric | Tokens | Method |
|--------|-------:|--------|
| Schema() roundtrip cost | 85 | 10 (call) + 71 (response) + 4 (overhead) |
| Alias savings per query (compact) | 5 | Fixed: header abbreviation only |
| Alias savings per query (JSON get) | 3 | Abbreviated object keys |
| Alias savings per query (JSON list, 10 items) | 30 | 3 tokens/item x 10 |
| Typical get() response (compact) | 23 | Real CLI measurement |
| Typical list() response, 8 items (compact) | 177 | Real CLI measurement |

### Session Simulation

Modeled agent sessions of varying length with context eviction — the dictionary is forgotten every K turns, requiring a `schema()` re-query:

| Session | Eviction K | Schema Calls | Schema Cost | Alias Savings | **Net** |
|--------:|-----------:|-------------:|------------:|--------------:|--------:|
| 10 | 10 | 1 | 85 | 40 | **-45** |
| 10 | never | 1 | 85 | 40 | **-45** |
| 20 | 10 | 2 | 170 | 80 | **-90** |
| 20 | 20 | 1 | 85 | 80 | **-5** |
| 50 | 10 | 5 | 425 | 200 | **-225** |
| 50 | 50 | 1 | 85 | 200 | **+115** |
| 100 | 10 | 10 | 850 | 400 | **-450** |
| 100 | 20 | 5 | 425 | 400 | **-25** |
| 100 | 50 | 2 | 170 | 400 | **+230** |
| 100 | never | 1 | 85 | 400 | **+315** |

**Net positive in only 4 of 16 scenarios (25%).** All require either long sessions (50+ queries) with infrequent eviction (50+ turns), or the unrealistic assumption that the dictionary is never evicted from context.

### Break-Even Analysis

```
queries_between_evictions > schema_cost / savings_per_query
```

| Format | Savings/Query | Schema Cost | Break-Even |
|--------|-------------:|------------:|-----------:|
| Compact | 5 | 85 | **17 queries** |
| JSON (get) | 3 | 85 | **29 queries** |
| JSON (list, 10 items) | 30 | 85 | **3 queries** |

For compact format, the agent needs 17 consecutive data queries between schema refreshes — unreliable given typical context eviction patterns (every 10-20 turns in production agent systems after ~180K context tokens).

### Workflow Analysis

Four representative agent workflows modeled end-to-end:

| Workflow | Without Aliases | With Aliases | Delta |
|----------|---------------:|-------------:|------:|
| Check status of 5 tasks | 246 tok | 306 tok | **+24% worse** |
| Find blocked tasks + update | 486 tok | 541 tok | **+11% worse** |
| Daily standup review | 130 tok | 196 tok | **+51% worse** |
| Heavy analytics (100q, evict/20) | ~6,000 tok | ~6,025 tok | **-0.4% marginal loss** |

Every workflow shows aliases performing equal to or worse than the no-alias baseline.

**Batching comparison:** For the "check status of 5 tasks" workflow, batching 5 queries into a single call costs 165 tokens — 33% less than individual queries (246 tok) and 46% less than aliases (306 tok). Batching is the superior optimization with zero overhead.

### The JSON Alias Irony

Aliases in JSON format *are* economical — saving ~3 tokens per item per query, easily exceeding the 85-token schema cost in moderate sessions. But this finding is irrelevant: the recommendation is to use compact format, which already saves ~46% vs JSON. Compact format eliminates the key repetition that makes JSON aliases valuable. **The two optimizations are substitutes, not complements.** Implementing aliases to optimize JSON output is like tuning a carburetor after installing fuel injection.

---

## 5. Discussion

### The Core Insight: Schema-Once Kills Aliases

The three studies converge on a single structural insight:

> **In any schema-once output format (CSV, TSV, TOON), field names appear exactly once. Abbreviating a one-time occurrence produces a one-time saving. As payload size grows, the marginal value of that saving approaches zero.**

This is not a limitation of a specific implementation — it's an inherent property of the format. Any compact tabular format with a header row followed by value rows exhibits this behavior. Aliases are a solution to key repetition, and schema-once formats have no key repetition left to solve.

### The Optimization Hierarchy

| Rank | Optimization | Savings | Discovery Overhead |
|------|-------------|---------|-------------------|
| 1 | Field projection | 50-80% (selective queries) | Zero |
| 2 | Compact format (JSON -> tabular) | ~46% | Zero |
| 3 | Batching | ~80 tok per avoided call | Zero |
| 4 | Presets | ~5-10 tok per query input | Zero |
| **5** | **Field aliases** | **5 tokens (fixed)** | **85 tok per schema() call** |

Optimizations 1-4 work from the first query with zero discovery overhead. Aliases are the only optimization requiring upfront investment (schema roundtrip), and the investment exceeds the return in most scenarios.

### BPE Tokenizers Already Compress Field Names

A second-order insight: modern BPE tokenizers already perform the compression that aliases attempt. Common field names — "status", "name", "id", "type" — are single tokens in cl100k_base. Abbreviating `status` (1 token) to `s` (1 token) saves zero tokens. Only multi-token field names benefit: `blocked_by` (3 tokens) -> `bb` (1 token) saves 2 tokens. But these savings only manifest in JSON (where keys repeat per item), not in compact format (where the header appears once).

### Format as Transport Concern

This study reinforces a broader design principle: **output format is a transport concern, not a domain concern.** The data source should never decide serialization format. The caller declares format through explicit transport-level mechanisms:

| Layer | Format Declaration |
|-------|-------------------|
| CLI | `--format compact` flag (required, no default) |
| SDK | `QueryJSONWithMode(query, LLMReadable)` per-call parameter |
| REST | `Accept: application/json` header |
| gRPC | `output_format` request field |

If aliases were implemented, they would follow the same principle — a `--aliases` flag or per-call parameter, never a schema-level configuration. But even at the transport level, the economics don't justify the feature.

### Comparison with External Research

**Columbo (EMNLP 2025)** found devastating accuracy drops (10.54% NL2SQL, 40.5% relation detection) from abbreviated column names *without* dictionaries. Our benchmark showed 0% degradation *with* dictionaries. The dictionary is essential infrastructure, but requiring a dictionary means requiring a schema roundtrip — which is the overhead that kills the economics.

**TOON format** uses full field names in its schema headers despite being designed explicitly for LLM token efficiency. TOON achieves 73.9% accuracy (vs 69.7% for JSON) with 39.6% fewer tokens through structural clarity, not name abbreviation. This validates the approach: **structure matters more than name length.**

**Better Think with Tables (2024)** showed 40.29% performance gain from tabular format vs text for data analytics — driven by structural clarity (delimiters, alignment), not header verbosity.

---

## 6. Conclusion

Field name aliases in schema-once output are a solution to a problem that no longer exists. The compact tabular format — by declaring field names once in a header row — already eliminates the per-item key repetition that aliases target. The marginal savings (5 tokens, fixed) are dwarfed by the schema discovery overhead (85 tokens per roundtrip), producing a net token loss in 75% of simulated scenarios.

The token optimization hierarchy for agent-facing output is clear: field projection and compact format deliver the largest gains (46%+) with zero overhead. Batching and presets provide additional savings. Aliases sit at the bottom — the only optimization where discovery cost exceeds the benefit in typical usage.

### Decision Record

| Criterion | Threshold | Measured | Result |
|-----------|-----------|----------|--------|
| Net token savings | > 10% | 0.02-1.86% (5 tokens fixed) | FAIL |
| Comprehension degradation | < 5% | 0% (with dictionary) | PASS |
| Schema roundtrip ratio | < 1:5 | Break-even at 1:17 | FAIL |
| **Overall** | All pass | **2 of 3 failed** | **NO-GO** |

### What to Optimize Instead

1. **Compact format adoption** — the 46% savings is the single biggest win available
2. **Batch query utilization** — agents underutilize multi-query syntax, leaving ~80 tokens per call on the table
3. **Preset tuning** — matching named field bundles to common agent workflows
4. **Value-level compression** — shortening date formats, status codes (warranting separate measurement)

---

## References

1. Columbo: Expanding Abbreviated Column Names for Tabular Data. EMNLP 2025 Findings. [arxiv.org/html/2508.09403](https://arxiv.org/html/2508.09403)

2. TOON: Token-Oriented Object Notation. [github.com/toon-format/toon](https://github.com/toon-format/toon)

3. Better Think with Tables: Tabular Structures Enhance LLM Comprehension. 2024. [arxiv.org/html/2412.17189v3](https://arxiv.org/html/2412.17189v3)

4. LLMLingua: Compressing Prompts for Accelerated Inference. 2023. [arxiv.org/html/2310.05736v2](https://arxiv.org/html/2310.05736v2)

5. TOON vs JSON — The New Format Designed for AI. dev.to, 2025. [dev.to/akki907/toon-vs-json-the-new-format-designed-for-ai-nk5](https://dev.to/akki907/toon-vs-json-the-new-format-designed-for-ai-nk5)

---

## Appendix: Reproducibility

All experimental artifacts are available in the repository:

| Artifact | Description |
|----------|-------------|
| `generate.py` | Synthetic payload generator (4 scales x 3 variants, seeded) |
| `measure.py` | Token measurement script using tiktoken cl100k_base |
| Comprehension tests | 3 levels x 12 items x 10 questions with ground truth |
| `simulate.py` | Session simulator with configurable eviction rates |

To reproduce the token measurements:

```bash
pip3 install tiktoken
python3 generate.py    # generates 12 payload files
python3 measure.py     # tokenizes, computes savings, writes report
```

To run the session simulator:

```bash
python3 simulate.py    # models 16 session scenarios, outputs results
```
