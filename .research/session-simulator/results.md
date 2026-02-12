# Session Simulator Results

**Date:** 2026-02-12
**Purpose:** Model token economics of field-name aliases across agent sessions

## Measured Constants

- Schema() roundtrip cost: **85 tokens** (call=10 + response=71 + overhead=4)
- Alias savings per query (compact format): **5 tokens** (fixed, header-only)
- Alias savings per query (JSON get): **3 tokens**
- Alias savings per query (JSON list, 10 items): **30 tokens**

## Query Mix Assumptions

- get: 50%
- list: 30%
- summary: 10%
- other: 10%
- Average items per list(): 10

## Results: Compact Format

| Session Length | Eviction K | Schema Calls | Schema Cost | Alias Savings | Net Balance | Break-Even |
|--------------:|-----------:|-------------:|------------:|--------------:|------------:|-----------:|
| 10 | 10 | 1 | 85 | 40 | -45 | never |
| 10 | 20 | 1 | 85 | 40 | -45 | never |
| 10 | 50 | 1 | 85 | 40 | -45 | 21 |
| 10 | never | 1 | 85 | 40 | -45 | never |
| 20 | 10 | 2 | 170 | 80 | -90 | never |
| 20 | 20 | 1 | 85 | 80 | -5 | never |
| 20 | 50 | 1 | 85 | 80 | -5 | 21 |
| 20 | never | 1 | 85 | 80 | -5 | never |
| 50 | 10 | 5 | 425 | 200 | -225 | never |
| 50 | 20 | 3 | 255 | 200 | -55 | never |
| 50 | 50 | 1 | 85 | 200 | +115 | 21 |
| 50 | never | 1 | 85 | 200 | +115 | 21 |
| 100 | 10 | 10 | 850 | 400 | -450 | never |
| 100 | 20 | 5 | 425 | 400 | -25 | never |
| 100 | 50 | 2 | 170 | 400 | +230 | 21 |
| 100 | never | 1 | 85 | 400 | +315 | 21 |

## Results: JSON Format

| Session Length | Eviction K | Schema Calls | Schema Cost | Alias Savings | Net Balance | Break-Even |
|--------------:|-----------:|-------------:|------------:|--------------:|------------:|-----------:|
| 10 | 10 | 1 | 85 | 105 | +20 | 8 |
| 10 | 20 | 1 | 85 | 105 | +20 | 8 |
| 10 | 50 | 1 | 85 | 105 | +20 | 8 |
| 10 | never | 1 | 85 | 105 | +20 | 8 |
| 20 | 10 | 2 | 170 | 210 | +40 | 8 |
| 20 | 20 | 1 | 85 | 210 | +125 | 8 |
| 20 | 50 | 1 | 85 | 210 | +125 | 8 |
| 20 | never | 1 | 85 | 210 | +125 | 8 |
| 50 | 10 | 5 | 425 | 525 | +100 | 8 |
| 50 | 20 | 3 | 255 | 525 | +270 | 8 |
| 50 | 50 | 1 | 85 | 525 | +440 | 8 |
| 50 | never | 1 | 85 | 525 | +440 | 8 |
| 100 | 10 | 10 | 850 | 1050 | +200 | 8 |
| 100 | 20 | 5 | 425 | 1050 | +625 | 8 |
| 100 | 50 | 2 | 170 | 1050 | +880 | 8 |
| 100 | never | 1 | 85 | 1050 | +965 | 8 |

## Head-to-Head: Compact vs JSON Alias Savings

| Session | Eviction | Compact Net | JSON Net | Better Format | Margin |
|--------:|---------:|------------:|---------:|--------------:|-------:|
| 10 | 10 | -45 | +20 | JSON | 65 |
| 10 | 20 | -45 | +20 | JSON | 65 |
| 10 | 50 | -45 | +20 | JSON | 65 |
| 10 | never | -45 | +20 | JSON | 65 |
| 20 | 10 | -90 | +40 | JSON | 130 |
| 20 | 20 | -5 | +125 | JSON | 130 |
| 20 | 50 | -5 | +125 | JSON | 130 |
| 20 | never | -5 | +125 | JSON | 130 |
| 50 | 10 | -225 | +100 | JSON | 325 |
| 50 | 20 | -55 | +270 | JSON | 325 |
| 50 | 50 | +115 | +440 | JSON | 325 |
| 50 | never | +115 | +440 | JSON | 325 |
| 100 | 10 | -450 | +200 | JSON | 650 |
| 100 | 20 | -25 | +625 | JSON | 650 |
| 100 | 50 | +230 | +880 | JSON | 650 |
| 100 | never | +315 | +965 | JSON | 650 |

## Analysis

### Scenarios with positive net balance (aliases pay off)
- Compact format: **4/16** scenarios
- JSON format: **16/16** scenarios

### Best/Worst Cases

**Compact format:**
- Best: session=100, eviction=never → net=+315 tokens
- Worst: session=100, eviction=10 → net=-450 tokens

**JSON format:**
- Best: session=100, eviction=never → net=+965 tokens
- Worst: session=10, eviction=10 → net=+20 tokens

### Key Insight

The fundamental problem is the **asymmetry between savings and costs**:

- Schema() costs **85 tokens** per call
- Compact aliases save **5 tokens** per query
- Therefore, each schema() call requires **17 data queries** just to break even

With context eviction every K turns, the agent must repeatedly re-learn the alias
dictionary. Each re-learn costs 85 tokens and saves only 5 per subsequent query.
For compact format, aliases are **almost never worth it**.

For JSON format, aliases save more per query (~3 tokens/item in lists), which can
occasionally make them worthwhile in long sessions without eviction. But this only
applies to the JSON output format — and the recommendation is already to use compact
format, which eliminates the per-item key repetition entirely.
