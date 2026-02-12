# LLM Alias Comprehension Test Framework

**Purpose:** Measure whether abbreviated field names (aliases) degrade LLM comprehension of structured data.

## Structure

Each level has 4 files:
- `level-N-aliases.txt` — alias dictionary + abbreviated compact output
- `level-N-full.txt` — same data with full field names (control group)
- `level-N-questions.txt` — 10 questions about the data
- `level-N-answers.txt` — correct answers (ground truth)

## Levels

| Level | Fields | Alias Style | Purpose |
|-------|--------|-------------|---------|
| 1 | 5 | Single-char (i,n,s,a,d) | Baseline — easy |
| 2 | 15 | 1-2 char (i,n,s,a,d,p,t,cr,up,dl,e,tg,bl,cm,pr) | Realistic |
| 3 | 30 | 1-2 char with collisions (s vs sc vs sp, c vs cl vs cr vs cm) | Stress test |

## Methodology

1. Present alias dictionary + data to LLM
2. Ask 10 questions (mix of lookup, filter, cross-reference, aggregation, multi-field)
3. Score accuracy (correct/incorrect per question)
4. Compare abbreviated vs full field names
5. Analyze failure modes
