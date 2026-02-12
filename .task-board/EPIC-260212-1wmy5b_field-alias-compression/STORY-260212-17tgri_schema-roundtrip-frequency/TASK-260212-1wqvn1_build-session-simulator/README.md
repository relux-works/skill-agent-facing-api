# TASK-260212-1wqvn1: build-session-simulator

## Description
Build a lightweight session simulator that tracks schema() calls. The simulator models an agent session: N queries against a CLI tool with abbreviated output. After each query, the agent may or may not need to re-query schema() for the alias dictionary. Instrument the simulator to count: (1) total schema() calls per session, (2) tokens spent on schema() responses, (3) tokens saved from abbreviation per query. Model context eviction: assume dictionary gets compressed after K turns (configurable). Output: net token balance per session length (10, 20, 50, 100 queries).

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
