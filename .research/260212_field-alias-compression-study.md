# Field Name Aliases in Schema-Once Output: Do They Save Tokens?

**A three-part empirical study on the marginal value of field name abbreviation when compact tabular format already eliminates key repetition.**

Date: 2026-02-12
Context: agentquery library — agent-optimized CLI query layer with DSL, field projection, and dual output modes (JSON / compact tabular)

---

## Abstract

We investigated whether registering short aliases for field names (e.g., `status` -> `s`, `assignee` -> `a`) would meaningfully reduce token consumption in agent-facing CLI output. Three independent studies measured: (1) raw token savings from abbreviation, (2) LLM comprehension impact, and (3) session-level economics including schema discovery overhead.

**Finding: aliases are not worth implementing.** In compact tabular (schema-once) format, field names appear exactly once in the header row. Abbreviating the header saves a fixed 5 tokens regardless of payload size — 0.02% to 1.86% marginal reduction. Meanwhile, the alias dictionary requires a `schema()` roundtrip costing 85 tokens, producing a net token loss in 75% of simulated session scenarios. The compact format already solves the problem aliases target (repeated key names), making them architecturally redundant.

---

## 1. Introduction

### The Optimization Landscape

When AI agents consume structured data from CLI tools, token efficiency directly impacts cost and context window utilization. The established optimization hierarchy for agent-facing output is:

1. **Field projection** — return only requested fields (~variable savings)
2. **Format switch** — compact tabular instead of JSON (~46% savings)
3. **Batching** — multiple queries per tool call (~80 tokens saved per avoided call)
4. **Presets** — named field bundles reduce query input tokens

After implementing these four optimizations, a natural question arises: can we squeeze more tokens by abbreviating field names themselves?

### The Hypothesis

Field name aliases (`id` -> `i`, `name` -> `n`, `status` -> `s`) should reduce output tokens because shorter strings produce fewer tokens. The alias mapping would be stored in the schema and exposed via a `schema()` introspection call. The agent learns the dictionary once, then reads abbreviated output for the remainder of the session.

### The Concern

Three potential costs could negate the savings:
1. **Token savings might be trivial** if field names are already short or appear infrequently
2. **LLM comprehension might degrade** when reading cryptic abbreviated headers
3. **Schema roundtrip overhead** (85 tokens per call) might exceed the per-query savings, especially if context compression forces re-learning

We designed three independent studies to test each concern.

---

## 2. Study 1: Token Savings Measurement

### Methodology

Generated synthetic task tracker payloads at four scales (5, 20, 100, 500 items) with 8 fields per item: `id`, `name`, `status`, `assignee`, `description`, `priority`, `created`, `updated`.

Three format variants per scale:
- **JSON** — standard `json.dumps(items, indent=2)`
- **Compact-full** — CSV-style with full field names as header, data rows below
- **Compact-alias** — same CSV-style with 1-character abbreviated header (`i,n,s,a,d,p,c,u`)

Token counts measured with `tiktoken` using `cl100k_base` encoding (compatible with GPT-4 and Claude tokenizers). Seed fixed at 42 for reproducibility.

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

The alias savings are constant at 5 tokens because in CSV-style output, field names appear **exactly once** — in the header row. The header line:

```
id,name,status,assignee,description,priority,created,updated
```

is 14 tokens. The abbreviated version:

```
i,n,s,a,d,p,c,u
```

is 9 tokens. The difference (5 tokens) is fixed regardless of how many data rows follow. All data rows are identical in both variants — they contain values, not keys.

This is the fundamental structural property of schema-once formats: field names are declared once, then never repeated. Abbreviating a one-time declaration yields a one-time saving.

### Per-Item Amortization

| Items | JSON tok/item | Compact tok/item | Alias tok/item |
|------:|--------------:|-----------------:|---------------:|
| 5 | 97.0 | 53.8 | 52.8 |
| 20 | 97.8 | 52.8 | 52.5 |
| 100 | 98.4 | 52.8 | 52.8 |
| 500 | 97.9 | 52.3 | 52.3 |

JSON costs ~98 tokens per item (keys repeat). Compact costs ~53 tokens per item (keys declared once). Aliases save 1 token per item at 5 items, converging to 0 at scale.

---

## 3. Study 2: LLM Comprehension Impact

### Methodology

Designed a three-level comprehension benchmark to measure whether abbreviated field names degrade LLM accuracy:

| Level | Fields | Alias Style | Collision Risk |
|-------|--------|-------------|----------------|
| 1 | 5 | Single-char (`i,n,s,a,d`) | Low |
| 2 | 15 | 1-2 char (`i,n,s,a,d,p,t,cr,up,dl,e,tg,bl,cm,pr`) | Medium |
| 3 | 30 | 1-2 char with near-collisions (`s,sc,sp,st,sr` / `c,cl,cr,cm`) | High |

Each level includes 12 data items and 10 questions spanning five types: direct lookup, filtering, cross-reference, aggregation, and multi-field reasoning. Each level tested in two conditions:

- **Abbreviated (A):** alias dictionary provided, data uses abbreviated headers
- **Full (F):** identical data with full field names, no dictionary needed

Evaluated by Claude Opus 4.6 with pre-computed ground truth verified independently by positional field counting.

### Results

| Level | Fields | Abbreviated Score | Full Score | Delta |
|-------|-------:|:-----------------:|:----------:|:-----:|
| 1 | 5 | 10/10 (100%) | 10/10 (100%) | 0% |
| 2 | 15 | 10/10 (100%) | 10/10 (100%) | 0% |
| 3 | 30 | 10/10 (100%) | 10/10 (100%) | 0% |

**Zero accuracy degradation across all levels** when an explicit dictionary is provided.

### Qualitative Friction Analysis

Despite perfect accuracy, aliases introduced measurable cognitive overhead at Level 3:

**Collision clusters** required repeated dictionary lookups:
- The "s-family": `s`=status, `sc`=scope, `sp`=sprint, `st`=story-points, `sr`=source
- The "c-family": `c`=category, `cl`=closed, `cm`=comments, `cr`=created
- The "r-family": `r`=reporter, `rv`=reviewer, `rn`=rank

Each question touching a collision cluster added one extra processing step (dictionary lookup) that was unnecessary with full names. Over thousands of queries in a real session, this accumulates.

**Processing path comparison** (Level 3, Q2: "What sprint and scope does T-209 belong to?"):

| Step | Full Names | Abbreviated |
|------|-----------|-------------|
| 1 | Find T-209 row | Lookup: sp=sprint, sc=scope (not s, not st) |
| 2 | Read "sprint" column -> S-15 | Find T-209 row |
| 3 | Read "scope" column -> backend | Locate `sp` column (16th) -> S-15 |
| 4 | — | Locate `sc` column (15th) -> backend |

### Failure Mode Analysis

While the benchmark showed 0% degradation, five failure modes were identified for production scenarios:

1. **No dictionary provided** — catastrophic ambiguity. `c` could be category, comments, created, closed. External research (Columbo, EMNLP 2025) measured 10.54% NL2SQL accuracy drop and 40.5% relation detection drop with undocumented abbreviations.

2. **Domain-specific ambiguity** — `s` defaults to "status" in developer tools but could mean "story", "sprint", "severity" in other contexts. Domain priors may override dictionary lookup.

3. **Multi-schema contexts** — when an agent works with multiple tools using different alias dictionaries, cross-contamination is likely. `p`=priority in Tool A, `p`=parent in Tool B.

4. **Partial context eviction** — the worst failure mode. After context compression, the agent may remember some aliases but not others, applying incorrect mappings silently.

5. **Aggregation at scale** — counting or filtering over 100+ rows while holding alias mappings strains attention, though this is a general LLM limitation independent of aliasing.

---

## 4. Study 3: Session-Level Token Economics

### Measured Constants

All token counts measured with `tiktoken` cl100k_base on real agentquery CLI output:

| Metric | Tokens | Method |
|--------|-------:|--------|
| Schema() roundtrip cost | 85 | 10 (call) + 71 (response) + 4 (overhead) |
| Alias savings per query (compact) | 5 | Fixed: header abbreviation only |
| Alias savings per query (JSON get) | 3 | Abbreviated object keys |
| Alias savings per query (JSON list, 10 items) | 30 | 3 tokens/item x 10 |
| Typical get() response | 23 | Real CLI measurement |
| Typical list() response (8 items) | 177 | Real CLI measurement |

### Session Simulation

Modeled agent sessions of varying length with context eviction (dictionary forgotten every K turns, requiring schema() re-query):

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

**Net positive in only 4 of 16 scenarios (25%).** All require either long sessions (50+ queries) with infrequent eviction, or the unrealistic assumption of no eviction at all.

### Break-Even Analysis

The break-even equation:

```
queries_between_evictions > schema_cost / savings_per_query
```

| Format | Savings/Query | Schema Cost | Break-Even |
|--------|-------------:|------------:|-----------:|
| Compact | 5 | 85 | **17 queries** |
| JSON (get) | 3 | 85 | **29 queries** |
| JSON (list, 10 items) | 30 | 85 | **3 queries** |

For compact format, the agent needs 17 consecutive data queries between schema refreshes — unreliable given typical context eviction patterns (every 10-20 turns in Claude Code after ~180K tokens).

### Workflow Token Budgets

Four representative agent workflows modeled end-to-end:

**Workflow 1: "Check status of 5 tasks"**
- Without aliases: 246 tokens (6 queries)
- With aliases: 306 tokens (7 queries including schema). **Net: +60 tokens (worse)**
- With batching (no aliases): 165 tokens (1 query). **Best option.**

**Workflow 2: "Find blocked tasks and update"**
- Without aliases: 486 tokens (7 queries)
- With aliases: 541+ tokens. **Net: +55 tokens (worse)**

**Workflow 3: "Daily standup review"**
- Without aliases: 130 tokens (3 queries)
- With aliases: 196 tokens (4 queries). **Net: +51% overhead**

**Workflow 4: "Heavy analytics" (100 queries, eviction every 20 turns)**
- Without aliases: ~6,000 tokens
- With aliases: ~6,025 tokens. **Net: -25 tokens (marginal loss)**

### The Irony of JSON Aliases

Aliases in JSON format *are* economical — saving ~3 tokens per item per query, easily exceeding the 85-token schema cost in moderate sessions. But this finding is moot: the recommendation is to use compact format (which saves ~46% vs JSON). Compact format already eliminates the key repetition that makes JSON aliases valuable. **The two optimizations are substitutes, not complements.**

---

## 5. Discussion

### The Core Insight: Schema-Once Kills Aliases

The three studies converge on a single structural insight:

**In any schema-once output format (CSV, TSV, TOON), field names appear exactly once. Abbreviating a one-time occurrence produces a one-time saving. As payload size grows, the marginal value of that saving approaches zero.**

This is not a limitation of our specific implementation — it's an inherent property of the format. Any compact tabular format with a header row followed by value rows will exhibit this behavior. Aliases are a solution to key repetition, and schema-once formats have no key repetition to solve.

The optimization hierarchy for agent-facing output is:

| Rank | Optimization | Savings | Overhead |
|------|-------------|---------|----------|
| 1 | Field projection | Variable (50-80% for selective queries) | Zero |
| 2 | Compact format (JSON -> tabular) | ~46% | Zero |
| 3 | Batching | ~80 tokens per avoided call | Zero |
| 4 | Presets | ~5-10 tokens per query input | Zero |
| **5** | **Field aliases** | **5 tokens (fixed)** | **85 tokens per schema() call** |

Optimizations 1-4 all have zero discovery overhead — they work from the first query. Aliases are the only optimization that requires upfront investment (schema roundtrip) to use, and the investment exceeds the return in most scenarios.

### Format as Transport Concern

This study reinforces a design principle discovered during the output compression implementation: **output format is a transport concern, not a domain concern**. The data source (Schema) should never decide serialization. The caller declares format via CLI flags (`--format compact`, `--aliases`) or SDK parameters (`QueryJSONWithMode(query, LLMReadable)`).

If aliases were implemented, they would follow the same principle — a `--aliases` flag, not a schema-level configuration. But even at the transport level, the economics don't justify the feature.

### When Aliases Would Matter

Aliases become valuable in exactly one scenario: **JSON output where keys repeat per item.** A 100-item list in JSON repeats each field name 100 times. Abbreviating `assignee` (1 token) to `a` (1 token) saves 0 tokens per item (same token count), but abbreviating `description` (1 token) to `d` (1 token) also saves 0 — most common field names are already single tokens in modern BPE tokenizers.

This reveals a second-order insight: **BPE tokenizers already compress common English words efficiently.** "status", "name", "id" — these are all single tokens. The tokenizer has already done the alias's job. Abbreviation only helps for multi-token field names (`blocked_by`, `last_updated`, `story_points`), and even then, the savings per item are 1-2 tokens.

### Comparison with External Research

The Columbo paper (EMNLP 2025) found devastating accuracy drops (10-40%) from abbreviated column names **without dictionaries**. Our benchmark showed 0% degradation **with** dictionaries. The dictionary is essential infrastructure — not optional. But requiring a dictionary means requiring a schema roundtrip, which is the overhead that kills the economics.

The TOON format specification uses full field names in its schema headers despite being designed explicitly for LLM token efficiency. TOON's benchmark shows 73.9% accuracy (vs 69.7% for JSON) with 39.6% fewer tokens — achieved through structural clarity, not name abbreviation. This validates our approach: **structure matters more than name length.**

---

## 6. Conclusion

Field name aliases in schema-once output are a solution to a problem that no longer exists. The compact tabular format — by declaring field names once in a header row — already eliminates the per-item key repetition that aliases target. The marginal savings (5 tokens, fixed) are dwarfed by the schema discovery overhead (85 tokens per roundtrip), producing a net token loss in 75% of simulated scenarios.

### Recommendation

Do not implement field name aliases for compact output format. The existing optimization stack (field projection, compact format, batching, presets) provides 46%+ token reduction with zero discovery overhead. Further token optimization efforts should focus on:

1. **Encouraging `--format compact` adoption** — the 46% savings is the biggest single win
2. **Batch query support** — agents still underutilize `;`-separated multi-query syntax
3. **Preset refinement** — tuning named field bundles to match common agent workflows
4. **Value-level compression** — shortening date formats, using status codes (if warranted by future measurement)

### Decision Record

| Criterion | Threshold | Measured | Result |
|-----------|-----------|----------|--------|
| Net token savings | > 10% | 0.02-1.86% (5 tokens fixed) | FAIL |
| Comprehension degradation | < 5% | 0% (with dictionary) | PASS |
| Schema roundtrip ratio | < 1:5 | Break-even at 1:17 | FAIL |
| **Overall** | All criteria pass | **2 of 3 failed** | **NO-GO** |

---

## References

1. **Columbo: Expanding Abbreviated Column Names for Tabular Data.** EMNLP 2025 Findings. [arxiv.org/html/2508.09403](https://arxiv.org/html/2508.09403). Measured 10.54% NL2SQL accuracy drop and 40.5% relation detection drop from abbreviated column names without dictionaries.

2. **TOON: Token-Oriented Object Notation.** [github.com/toon-format/toon](https://github.com/toon-format/toon). Schema-once format with 39.6% token reduction and 73.9% accuracy (vs 69.7% JSON). Uses full field names in schema headers.

3. **Better Think with Tables: Tabular Structures Enhance LLM Comprehension.** 2024. [arxiv.org/html/2412.17189v3](https://arxiv.org/html/2412.17189v3). 40.29% average performance gain from tabular format vs text — driven by structure, not naming.

4. **LLMLingua: Compressing Prompts for Accelerated Inference.** 2023. [arxiv.org/html/2310.05736v2](https://arxiv.org/html/2310.05736v2). Demonstrated LLM robustness to aggressive prompt compression while maintaining task accuracy.

5. **TOON vs JSON — The New Format Designed for AI.** dev.to, 2025. [dev.to/akki907/toon-vs-json-the-new-format-designed-for-ai-nk5](https://dev.to/akki907/toon-vs-json-the-new-format-designed-for-ai-nk5). Overview of TOON format with token comparison benchmarks.

---

## Appendix: Artifacts and Reproducibility

All research materials are stored in the repository under `.research/`:

| Artifact | Path |
|----------|------|
| Synthetic payload generator | `.research/synthetic-payloads/generate.py` |
| Token measurement script | `.research/synthetic-payloads/measure.py` |
| Token measurement results | `.research/260212_field-alias-token-measurements.md` |
| Comprehension test framework | `.research/comprehension-tests/` |
| Comprehension benchmark results | `.research/260212_alias-comprehension-benchmark.md` |
| Session simulator | `.research/session-simulator/simulate.py` |
| Session analysis + counter spec | `.research/260212_real-session-roundtrip-analysis.md` |

To reproduce the token measurements:

```bash
cd .research/synthetic-payloads
pip3 install tiktoken
python3 generate.py    # generates 12 payload files
python3 measure.py     # tokenizes and writes results
```

To run the session simulator:

```bash
cd .research/session-simulator
python3 simulate.py    # outputs results.md and results.json
```
