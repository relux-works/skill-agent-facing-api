# TASK-260212-1o8riy: tokenize-and-measure

## Description
Run each synthetic payload through a tokenizer (tiktoken cl100k_base or Claude tokenizer). Record token counts per variant per scale. Build comparison table: JSON baseline vs compact-full vs compact-abbreviated. Calculate: (1) absolute savings per payload, (2) % savings from abbreviation alone (compact-full vs compact-abbreviated), (3) marginal benefit of abbreviation on top of compact format. Store results in .research/260212_field-alias-token-measurements.md.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
