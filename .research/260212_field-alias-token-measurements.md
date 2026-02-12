# Field Alias Token Measurement Results

**Date:** 2026-02-12
**Encoding:** cl100k_base (GPT-4 / Claude-compatible BPE tokenizer)
**Seed:** 42 (reproducible)

## Methodology

Generated synthetic task tracker payloads at 4 scales (5, 20, 100, 500 items)
with 8 fields per item: id, name, status, assignee, description, priority, created, updated.

Three format variants per scale:
- **JSON**: Standard `json.dumps(items, indent=2)` — pretty-printed JSON array
- **Compact-full**: CSV-style with full field names as header, data rows below
- **Compact-alias**: Same CSV-style but header uses 1-char abbreviations (id->i, name->n, etc.)

Token counts measured with `tiktoken` using `cl100k_base` encoding.

## 1. Raw Token Counts

| Scale | JSON | Compact-Full | Compact-Alias | JSON bytes | Compact-Full bytes | Compact-Alias bytes |
|------:|-----:|-------------:|--------------:|-----------:|-------------------:|--------------------:|
| 5 | 485 | 269 | 264 | 1,732 | 1,047 | 1,002 |
| 20 | 1,957 | 1,055 | 1,050 | 6,912 | 4,000 | 3,955 |
| 100 | 9,836 | 5,283 | 5,278 | 34,740 | 19,942 | 19,897 |
| 500 | 48,933 | 26,144 | 26,139 | 172,962 | 98,726 | 98,681 |

## 2. Savings: JSON to Compact-Full

| Scale | JSON Tokens | Compact-Full Tokens | Saved | % Reduction |
|------:|------------:|--------------------:|------:|------------:|
| 5 | 485 | 269 | 216 | 44.5% |
| 20 | 1,957 | 1,055 | 902 | 46.1% |
| 100 | 9,836 | 5,283 | 4,553 | 46.3% |
| 500 | 48,933 | 26,144 | 22,789 | 46.6% |

## 3. Marginal Savings: Compact-Full to Compact-Alias (KEY METRIC)

This measures the **incremental benefit** of abbreviating field names in the header,
given that we already use compact tabular format.

| Scale | Compact-Full Tokens | Compact-Alias Tokens | Tokens Saved | % Reduction | Bytes Saved |
|------:|--------------------:|---------------------:|-------------:|------------:|------------:|
| 5 | 269 | 264 | 5 | 1.86% | 45 |
| 20 | 1,055 | 1,050 | 5 | 0.47% | 45 |
| 100 | 5,283 | 5,278 | 5 | 0.09% | 45 |
| 500 | 26,144 | 26,139 | 5 | 0.02% | 45 |

## 4. Full Comparison Summary

| Scale | JSON | Compact-Full | Compact-Alias | JSON->CF % | CF->CA % | JSON->CA % |
|------:|-----:|-------------:|--------------:|-----------:|---------:|-----------:|
| 5 | 485 | 269 | 264 | 44.5% | 1.86% | 45.6% |
| 20 | 1,957 | 1,055 | 1,050 | 46.1% | 0.47% | 46.3% |
| 100 | 9,836 | 5,283 | 5,278 | 46.3% | 0.09% | 46.3% |
| 500 | 48,933 | 26,144 | 26,139 | 46.6% | 0.02% | 46.6% |

## 5. Tokens Per Item (Amortized)

| Scale | JSON/item | Compact-Full/item | Compact-Alias/item |
|------:|----------:|------------------:|-------------------:|
| 5 | 97.0 | 53.8 | 52.8 |
| 20 | 97.8 | 52.8 | 52.5 |
| 100 | 98.4 | 52.8 | 52.8 |
| 500 | 97.9 | 52.3 | 52.3 |

## 6. Analysis

### JSON to Compact-Full: Large, Consistent Win

Switching from JSON to compact tabular format saves **~46%** of tokens on average.
This is a substantial reduction driven by eliminating:
- Repeated field name keys on every item
- JSON structural characters (`{{`, `}}`, `[`, `]`, `:`, `"`)
- Indentation whitespace

The savings scale well: they remain consistent as item count grows,
because the overhead is proportional to the number of items in JSON.

### Compact-Full to Compact-Alias: Negligible Marginal Benefit

Abbreviating field names in the header saves **~0.61%** of tokens on average.
In absolute terms, the savings are **5 tokens** (5 items) to **5 tokens** (500 items).

Why so small? Because in the compact format, field names appear **only once** — in the header row.
The header `id,name,status,assignee,description,priority,created,updated` is a single line
consuming a fixed number of tokens regardless of how many data rows follow.
Abbreviating it to `i,n,s,a,d,p,c,u` saves those few tokens once, and that's it.

The data rows (which dominate the payload) are identical in both variants.

### Cost-Benefit Verdict

| Factor | Assessment |
|--------|-----------|
| Token savings | 0.61% average — negligible |
| Absolute savings at 500 items | 5 tokens — trivial |
| Readability cost | High — `i,n,s,a,d,p,c,u` is unreadable without a legend |
| Implementation complexity | Moderate — need alias registry, mapping, docs |
| Agent confusion risk | Non-trivial — agents may misinterpret abbreviated headers |
| Schema discoverability | Degraded — header no longer self-documenting |

**Recommendation: Do NOT implement field name aliases.**

The marginal token savings are negligible compared to the readability and complexity costs.
The big win is already captured by switching from JSON to compact tabular format.
Further compression efforts should target the data values themselves (e.g., date format
abbreviation, status code mapping) rather than the one-time header line, though even those
are unlikely to be worth the tradeoff.
