# STORY-260212-17tgri: schema-roundtrip-frequency

## Description
Research how often an LLM agent needs to re-query schema() to refresh the alias dictionary during a typical work session. Analyze: (1) How long does the mapping persist in agent context before it gets compressed/evicted? (2) In a typical 20-query session, how many schema() calls would be needed? (3) Do the extra schema() round-trips negate the per-query savings from abbreviation? Model the break-even: if schema() costs N tokens and each abbreviated query saves M tokens, after how many queries does abbreviation pay off? Factor in context window compression — if the alias dictionary gets evicted, the agent must re-query.

## Scope
(define story scope)

## Acceptance Criteria
(define acceptance criteria)
