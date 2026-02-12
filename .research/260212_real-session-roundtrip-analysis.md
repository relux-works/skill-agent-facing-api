# Real Session Roundtrip Analysis: Field Alias Token Economics

**Date:** 2026-02-12
**Epic:** EPIC-260212-1wmy5b (field-alias-compression)
**Story:** STORY-260212-17tgri (schema roundtrip overhead)
**Tasks:** TASK-260212-1wqvn1, TASK-260212-2jl0j4, TASK-260212-2uzb40

---

## 1. Session Simulator Results

### Measured Constants

All token counts measured with `tiktoken` cl100k_base encoding on real agentquery CLI outputs.

| Metric | Tokens | Source |
|--------|-------:|--------|
| Schema() roundtrip cost | **85** | 10 (call) + 71 (response) + 4 (overhead) |
| Schema() response only | **71** | Real `taskdemo q 'schema()' --format json` output (304 chars) |
| Alias savings per query (compact) | **5** | Fixed: header abbreviation only |
| Alias savings per query (JSON get) | **3** | Abbreviated object keys |
| Alias savings per query (JSON list, 10 items) | **30** | 3 tokens/item x 10 items |
| get(id=X) { overview } result | **23** | Real CLI output |
| list() { overview } 8 items result | **177** | Real CLI output |
| summary() result | **20** | Real CLI output |

### Compact Format Results

With compact/tabular output, aliases only save header tokens (field names appear once).

| Session Length | Eviction K | Schema Calls | Schema Cost | Alias Savings | **Net Balance** |
|--------------:|-----------:|-------------:|------------:|--------------:|----------------:|
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

**Verdict for compact format:** Aliases only pay off in 4/16 scenarios (long sessions with infrequent eviction). Break-even requires **17 queries** between schema() refreshes. With typical eviction every 10-20 turns, aliases are a net loss.

### JSON Format Results

With JSON output, aliases save tokens per item (key names repeat in each object).

| Session Length | Eviction K | Schema Calls | Schema Cost | Alias Savings | **Net Balance** |
|--------------:|-----------:|-------------:|------------:|--------------:|----------------:|
| 10 | 10 | 1 | 85 | 105 | **+20** |
| 20 | 10 | 2 | 170 | 210 | **+40** |
| 50 | 10 | 5 | 425 | 525 | **+100** |
| 100 | 10 | 10 | 850 | 1050 | **+200** |
| 100 | never | 1 | 85 | 1050 | **+965** |

**Verdict for JSON format:** Aliases pay off in 16/16 scenarios. BUT: the recommendation is already to use compact format (which saves ~46% vs JSON). If we're using compact format, this column is irrelevant.

### Head-to-Head Summary

| Scenario | Compact Net | JSON Net | Winner |
|----------|------------:|---------:|--------|
| Short session (10q), frequent eviction | -45 | +20 | JSON aliases work, compact don't |
| Medium session (50q), moderate eviction | -55 | +270 | JSON far better |
| Long session (100q), no eviction | +315 | +965 | JSON still better, but compact ok |

**The irony:** Aliases are most useful in JSON format, but compact format already eliminates the problem aliases try to solve (repeated key names). The two optimizations are substitutes, not complements.

---

## 2. Dictionary Lookup Counter Spec

### Purpose

An instrumented CLI wrapper that captures real-world agent session patterns: how often agents call `schema()`, when they do it, and what the ratio of discovery-to-data queries is.

### Architecture

```
Agent (LLM)
    |
    v
┌──────────────────────┐
│  Instrumented Wrapper │  <-- shell script or Go binary
│  (intercepts all CLI  │
│   invocations)        │
│                       │
│  1. Classify call     │
│  2. Timestamp it      │
│  3. Log to session    │
│  4. Forward to real   │
│     CLI binary        │
└──────────────────────┘
    |
    v
  Real CLI (taskdemo, etc.)
    |
    v
  Output → back to agent
```

### Call Classification

| Category | Detection Pattern | Examples |
|----------|-------------------|----------|
| Schema lookup | Query contains `schema()` | `q 'schema()' --format json` |
| Data query (read) | Query contains `get()`, `list()`, `summary()` | `q 'get(id=X) { overview }'` |
| Search | Command is `grep` or `search` | `grep "TODO"` |
| Batch | Query contains `;` separator | `q 'summary(); list(status=blocked)'` |
| Mutation | Would be any write operations (not in current agentquery, but future) | `update(id=X, status=done)` |

### Session Log Format

```jsonl
{"ts":"2026-02-12T14:30:00Z","session":"s-abc123","seq":1,"category":"schema","query":"schema()","tokens_in":10,"tokens_out":71,"latency_ms":45}
{"ts":"2026-02-12T14:30:05Z","session":"s-abc123","seq":2,"category":"data_query","query":"list(status=blocked) { overview }","tokens_in":15,"tokens_out":120,"latency_ms":38}
{"ts":"2026-02-12T14:30:12Z","session":"s-abc123","seq":3,"category":"data_query","query":"get(id=task-3) { full }","tokens_in":12,"tokens_out":35,"latency_ms":22}
```

### Implementation: Shell Script Wrapper

```bash
#!/usr/bin/env bash
# agent-cli-logger.sh — Instrumented wrapper for agentquery CLIs
#
# Usage: Replace the real CLI binary path, then alias or symlink.
#   alias taskdemo="/path/to/agent-cli-logger.sh"
#
# Session logs go to: ./.agent-sessions/<session-id>.jsonl

REAL_CLI="${AGENT_CLI_REAL_PATH:-./taskdemo}"
SESSION_DIR="${AGENT_SESSION_DIR:-./.agent-sessions}"
SESSION_ID="${AGENT_SESSION_ID:-$(date +%Y%m%d-%H%M%S)-$$}"
LOG_FILE="${SESSION_DIR}/${SESSION_ID}.jsonl"
SEQ_FILE="${SESSION_DIR}/${SESSION_ID}.seq"

mkdir -p "$SESSION_DIR"
[[ -f "$SEQ_FILE" ]] || echo 0 > "$SEQ_FILE"

# Increment sequence number
SEQ=$(($(cat "$SEQ_FILE") + 1))
echo "$SEQ" > "$SEQ_FILE"

# Classify the call
QUERY="$*"
CATEGORY="unknown"
case "$QUERY" in
  *schema\(\)*)  CATEGORY="schema" ;;
  *get\(*)       CATEGORY="data_query" ;;
  *list\(*)      CATEGORY="data_query" ;;
  *summary\(*)   CATEGORY="data_query" ;;
  *grep*|*search*) CATEGORY="search" ;;
esac

# Check for batch queries
if [[ "$QUERY" == *";"* ]]; then
  CATEGORY="batch"
fi

# Timestamp and execute
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
START_MS=$(python3 -c "import time; print(int(time.time()*1000))")

OUTPUT=$("$REAL_CLI" "$@" 2>&1)
EXIT_CODE=$?

END_MS=$(python3 -c "import time; print(int(time.time()*1000))")
LATENCY=$((END_MS - START_MS))

# Estimate token counts (rough: chars/4)
TOKENS_OUT=$(echo -n "$OUTPUT" | wc -c | tr -d ' ')
TOKENS_OUT_EST=$((TOKENS_OUT / 4))

# Write log entry
echo "{\"ts\":\"$TS\",\"session\":\"$SESSION_ID\",\"seq\":$SEQ,\"category\":\"$CATEGORY\",\"query\":\"$(echo "$QUERY" | sed 's/"/\\"/g')\",\"exit_code\":$EXIT_CODE,\"chars_out\":$TOKENS_OUT,\"tokens_out_est\":$TOKENS_OUT_EST,\"latency_ms\":$LATENCY}" >> "$LOG_FILE"

# Pass through output to agent
echo "$OUTPUT"
exit $EXIT_CODE
```

### Go Binary Alternative (Concept)

A Go implementation would be more robust:

```go
// cmd/agent-cli-logger/main.go
// Wraps any agentquery-based CLI with session instrumentation.
//
// Key advantages over shell script:
// - Accurate token counting via tiktoken-go
// - Proper JSON escaping
// - Sub-millisecond timing
// - Can parse the DSL directly (import agentquery parser)
//
// Would import agentquery.Parse() to classify queries at the AST level
// rather than regex matching.
```

### Key Metrics This Tool Would Capture

| Metric | How Computed | What It Tells Us |
|--------|-------------|------------------|
| Schema-to-data ratio | `count(schema) / count(data_query)` | How much overhead schema discovery adds |
| Schema refresh interval | Time gap between consecutive schema() calls | Whether agent front-loads or sprinkles |
| Schema front-loading | Whether schema() is always seq=1 | Validates "learn once, query many" pattern |
| Batch utilization | `count(batch) / count(total)` | Whether agents batch queries or do them one-by-one |
| Avg queries per session | `max(seq)` per session | Typical session length for simulator calibration |
| Token budget per session | `sum(tokens_out)` per session | Real-world token consumption |

### Expected Findings (Predictions)

Based on how LLM agents typically interact with CLI tools:

1. **Schema is front-loaded:** Agents call schema() as their first or second query in ~90% of sessions. LLMs learn the API contract early, then query data.

2. **Schema-to-data ratio is low:** Expected ~1:10 to 1:20. One schema() call, then many data queries.

3. **Batching is underutilized:** Most agents do single queries per tool call, not batched `a(); b()` syntax. This is a training data artifact -- agents are not usually taught batch DSLs.

4. **Schema refresh is rare in short sessions:** For sessions under 20 queries, the agent never re-calls schema(). Context window is large enough to retain it.

5. **Schema refresh happens after context compression:** When Claude Code compresses context (~180K tokens), schema details are likely evicted as "tool output from 30 turns ago" and the agent must re-query.

---

## 3. Workflow Analysis with Token Budgets

### Workflow 1: "Check status of 5 tasks"

Agent goal: Look up the current status of 5 specific tasks by ID.

**Without aliases (compact format):**

| Step | Query | Tokens In | Tokens Out | Cumulative |
|------|-------|----------:|-----------:|-----------:|
| 1 | `schema()` (optional -- agent may know API already) | 10 | 71 | 81 |
| 2 | `get(id=task-1) { overview }` | 10 | 23 | 114 |
| 3 | `get(id=task-2) { overview }` | 10 | 23 | 147 |
| 4 | `get(id=task-3) { overview }` | 10 | 23 | 180 |
| 5 | `get(id=task-4) { overview }` | 10 | 23 | 213 |
| 6 | `get(id=task-5) { overview }` | 10 | 23 | 246 |
| **Total** | 6 queries | **60** | **186** | **246** |

**Optimized (batch query):**

| Step | Query | Tokens In | Tokens Out | Cumulative |
|------|-------|----------:|-----------:|-----------:|
| 1 | `get(id=task-1){overview}; get(id=task-2){overview}; get(id=task-3){overview}; get(id=task-4){overview}; get(id=task-5){overview}` | ~50 | ~115 | ~165 |
| **Total** | 1 query | **~50** | **~115** | **~165** |

**With aliases, savings:** 5 tokens per get() response (compact header abbreviation) x 5 queries = 25 tokens saved. But schema() cost 85 tokens. **Net: -60 tokens (loss).**

**Verdict:** For this workflow, aliases lose 60 tokens. Batching saves more (81 tokens) with zero schema overhead.

### Workflow 2: "Find all blocked tasks and update them"

Agent goal: Find tasks with status=blocked, understand what's blocked, then update.

**Without aliases:**

| Step | Query | Tokens In | Tokens Out | Notes |
|------|-------|----------:|-----------:|-------|
| 1 | `schema()` | 10 | 71 | Learn available operations, fields |
| 2 | `list(status=blocked) { full }` | 15 | ~180 | Find blocked tasks (assume 5 blocked) |
| 3-7 | 5x data queries / updates | ~60 | ~150 | Investigate and resolve each |
| **Total** | 7 queries | **~85** | **~401** | **~486** |

**With aliases:**

| Step | Query | Tokens In | Tokens Out | Notes |
|------|-------|----------:|-----------:|-------|
| 1 | `schema()` (must call to learn aliases) | 10 | 71+alias_dict | Now 71+ tokens for expanded schema |
| 2-7 | Same data queries | ~75 | ~371 | Save ~5 tokens per query |
| **Total** | 7 queries | **~85** | **~442+** | Savings: 30 tokens. Cost: 85+. **Net: -55** |

**Verdict:** Again, aliases lose tokens in a typical workflow.

### Workflow 3: "Daily standup review"

Agent goal: Get project summary, see in-progress tasks, check for blockers.

**Without aliases:**

| Step | Query | Tokens In | Tokens Out | Notes |
|------|-------|----------:|-----------:|-------|
| 1 | `summary()` | 10 | 20 | Quick overview |
| 2 | `list(status=in-progress) { overview }` | 15 | ~70 | 3 in-progress tasks |
| 3 | `list(status=blocked) { overview }` | 15 | ~0 | Check for blockers |
| **Total** | 3 queries | **40** | **~90** | **~130** |

**With aliases (must call schema first):**

| Step | Query | Tokens In | Tokens Out | Notes |
|------|-------|----------:|-----------:|-------|
| 0 | `schema()` | 10 | 71 | Learn aliases |
| 1-3 | Same data queries | 40 | ~75 | Save ~15 tokens total |
| **Total** | 4 queries | **50** | **~146** | **~196** |

**Verdict:** Aliases make this workflow **worse** -- 196 vs 130 tokens (+51% overhead).

### Workflow 4: "Heavy analytics session" (100 queries)

Agent goal: Deep dive into project data, many queries.

**Without aliases (compact):**

| Metric | Value |
|--------|------:|
| Queries | 100 |
| Schema calls | 1 (front-loaded) |
| Avg tokens per query result | ~50 |
| Total query input tokens | ~1000 |
| Total query output tokens | ~5000 |
| **Total session tokens** | **~6000** |

**With aliases (compact, eviction every 20 turns):**

| Metric | Value |
|--------|------:|
| Queries | 100 |
| Schema calls | 5 (1 initial + 4 re-learns) |
| Schema cost | 425 tokens |
| Alias savings | 5 tokens x 80 eligible queries = 400 |
| **Net balance** | **-25 tokens (loss)** |

**Verdict:** Even in a 100-query session with moderate eviction, compact aliases barely break even.

---

## 4. Context Eviction Behavior Analysis

### Claude Code Context Window

- Total context: ~200K tokens
- Compression triggers at: ~180K tokens (estimated)
- Compression preserves: recent messages, system prompt, file contents
- Compression evicts: old tool outputs, verbose intermediate results

### Tool Calls Before Compression

Typical Claude Code session token consumption per tool call:

| Component | Tokens |
|-----------|-------:|
| User message (average) | ~100 |
| Agent reasoning | ~200 |
| Tool call (request) | ~50 |
| Tool result (output) | ~200 |
| **Per-turn total** | **~550** |

At 550 tokens per turn, compression triggers after approximately:

```
180,000 / 550 = ~327 tool calls
```

This is a **very generous** estimate. In practice, with multi-tool turns, file reads, and longer reasoning, compression might trigger after 50-100 turns.

### Would the Alias Dictionary Survive Compression?

Context compression in Claude Code works by summarizing old conversation turns. The schema() output from turn 1 would be compressed to something like:

> "The agent queried the task CLI schema and learned available fields (id, name, status, assignee, description), presets (default, full, minimal, overview), and operations (get, list, schema, summary)."

**Critical question:** Would an alias dictionary survive this compression?

If the schema included aliases like `id->i, name->n, status->s, assignee->a`, the compressed summary would either:

1. **Preserve the mapping** (unlikely for verbose mappings)
2. **Drop the mapping details** (most likely -- compression favors semantic gist over exact values)
3. **Partially preserve** (some aliases remembered, others forgotten -- worst case, leads to errors)

**Prediction:** The alias dictionary has a **high probability of being evicted or corrupted** during compression. It's the exact type of detail (arbitrary abbreviation mapping) that compression algorithms would consider non-essential.

### Risk: Partial Eviction

The worst failure mode is not total eviction (agent knows to re-query) but **partial eviction**:

- Agent remembers `id->i` but forgets `assignee->a`
- Agent uses abbreviated field names inconsistently
- Results are misinterpreted because the agent applies wrong mappings

This risk exists with any arbitrary abbreviation scheme and is hard to detect or prevent.

---

## 5. Break-Even Analysis

### The Fundamental Equation

```
Net benefit = (queries_between_evictions * savings_per_query) - schema_roundtrip_cost
```

For net benefit > 0:

```
queries_between_evictions > schema_roundtrip_cost / savings_per_query
```

### Break-Even Points by Format

| Format | Savings/Query | Schema Cost | Break-Even Queries |
|--------|-------------:|------------:|-------------------:|
| Compact (all queries) | 5 | 85 | **17** |
| JSON (get only) | 3 | 85 | **29** |
| JSON (list, 10 items) | 30 | 85 | **3** |
| JSON (mixed, 50% get + 30% list) | 10.5 | 85 | **9** |

### Realistic Break-Even With Eviction

For compact format, the agent needs 17 data queries between each schema() call. With eviction every K turns:

| Eviction K | Effective data queries per cycle | Breaks even? |
|-----------:|--------------------------------:|:-------------|
| 10 | 8 (80% are data queries) | **No** (need 17) |
| 20 | 16 | **No** (barely short) |
| 25 | 20 | **Yes** (just barely) |
| 50 | 40 | **Yes** |
| Never | All session queries | **Yes** (if session > 17 queries) |

**Bottom line for compact format:** Aliases only break even if context survives at least 25 turns between compressions. This is achievable in some sessions but not reliably guaranteed.

For JSON format with mixed queries, break-even is at 9 queries -- much more achievable. But this is moot because the recommendation is to use compact format.

---

## 6. Recommendation

### Is the schema() overhead acceptable?

**For compact output format: No, field-name aliases are not worth the schema() overhead.**

The numbers are clear:

1. **Compact aliases save 5 tokens per query** -- a fixed, tiny amount because field names appear only in the header
2. **Schema() costs 85 tokens per call** -- requiring 17 data queries to recoup
3. **Context eviction** forces re-learning every 10-50 turns, repeatedly paying the 85-token cost
4. In **12 out of 16** simulated scenarios (compact format), aliases produce a net token loss
5. **Best case** savings (100-query session, no eviction): +315 tokens -- a 5% reduction on a ~6000 token session
6. **Worst case** loss (100-query session, eviction every 10 turns): -450 tokens

### The Larger Picture

The alias question is already answered by the output format choice:

- **JSON format** repeats field names per item, making aliases save ~3 tokens per item -- meaningful at scale
- **Compact/tabular format** declares field names once in a header -- aliases save 5 tokens total, once

Since the recommendation (from the output compression research) is to use compact format, field-name aliases become pointless. The compact format **already eliminates the key repetition** that aliases are designed to address.

**Aliases in compact format are solving a problem that compact format already solved.**

### What Actually Matters

Instead of aliases, token budget is far better spent on:

1. **Compact output format itself** -- saves ~46% vs JSON (confirmed by token measurements)
2. **Batching support** -- agents sending `a(); b(); c()` saves tool call overhead
3. **Field projection** -- `{ overview }` vs `{ full }` lets agents request only needed fields
4. **Presets** -- `{ overview }` instead of `{ id name status assignee }` saves query input tokens

These existing features of agentquery provide far more token savings than aliases ever could, with zero schema() overhead cost.

---

## Appendix: Simulator Location and Usage

**Simulator script:** `.research/session-simulator/simulate.py`

```bash
# Run the simulator
python3 .research/session-simulator/simulate.py

# Results written to:
# .research/session-simulator/results.md    (formatted report)
# .research/session-simulator/results.json  (raw data)
```

**Dictionary counter spec:** Section 2 of this document (above). Shell script implementation included inline. Go binary concept described for future implementation if empirical data is needed.

**Token measurements:** `.research/260212_field-alias-token-measurements.md` and `.research/synthetic-payloads/measure.py`
