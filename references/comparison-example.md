# Context Efficiency Comparison: MCP vs Mini-Query DSL vs Board Grep

**Date:** 2026-02-11
**Board:** `/Users/aagrigore1/src/skill-project-management/.task-board/`
**Board size:** 18 epics, 69 stories, 244 tasks, 15 bugs (346 elements total)

---

## 1. Methodology

### Measurement approach

1. **Live execution** against the real `.task-board/` directory (346 elements, mostly in `done` status).
2. **Output bytes** measured by piping command output through `wc -c`.
3. **ANSI stripping** applied to CLI text output (`perl -pe 's/\e\[\d+m//g'`) since LLMs process the raw byte stream including escape codes, but they carry zero semantic information.
4. **Token estimation** using standard ratios:
   - English text: **1 token per 4 bytes**
   - JSON output: **1 token per 3 bytes** (more punctuation, shorter words)
   - JSON schemas/tool definitions: **1 token per 3 bytes**
5. **MCP and DSL produce identical JSON** for the same field selections -- both use the shared `internal/fields` package. MCP output measurements are therefore equal to DSL output measurements.
6. **Input overhead** measured as byte length of the command string (Bash) or JSON input body (MCP).
7. **Per-call framing** estimated at ~80 tokens per tool round-trip (50 tokens request frame + 30 tokens result frame). This is identical for Bash and MCP tool calls.

### What is NOT identical: session overhead

- **MCP**: 12 tool definitions loaded into the agent's context at session start. This is a one-time cost (~2,200 tokens) that persists for the entire session.
- **DSL/Grep via Bash**: Zero additional tool definitions. The Bash tool is already part of the agent's base system prompt.
- **CLI via Bash**: Same as DSL/Grep -- zero additional definitions.

---

## 2. Raw Measurements

### Output sizes (response tokens consumed by the agent reading the result)

| Workflow | CLI (text) | DSL (JSON) | MCP (JSON) | Grep (text) |
|----------|-----------|-----------|-----------|------------|
| **W1: Orientation** (`summary`) | 714 B / ~179 tok | 1,099 B / ~366 tok | 1,099 B / ~366 tok | 1,818 B / ~455 tok |
| **W2: Task lookup** (default) | 657 B / ~164 tok | 114 B / ~38 tok | 114 B / ~38 tok | 0 B (task not in file content) |
| **W2: Task lookup** (full) | 657 B / ~164 tok | 907 B / ~302 tok | 907 B / ~302 tok | N/A |
| **W2: Task lookup** (minimal) | 657 B / ~164 tok | 60 B / ~20 tok | 60 B / ~20 tok | N/A |
| **W3: Filtered list** (minimal) | 285 B / ~71 tok | 114 B / ~38 tok | 114 B / ~38 tok | 1,346 B / ~337 tok |
| **W3: Filtered list** (overview) | 285 B / ~71 tok | 246 B / ~82 tok | 246 B / ~82 tok | 1,346 B / ~337 tok |
| **W4: Agent monitoring** | 1,552 B / ~388 tok | 1,795 B / ~598 tok | 1,795 B / ~598 tok | 44,084 B / ~11,021 tok |
| **W5: Batch (3 tasks, status only)** | 2,173 B / ~543 tok | 103 B / ~34 tok | 3x ~60 B = ~180 B / ~60 tok | 1,045 B / ~261 tok |

### Input sizes (tokens the agent spends to formulate the request)

| Workflow | CLI | DSL | MCP | Grep |
|----------|-----|-----|-----|------|
| **W1** | 18 B / ~5 tok | 24 B / ~6 tok | 20 B / ~5 tok | 66 B / ~17 tok |
| **W2** (default) | 34 B / ~9 tok | 50 B / ~13 tok | 28 B / ~7 tok | 41 B / ~10 tok |
| **W3** (minimal) | 42 B / ~11 tok | 62 B / ~16 tok | 64 B / ~16 tok | 48 B / ~12 tok |
| **W4** | 17 B / ~4 tok | 31 B / ~8 tok | 20 B / ~5 tok | 48 B / ~12 tok |
| **W5** (3 tasks) | 110 B / ~28 tok | 121 B / ~30 tok | 3x 50 B = 150 B / ~38 tok | 116 B / ~29 tok |

### Total tokens per workflow (input + output + per-call framing at 80 tok/call)

| Workflow | CLI | DSL | MCP | Grep |
|----------|-----|-----|-----|------|
| **W1: Orientation** | 264 | 452 | 451 | 552 |
| **W2: Task lookup** (default) | 253 | 131 | 125 | N/A (fails) |
| **W2: Task lookup** (minimal) | 253 | 113 | 107 | N/A |
| **W3: Filtered list** (minimal) | 162 | 134 | 134 | 429 |
| **W4: Agent monitoring** | 472 | 686 | 683 | 11,113 |
| **W5: Batch** (status) | 811 (3 calls x 270) | 144 | 338 (3 calls x 113) | 610 (3 calls x 203) |

**Notes:**
- CLI counts include 3 separate Bash calls for W5 (3x framing overhead).
- MCP counts include 3 separate MCP calls for W5 (3x framing overhead).
- DSL handles W5 in a single Bash call (1x framing overhead).
- Grep fails entirely for W2 (task ID is in directory name, not file content).
- Grep's W4 result (44 KB) is catastrophically large -- it matches every `progress.md` header.

---

## 3. Session Overhead

### MCP: one-time tool definition cost

| Component | Bytes | Tokens |
|-----------|-------|--------|
| 12 tool definitions (names, descriptions, JSON schemas) | ~7,600 B | ~2,200 tok |
| **Read-only tools** (5 tools: get_element, list, summary, plan, agents) | ~2,448 B | ~815 tok |
| **Write tools** (7 tools: update_status, create, assign, unassign, progress, link, search) | ~3,327 B | ~1,109 tok |
| **Total loaded per session** | ~7,621 B | ~2,200 tok |

### DSL: zero additional overhead

The DSL adds no tool definitions. The agent already has the Bash tool. It just needs to know the query syntax, which can be provided in the system prompt or discovered with `task-board q --help`.

If we include DSL syntax documentation in the system prompt (the `--help` text is ~500 bytes), that's ~125 tokens -- about 18x smaller than MCP tool definitions.

### Grep: zero additional overhead

Same as DSL. No tool definitions. Usage is discoverable via `task-board grep --help` (~200 bytes / ~50 tokens).

### CLI: zero additional overhead

Same. No tool definitions.

---

## 4. Break-Even Analysis

The question: **At how many queries does MCP's session overhead (2,200 tokens) pay for itself vs DSL?**

### Per-query savings: MCP vs DSL

MCP and DSL produce identical JSON output. The only per-query differences are:

| Factor | MCP | DSL | Delta |
|--------|-----|-----|-------|
| Input size | Slightly smaller (JSON vs command string) | Slightly larger | ~5-10 tokens MCP advantage |
| Output size | Identical | Identical | 0 |
| Framing | Same (~80 tok) | Same (~80 tok) | 0 |
| Batch capability | No (one call per operation) | Yes (semicolons in single call) | DSL advantage for batches |

**Single-query delta:** MCP saves ~5-10 tokens per query on input.

**Break-even at:** 2,200 / 7.5 = **~293 queries per session**

For batch queries (W5-style), the math reverses:
- DSL batch of 3: 144 tokens (1 call)
- MCP batch of 3: 338 tokens (3 calls)
- **DSL saves ~194 tokens per 3-element batch**

With batching factored in, MCP **never breaks even** against DSL for typical agent sessions.

### MCP vs CLI

CLI text output is sometimes smaller (W1: 179 vs 366 tokens) and sometimes larger (W5: 543 vs 34 tokens). The key advantage of structured approaches (MCP/DSL) is **field selection** -- the agent can request only what it needs.

For field-selected queries:
- `get(X) { minimal }` = 20 tokens vs `show X` = 164 tokens
- Savings: **144 tokens per lookup**
- Break-even for MCP vs CLI: 2,200 / 144 = **~15 queries per session**

### MCP vs Grep

Grep is so unreliable (fails for W2, 11K tokens for W4) that break-even analysis is meaningless. Grep loses on every workflow where the board has more than a few elements.

---

## 5. Recommendation

### Primary recommendation: **Mini-Query DSL**

The DSL is the optimal approach for all measured workflows:

| Criterion | DSL | MCP | CLI | Grep |
|-----------|-----|-----|-----|------|
| Session overhead | None | ~2,200 tok | None | None |
| Field selection | Yes | Yes | No | No |
| Batch support | Yes (single call) | No (N calls) | No (N calls) | No (N calls) |
| Output format | Structured JSON | Structured JSON | Human text + ANSI | Raw file:line:content |
| Reliability | High | High | High | Low (content-only search) |
| Discovery | `--help` in prompt | Tool definitions auto-loaded | `--help` in prompt | `--help` in prompt |

### When to use each approach

**DSL (default for all agent workflows):**
- Any read operation: orientation, lookup, filtering, monitoring, batch queries
- Particularly dominant for batch operations (single call vs N calls)
- Best for sessions with mixed read patterns

**MCP (specific niche: long-lived sessions with external tool orchestration):**
- When the agent platform natively supports MCP and tool switching overhead matters more than context size
- When the session is so long that 2,200 tokens of overhead becomes negligible
- When other MCP servers are already in use (marginal cost of adding one more is lower)

**CLI (human-facing output or quick checks):**
- When a human is reading the output directly
- When ANSI formatting aids comprehension
- Not recommended for agent-to-agent communication

**Grep (last resort for unstructured exploration):**
- Searching for text patterns that span multiple elements (e.g., "who mentioned X in their notes?")
- Discovering elements by content rather than by ID or status
- Never for structured queries that the DSL handles natively

---

## 6. Summary

The Mini-Query DSL is the clear winner for agent workflows: it has zero session overhead (vs MCP's 2,200 tokens), supports batch queries in a single tool call (saving ~194 tokens per 3-element batch vs MCP's 3 separate calls), and produces the same structured JSON output as MCP. Board Grep is a niche tool for unstructured text search that fails catastrophically on structured queries (11K tokens for agent monitoring vs DSL's 686). MCP only becomes competitive in sessions exceeding ~300 queries, a threshold that typical agent tasks never reach.

---

## Appendix: Token Estimation Calibration

Token estimates used in this document are approximations. Actual token counts vary by model tokenizer. The ratios used:

| Content type | Bytes per token | Rationale |
|-------------|-----------------|-----------|
| English text (CLI output) | 4.0 | Standard English tokenization |
| JSON (DSL/MCP output) | 3.0 | More punctuation characters, shorter key names |
| JSON Schema (tool definitions) | 3.0 | Same as JSON |
| Command strings | 4.0 | Mix of English words and identifiers |

For production use, these should be validated against the actual tokenizer (cl100k_base for Claude models).
