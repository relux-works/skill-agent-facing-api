# TASK-260212-27n2hk: implement-field-aliases

## Description
Conditional on positive decision gate. Add alias support to Schema: schema.Field('status', accessor, WithAlias('s')). Schema stores the mapping but never decides when to use it — aliasing is a TRANSPORT CONCERN, same as format. Implementation: (1) Register aliases on Schema: schema.Field('status', accessor, WithAlias('s')). (2) Expose via per-call API: QueryJSONWithMode(query, LLMReadable, WithAliases(true)) or a dedicated mode. (3) CLI flag: --aliases (bool, off by default). When --aliases is passed with --format compact, headers use short names. Without --aliases, compact output uses full names. (4) schema() response includes alias dictionary regardless of flags — it's metadata, not transport. (5) JSON mode ignores --aliases (always full names). (6) Parser accepts both full names and aliases in field projections. Tests: verify alias in compact+aliases output, full names in compact without aliases, full names in JSON always, bidirectional field resolution.

## Scope
(define task scope)

## Acceptance Criteria
(define acceptance criteria)
