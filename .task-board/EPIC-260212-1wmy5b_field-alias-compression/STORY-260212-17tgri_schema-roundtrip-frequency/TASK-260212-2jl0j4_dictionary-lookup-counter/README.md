# TASK-260212-2jl0j4: dictionary-lookup-counter

## Description
Build an instrumented wrapper around the CLI that counts and logs every schema() call separately from data queries. The wrapper intercepts all CLI invocations, classifies them (schema lookup vs data query vs mutation), timestamps each call, and writes a session log. From the log, derive: (1) ratio of schema() calls to data queries, (2) time gaps between schema() refreshes, (3) whether the LLM front-loads schema() or sprinkles it throughout. This is the key metric — if the LLM calls schema() every 3-4 queries, the overhead may kill the savings. Tool can be a simple shell wrapper or Go binary that proxies to the real CLI.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
