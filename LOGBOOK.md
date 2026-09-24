# Flight Logbook

> Institutional memory. Concise, factual, high-signal.
> Newest entries first. One block per insight.

## 2026-09-24

### 2320 — the Confluence adapter pushes down one bit of the projection, not none of it
- FINDING: `TASK-260924-11465n`'s adapter evidence reports `skill-confluence-management` as projection-local-only, on the grounds that `getPageV1` sends a fixed expand list `version,space,ancestors,metadata.labels` "regardless of what the agent projected" and `getPageV2` "sends only `body-format`". Re-read of the primary source at the cited commit `10e342b1` shows one exception both the producer and the reviewer passed over: `opGet` (`internal/query/schema.go#L247-L256`) computes `includeBody := containsField(ctx.Statement.Fields, "body")` from the projection and passes it into `GetPage`, where `getPageV2` sends `body-format=storage` only when it is set and `getPageV1` appends `,body.storage` to the expand list only when it is set. One bit of the selection — the single most expensive field on a page — does reach the API.
- DECISION: the article states the partial pushdown explicitly instead of inheriting the flat "local only" phrasing. The accepted conclusion is unaffected: projection always saves agent context, and saves backend work only where the adapter pushes the selection upstream. Jira pushes the whole selection; Confluence pushes one boolean on `get` and nothing on `list`.
- SCOPE: `articles/field-alias-compression-study.md` and both blog posts. The measurement artifacts in `.research/layer-ladder/` are not edited by this task; the discrepancy is in prose framing, not in any measured number.

### 2315 — SKILL.md still carries the dead-weight figure the layer ladder rules out
- FINDING: `SKILL.md:37` ("Why not MCP?") claims "~2,000-3,000 tokens of dead weight per session" for a 10-15 operation tool surface, and asserts the DSL "costs zero". The layer-ladder MCP section measures 1,490 and 1,774 tokens for a 9-tool contract in the minimal and structured profiles, against 1,154 for the equivalent `schema()` response — so the figure is unsupported on every profile measured, and `schema()` is not zero either. The same section shows the scenario-contract margin inverting between profiles, so no single-direction claim survives.
- STATUS: pending, out of scope for `TASK-260924-jlzkva`, whose scope is the three article texts. The article does not repeat the claim and names it as ruled out. `SKILL.md` needs its own tracked edit.

### 2110 — layer-ladder measurement: a percentage is only as good as the gate on its denominator
- FINDING: `reconcile_ladder` closed the *token* arithmetic of every ladder exactly and still admitted a wrong published percentage. Substituting the ladder base for the previous rung in `.research/layer-ladder/measure.py:build_ladder` left all token counts correct, the ladder closing, and `measure.py --check` at exit 0 — while the "% of previous rung" column silently read 49.28% instead of 69.08% (L1→L3) and 8.66% instead of 39.25% (L3→L4) at scale 5. The whole 32-test suite stayed green.
- ROOT CAUSE: `pct()` was unit-tested in isolation. The production composition deciding *what gets passed to it* had no gate. A helper proven correct on its own promises nothing about its call site.
- FIX: new production gate `measure.py:reconcile_percentages`, called from `build_ladder` after `reconcile_ladder`. The regression test drives `measure.run` over the real fixtures, anchors each denominator to the raw `tokens_by_scale` table rather than the ladder's self-report, and recomputes the expected value inline instead of through `pct` — so a mutant changing both consistently still dies. Mutants M10–M12.
- NOTE: `measure.run` is called inside each test method, not in `setUpClass`. A production refusal raised in class setup is reported by unittest as `ERROR: setUpClass (...)`, which no named-test mutant check can match.

### 2105 — an amortization cost sunk on both sides of a transition cancels
- FINDING: the amortization table charged the full 535-token `schema()` roundtrip as the one-time cost of the projection (L1→L3) and compact (L3→L4) transitions. Both sides of both are `agentquery` output — L1/L3 are `QueryJSONWithMode(HumanReadable)`, L4 is `LLMReadable` — same schema, same session. The introspection was already paid on the source side, so it cancels.
- REGRESSION: published break-evens of 3 and 13 queries and "Net @10 = −115" for compact at 5 rows were all wrong. True values: 1 query, 1 query, +420. Only the alias row (21-token legend, break-even 11) was ever sound, because L4 genuinely does not hold that legend.
- FIX: `amortize` no longer takes a one-time cost; it derives it from the transition via `LAYER_SESSION_PREREQUISITES` and `incremental_session_artifacts` (set difference: what the destination needs that the source did not). `validate_amortization`, called from `measure.py:run`, refuses any row charging an artifact its source rung already held — over- and under-charging both. Mutants M13, M14.
- DECISION: the "the DSL pays for itself above a certain response size" framing is retracted. That prices DSL adoption against a non-DSL baseline, and this ladder has no non-DSL rung — L1 is agentquery output, not a REST response. The ladder cannot price adoption at all, and now says so in the generated report, the research note and the handoff.

### 2100 — spec-revision freshness needs a dated probe with a control, not an assertion
- FINDING: `validate_mcp_contract` required `protocol_version` to be *present* and had no way to know whether it was *current*. It passed a pin four revisions stale (`2025-06-18`) in silence. `/specification/latest` resolves 307 → `/specification/2026-07-28`.
- DECISION: freshness rests on `mcp/spec-probe.json`, a dated checked-in primary-source read, gated by `validate_spec_probe` (called from `measure_mcp`). The probe carries its own control: revision `2099-01-01`, which cannot exist, must return 404. Without that control an outage that 404s everything would make every real revision look withdrawn — an absence and a failed read are different facts, and a `503` on the control is rejected just as a `200` is. Mutants M15, M16 narrow each half; with both planted `SpecProbeFreshnessTest` is 7 pass / 3 fail, so the gate still rejects fabricated-future pins and failed reads.
- FINDING: re-deriving the contract under `2026-07-28` moves nothing on its own. Both revisions require only `name` and `inputSchema` on a `Tool`; everything else is optional. The regenerated minimal contract is byte-identical to the one built under `2025-06-18`. That is exactly why a minimal derived `tools/list` is a **floor**, not a figure any real server publishes.
- ANOMALY RESOLVED: the 471-vs-535 margin used to contradict `SKILL.md` was 64 tokens, 12%, and it **inverts**. Adding the `outputSchema` the current revision's structured-content surface expects — derived from the same `schema()` field list, so no information the minimal profile was denied — gives 676 vs 535. Both profiles are now published and the renderer states the inversion; neither direction is claimed. What survives on every profile and both contracts: the repository's "~2,000–3,000 tokens of dead weight" claim is unsupported, the largest figure measured anywhere being 1,774 for a nine-tool contract.
- SCOPE: `.research/layer-ladder/` — gates 4 → 7, narrowing mutants 9 → 17 (17 killed, 0 survivors), Python tests 32 → 63. `.research/260924_layer-ladder-measurement.md` corrected. STATUS: handed to review.

### 0430 — pipeline transport owns canonical rendering and one sealed response ledger
- IMPLEMENTATION: `QueryResponseRequest` stages each accepted native statement, measures its complete JSON/compact response against sealed `S = M - R`, commits atomically, and retains charged usage across retry, renewal, resume, and recovery before one matching publication.
- RENDERING: grouped compact output emits deterministic `@group` blocks from the accepted native `GroupResult`; the canonical JSON token is CSV-framed exactly once, projection order is retained for items, and empty group pages emit no blocks.
- TERMINAL PATH: the 12-row fixed redacted error registry is the only terminal publication domain; error records are checked against both the request reserve and the whole-response cap, while the compatibility native one-shot API retains its prior `nil,error` refusal shape.
- GATES: `TestQueryModelTypedGroupKeyEncoding` derives `18/18` mode cases plus `1/1` lexical coverage from the frozen vector; `TestQueryModelResponseBudgetAccounting` derives `6/6` surfaces, `13/13` cases, `8/8` refusals, `5/5` lifecycle attempts, `12/12` terminal rows, and `2/2` modes. Narrowing mutants for reserve omission/double subtraction, continuation reset, stringify/offset keys, and independent HTML/solidus/supplementary compact spellings all fail the named gates.
- REVIEW REWORK: independent review rejected unwrapped single-statement framing, publication-time re-marshalling, helper-only error rows, count-only matrices, and incomplete mutant classes. The request API now stages one top-level statement array into `committedWire`, publishes those exact bytes without re-marshalling, derives case IDs from the frozen vector, drives the grouped owner example in both modes, and executes all 12 error rows × 2 modes through public begin/dispatch/publish at statement 16 and maximum safe position. Added package-default, compact-counter, partial-commit, fallback/truncation, separate-frame, private-pause, real-481-envelope, and re-marshalling mutants.

### 0315 — response accounting revision 4 makes continuation pause production-reachable
- REGRESSION: revision 3 declared `initialPausesAfterCommittedStatement` in the vector fixture but exposed no production event or public state that caused and proved the pause; a test-only private pause bit could satisfy the lifecycle matrix.
- DECISION: each `QueryResponseRequest` dispatch executes exactly one next preflighted statement and returns `QueryResponsePaused` only after committing that statement when another remains, otherwise `QueryResponseTerminal`. Retry, renewal, resume, and recovery require the immediately preceding public paused return and retain the exact sealed Schema captured at begin time.
- GATE: `TestQueryModelResponseBudgetAccounting` now asserts `4/4` public paused returns before continuation and `4/4` terminal returns after it. The tenth narrowing mutant self-sets a private pause bit while production returns terminal; the named test must fail before continuation.
- SCOPE: no new board element, research, harness, diagram, grammar rule, group-key encoding, operation capability, limit range, or semver change was introduced. The frozen declaration ratio is `156/156`; response cases/refusals remain `13/13` and `8/8`.

### 0145 — response accounting revision 3 gives continuation one observable response owner
- REGRESSION: revision 2 retained one ledger but each continuation dispatch returned a separately closed response, so a caller could duplicate the accepted prefix or append bytes after JSON closure while the accounting matrix stayed green.
- DECISION: request dispatch methods now expose no native value or bytes. One opaque request owns an unpublished cumulative buffer, and exactly one matching publish method closes framing and exposes the response; premature, repeated, cross-family, and post-publication paths refuse.
- GATE: B05 drives initial plus retry/renewal/resume/recovery through public request methods, then consumes one actual publication per request and asserts `5/5` parseable responses, one prefix occurrence, at most 8,192 bytes, and zero bytes after closing framing. The ninth narrowing mutant preserves the ledger but returns an independently framed continuation response.
- SCOPE: no new task, harness, grammar, group-key encoding, operation capability, limit range, or semver change was introduced. The frozen declaration ratio is now `153/153`, with `8/8` response refusal rows.

### 0110 — response accounting revision 2 closes production-carrier and real-envelope gaps
- REGRESSION: revision 1 pre-seeded `committedSuccessBytes` for lifecycle labels and bounded only synthetic 480/481 error bytes; a real continuation could allocate a fresh ledger and a permitted real envelope could exceed the claimed reserve while all prior vectors stayed green.
- DECISION: opaque `QueryResponseRequest` is the production request owner. One-shot APIs remain wrappers for begin plus initial dispatch; retry, renewal, resume, and recovery run through public request methods that retain fixed model/access/limits/mode and charged usage.
- ENVELOPE: the terminal response registry is exhaustive at 12/12 code/message rows. Production encoding at statement/position maxima derives 158-byte JSON and 85-byte compact records, or 160/87 bytes with maximum framing, below the 480-byte minimum reserve. Setup-only messages are excluded because no response request exists yet.
- GATE: `TestQueryModelResponseBudgetAccounting` now derives 6/6 surfaces, 13/13 cases, 7/7 refusal rows, 5/5 lifecycle attempts, 12/12 real envelope rows, and 2/2 modes. New mutants allocate a fresh ledger only on real continuation branches or widen one permitted real envelope to 481 bytes.
- SCOPE: this focused revision preserves grammar, group-key encoding, operation capabilities, individual limit ranges, semver, board decomposition, and the separate synthetic publication-boundary test.

### 0055 — response accounting closes revision-5 F3
- DECISION: the repository carries forward revision-3 response accounting as `S = MaxResponseBytes - ErrorFramingReserveBytes`; success never borrows the reserve, and one sealed request ledger is shared by native canonical-JSON measurement plus exact JSON/compact wire rendering.
- ATOMICITY: `result_limit` latches on the first attempted byte whose complete statement would cross `S`. The current statement remains entirely scratch, earlier successful batch records stay charged, and one fixed redacted typed error must keep the whole response at or below `MaxResponseBytes`.
- PRODUCTION GATE: `TestQueryModelResponseBudgetAccounting` derives `5/5` surfaces, `12/12` cases, `7/7` refusal rows, and `5/5` lifecycle attempts from the machine resource. Required mutants ignore or double-count the reserve, drift by mode/default, expose a partial statement, fall back after accounting failure, reset continuations, or publish an over-cap error.
- SCOPE: this is the third and last loop-mandated architecture surface. It changes only response/error-reserve clauses, vectors, and handoff evidence; grammar, group-key encoding, operation capabilities, accepted limit ranges, `SetQueryLimits` sealing, semver, board size, and research depth remain unchanged.

## 2026-09-23

### 2407 — group-key revision 3 closes solidus and supplementary-plane escape bypasses
- REGRESSION: revision 2 covered HTML-escape divergence but its only ordinary non-ASCII witness was BMP `é`, and it contained no `/`; compact-only `\/` or UTF-16 surrogate-pair spellings could survive while preserving the decoded value.
- DECISION: the existing `string_escape_sensitive` case now includes `/` and U+1F600. The frozen raw token leaves `/` unescaped and emits U+1F600 as original UTF-8; the compact header carries those same bytes through exactly one CSV framing pass.
- PRODUCTION GATE: `TestQueryModelTypedGroupKeyEncoding` derives nine cases × two modes as `18/18`, and explicit compact-only solidus and supplementary-plane mutants must both fail the raw-token/exact-header assertions.
- SCOPE: this closes revision 2/F1 without changing the shared-token architecture, board decomposition, research depth, sibling capability/discovery work, or response accounting.

### 2359 — group-key revision 2 freezes lexical JSON string bytes
- REGRESSION: revision 1 fixed logical types but allowed JSON and compact to choose different JSON-equivalent string escapes; ASCII-only Q06 rows could not detect the renderer-mode bypass.
- DECISION: canonical string tokens are Go `encoding/json.Marshal` output with default HTML escaping. The contract now fixes quote/backslash, short and generic C0, ordinary UTF-8, U+2028/U+2029, HTML-sensitive, and solidus behavior and publishes it through `resultShapes`.
- PRODUCTION GATE: Q06 adds one escape-sensitive string case and compares raw JSON token bytes before decoding plus exact compact bytes after one CSV pass. Coverage is derived as nine cases by two modes (`18/18`) with lexical escape coverage `1/1`; the `SetEscapeHTML(false)` split-renderer mutant must fail.
- SCOPE: this closes review revision 1/F1 in the existing spec/vector/handoff artifacts. No implementation code, board element, research prerequisite, diagram, operation-capability, or response-accounting change is introduced.

### 2355 — revision 9 freezes canonical typed group keys across both renderers
- DECISION: grouped JSON preserves native scalar types: strings/identifiers/enums/timestamps are JSON strings, `int64` is a JSON number, Boolean is a JSON Boolean, and null/missing omit `value`. Compact carries the same canonical JSON token as its third logical CSV cell.
- TIMESTAMP: equal absolute instants coalesce before rendering; the only wire representative is UTC RFC3339Nano, so an arbitrary source offset cannot survive into JSON or compact output.
- PRODUCTION GATE: `TestQueryModelTypedGroupKeyEncoding` drives `Schema.QueryModelJSONASTWithMode` for eight key variants in two modes (`16/16`) and reproduces all discovery rows (`8/8`). Narrowing mutants stringify present keys, preserve a source timestamp offset, or split compact conversion from the shared canonical token.
- REFUSAL: the `7/7` boundary inventory covers arrays, secret-bearing registration, unauthorized keys, unsupported kinds, a second criterion, noncanonical timestamp representatives, and renderer divergence. The existing board remains the smallest proportional decomposition; no new task, research prerequisite, diagram, or generic renderer harness is justified.

### 2325 — revision 8 defeats the standard-name hybrid capability bypass
- REGRESSION: revision 7's conventional fixtures let a hybrid implementation hard-code `list` as full and `count` as where-only while consulting registered capability only for other names; all prior `20/20` cases could remain green.
- ADVERSARY: isolated production-entry cases now register `list` with `where` only and require `sortOrder` refusal before field resolution/load, then register `count` with `where+sortOrder` and require `sortOrder` admission with one bounded load. The exact hybrid mutant fails both counterfactuals.
- GATE: operation-capability coverage remains `5/5` surfaces and expands to `22/22` executable cases (`2+12+1+4+3`) without removing or weakening the prior 20 cases. Group-key and response-accounting ownership remain with their split sibling leaves.

### 2245 — revision 7 turns operation capability coverage into production evidence
- REGRESSION: revision 6 reported five umbrella surfaces but omitted count `groupBy`/`skip`/`take`, lowering aliases, and unregistered retry/resume/recovery attempts; the operation-capability gate now reports `5/5` surfaces and `20/20` executable cases.
- PRODUCTION ENTRY: `O01_full_list_capability` now drives `Schema.QueryModelAST`, performs one bounded load, and returns a grouped result whose rows and shape witness all six registered clauses. The prior zero-load success claim was helper-only evidence and is superseded.
- REFUSAL: where-only `count` independently rejects all five disallowed canonical clauses and all three owned lowering aliases before field resolution/load; unknown and unauthorized fields share the fixed non-enumerating error after capability admission.
- ADVERSARY: initial, retry, resume, and recovery attempts for an unregistered operation all reuse host-owned sealed registration state. Named narrowing mutants cover modifier/alias bypass and capability manufacture only on lifecycle continuation.

### 2140 — revision 6 splits repeated architecture rework by catalog surface
- LOOP RESPONSE: revision 5 was the fifth rejected wide candidate, so the architecture leaf is split into operation capability/discovery (`TASK-260923-37ss7m`), typed group-key encoding (`TASK-260923-2vs2fb`), and response-budget accounting (`TASK-260923-naqctx`). The three leaves are serialized and reuse the existing spec/vector/handoff artifacts; no research or generic oracle is added.
- DECISION: canonical query eligibility is an additive sealed `QueryOperationCapability` catalog keyed by registered operation and an explicit six-value `QueryClause` set. `list` and `count` are fixture registrations, never compiler name checks; an absent capability never means full capability.
- DISCOVERY: `Schema.QueryModelSchema(ctx, access)` is the only access-bearing query-model discovery entry. Legacy `schema()` remains unchanged because it has no `QueryAccess`; authenticated transports may attach the returned object under `queryModel`.
- REGRESSION: `TestQueryModelOperationCapabilityAndDiscovery` owns the `5/5` surface map and must kill hard-coded-operation, absent-as-full, and unfiltered-discovery narrowing mutants through production entries.

### 1837 — architecture revision 5 closes full-table limit and authorization findings
- REGRESSION: revision 4 bound one limit digest to nine stages but attacked only six; `TestQueryModelEffectiveLimitBinding` now owns a `10/10` state matrix and production-entry witnesses that make sorting, grouping, and pagination consume lowered sealed limits.
- REGRESSION: the registration proof covered exact `secret + Groupable` only; its `3/3` matrix now also refuses zero and unknown sensitivity atomically, with a mutant that defaults either value to public.
- REGRESSION: the portable fixture omitted ordinary visibility/access setup; all `12/12` fields now declare visibility and sensitivity, all `27/27` AST vectors resolve the normal fixture policy that admits only `public`, and `TestQueryModelFixtureValidity` refuses omissions before vector execution.
- DECISION: `ErrorFramingReserveBytes <= MaxResponseBytes` is a derived invariant under the frozen individual ranges, not an independently reachable refusal branch. The rev4 cross-field rejection claim is superseded.
- SCOPE: all three rev4 findings close inside the existing architecture, predicate, and pipeline leaves. No new task, research prerequisite, generic oracle, diagram, or unresolved product choice is justified.

### 2015 — architecture revision 4 closes limit-binding and secret-group bypasses
- REGRESSION: revision 3 exposed parser defaults but no host route for lowering execution-only ceilings; `Schema.SetQueryLimits` / `Schema.QueryLimits` now normalize zero/partial values, refuse raises, seal one digest, and bind parse, compile, bounded load, evaluation, grouping, rendering, and discovery.
- REGRESSION: revision 3 treated secret-bearing group refusal as prose around independent `Visibility` and `Groupable` fields; `FieldSensitivity` is now explicit and `RegisterQueryField` atomically rejects `secret + Groupable` even when the caller is authorized for the visibility label.
- GATE: `TestQueryModelEffectiveLimitBinding` owns the lowered snapshot `N`/`N+1` path and kills the `2 -> 1` narrowing mutant; `TestQueryModelSecretGroupRegistrationRefusal` drives the real registration entry and kills the visibility-empty-only narrowing mutant.
- SCOPE: the frozen registry is `131/131` with 35 unique vectors. Both findings belong to the existing predicate and pipeline slices; no new leaf, research prerequisite, diagram, or unresolved product choice is justified.

### 1845 — architecture revision 3 freezes immutable collision and legacy grammar gates
- REGRESSION: revision 2 derived the public-name collision check from the future implemented package, so every correct implementation would collide with its own declarations; revision 3 freezes the 115-name source-derived baseline at commit `c5a6fb45` / package tree `a00d35c` and keeps exact implemented-surface checking separate.
- REGRESSION: revision 2 narrowed the existing batch parser by requiring arguments and exactly one separator; the grammar now preserves zero arguments plus leading, repeated, and trailing semicolons and fixes signed/unsigned decimal lexemes and overflow errors.
- GATE: `TestQueryModelLegacyBatchGrammarCompatibility` owns G01-G04 and must kill the single-semicolon/no-edge-semicolon mutant; `TestQueryModelFrozenPublicSurfaceNoPackageScopeCollision` must kill the `QuerySortDirection` to `SortDirection` mutant against the immutable baseline.
- EVIDENCE: candidate whitespace validation uses a temporary Git index populated with all tracked and untracked paths, because ordinary `git diff --check` does not inspect untracked `.spec/` files.

### 1715 — architecture revision 2 closes public-surface freeze defects
- REGRESSION: revision 1 proposed `SortDirection string`, colliding with production's exported `SortDirection int`; the additive type is now `QuerySortDirection`, leaving `SortDirection`, `Asc`, `Desc`, and `SortSpec` unchanged.
- DECISION: the machine contract is exhaustive for the next slices: 27 grammar productions, 24 public types, 33 constants, 23 functions, and 5 methods (`112/112`), plus explicit textual-form-to-AST mappings.
- GATE: `TestQueryModelFrozenPublicSurfaceNoPackageScopeCollision` must derive current package scope and fail the narrowing mutant that substitutes `SortDirection` for `QuerySortDirection`; the architecture probe reports zero real collisions and catches the mutant.
- SCOPE: no new task or unresolved product choice was found; the existing predicate, pipeline, and integration/release leaves remain the proportional critical path.

### 1605 — composable query model freezes one compiler and pipeline
- MILESTONE: `TASK-260923-37ss7m` materialized `.spec/composable-query-expressions.md` and its machine-checkable vector set for independent review.
- FINDING: current `Statement`/`Arg` structs, legacy `FilterableField` EqualFold behavior, and helper-owned sort/pagination cannot be widened in place without either breaking unkeyed Go literals or retaining divergent evaluators.
- DECISION: add `QueryModel` beside the legacy AST; lower compatible flat filters and `sort_FIELD`/`skip`/`take` aliases into one compiler; keep old exported structs/helpers unchanged. The public DSL uses recursive `not`, `satisfiesAll`, and `satisfiesAny` nodes.
- DECISION: Go `regexp`/RE2 matching is compile-once substring search with a 1024-byte per-statement pattern budget. One scalar `groupBy` runs after deterministic row sorting; `skip`/`take` page groups when grouping is present.
- SCOPE: the prior rev3 no-regex restriction and missing `groupBy` are superseded. No additional research/oracle task or architecture diagram is justified; the existing two runtime slices are the proportional decomposition.
- ANOMALY: board-wide `task-board validate` reports 141 historical issues (legacy missing activity streams, old container links/status mismatches, uncommitted done lanes, and two unreadable foreign run ledgers). None names `STORY-260908-3ieod5` or its four tasks; task-scoped projections, dependencies, and resources are internally consistent. This task does not mutate unrelated control-plane history.

## 2026-09-23

### 1205 — Article now separates alias economics from MCP economics
- FINDING: the revised [field-alias article](articles/field-alias-compression-study.md) labels fixture-backed token counts as historical measurements, session output as a model estimate, and the current formatter/batching behavior as implementation evidence.
- DECISION: retain the field-alias NO-GO only for this schema-once formatter. State no MCP token winner because the repository has no adapter, tool-definition fixture, prompt snapshot, host trace, or aligned workload.
- EVIDENCE: `.research/260923_mcp-token-economics-evidence.md`; current `go test ./...` runs passed in `agentquery/` and `example/`; the tokenizer command was expected-red because `tiktoken` is not installed. No package or benchmark fixture was added.
- CAVEAT: the README article-index summary now carries the same evidence boundary. The broader README overview and SKILL.md retain stale fixed MCP-overhead, no-batching, and break-even claims outside this article-focused slice; correct them in a separate documentation task.

### 1045 — MCP token comparison needs a bounded correction
- FINDING: `references/comparison-example.md` asserts an `internal/fields`-backed MCP implementation, fixed 2,200-token definition cost, no batching, and a ~293-query break-even; this checkout has no MCP adapter or `internal/fields` package. A Go-source search finds only `assets/field-selector.go:4`, which explicitly describes a future MCP server.
- DECISION: revise `articles/field-alias-compression-study.md` as the canonical publication source. Keep proven DSL batching/schema-once formatter facts, but mark MCP token economics unknown without an aligned server, host, protocol/version, definitions, and tokenizer transcript.
- EVIDENCE: `.research/260923_mcp-token-economics-evidence.md`; `go test ./...` from `agentquery/` exited 0. Official MCP SDK documentation shows batching/version behavior is protocol- and SDK-dependent, so the current universal no-batching wording is stale.

## 2026-09-06

### 1520 — gofmt drift already on main at 656ad0a
- FINDING: `gofmt -l agentquery` reported `agentquery/types.go` at base commit `656ad0a`, before any of this task's changes. Comment-alignment drift in `ParameterDef`, `SortDirection` and `MutationContext`.
- SCOPE: `agentquery/types.go:57`, `agentquery/types.go:88`, `agentquery/types.go:118`.
- DECISION: fixed with `gofmt -w` under TASK-260906-1auqyl so the module's gofmt gate is green. Whitespace only; the exported API is unchanged.
- NOTE: no formatting gate runs in CI (no CI exists), which is how the drift reached main.

### 1515 — DSL error-guidance delta adopted into agentquery
- MILESTONE: the BUG-260903-12avv2 delta (7 files, 469/37) applied onto `656ad0a` and validated in this repo. Adds `Grammar`/`DSLGrammar()`/`GrammarSyntax()` to `schema()`, and `NewUnknownOperationError`/`IsUnknownOperation`/`ParseError.Operation`/`OperationPos`.
- FINDING: the exported board patch is byte-identical to the producer worktree state, so the resource and the worktree were one delta, not two variants.
- DECISION: unknown operation outranks a positional argument error. `attributeTokenizeError` (`agentquery/parser.go:49`) walks the tokens emitted before a tokenizer failure so `items(status in [a,b])` reports the unknown operation and the known-operation list instead of `unexpected character "["`.
- FINDING: the grammar text is derived from the tokenizer token table and escape map rather than restated, so it cannot drift. Pinned by `agentquery/grammar_test.go`; 9 narrowing mutants, 9 killed, no survivors.
- STATUS: handed to review; landing on main and tagging `agentquery/v1.6.0` is the orchestrator's step.
# 2026-09-24 — TASK-260923-3hhfe2 predicate runtime slice

- IMPLEMENTATION: added the frozen 156-declaration composable-query public surface, recursive parser/renderer, normalized sealed limit digest, typed field and operation-capability catalogs, access-bearing discovery, bounded snapshot compilation, one three-valued evaluator, compile-once Go/RE2 matching, legacy flat-filter lowering, and opaque request lifecycle.
- SAFETY: authorization and capability admission complete before field resolution or snapshot loading; external AST cycles/bad unions, regex syntax and byte bounds, snapshot N+1, digest mismatch, hidden unreachable branches, invalid sensitivity, and secret groupability refuse through production entries.
- SCOPE BOUND: modifier clauses are parsed, rendered, capability-checked, and field-authorized but execution returns `predicate_unsupported` before load. Sorting, grouping, pagination, typed group rendering, and response-budget publication remain with `TASK-260923-s86r49`; this leaf does not silently implement or ignore that sibling's runtime.
- VALIDATION: `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, `git diff --check`, and frozen vector JSON validation passed. Narrowing mutants for regex 1024→1023, snapshot 2→1, secret-group visibility, absent-as-full capability, lowering-alias bypass, public-name collision, and legacy separator grammar were killed by named tests.

## Revision 2 — adversarial review rework

- SECURITY: canonical projection now renders exclusively from the validated typed materialization. A missing or divergent legacy selector can no longer fall back to `[]T` or disclose unrequested raw fields; `TestQueryModelProjectionUsesTypedMaterialization` kills the selector-presence narrowing mutant.
- COMPATIBILITY: a named argument lowers to `legacyEqualsFold` only when the host registered the field through both `FilterableField` and `RegisterQueryField`. A same-named typed field alone remains an operation argument and cannot silently enter the canonical predicate.
- CONTRACT: `TestQueryModelFrozenContractCompleteness` now checks type shape/fields/tags, underlying types, generic parameters, interface methods, function and method signatures, and constant declared types/values. The token-preserving `QueryResponsePaused = "mutated-paused"` mutant and a behavior-changing parser mutant that retains the `where` token both fail the named gate.
- COVERAGE: the field/operator matrix is derived from the frozen vector and executes `57/57` allowed rows through `Schema.QueryModelAST`, plus every forbidden kind/operator pair and a wrong-literal case for each of the seven kinds. Full AND/OR/NOT three-valued tables are observed through selection and negated-selection production queries.
- CAPABILITY: the named gate now executes 21 predicate-slice cases, including all count canonical/alias refusals, four unregistered lifecycle replays, three discovery access policies, custom opt-in, and both standard-name counterfactuals. The exact hybrid `list`/`count` mutant fails. The remaining frozen full-list modifier-runtime case stays explicitly owned by `TASK-260923-s86r49`; this leaf continues to refuse it before load rather than ignore it or duplicate the pipeline.

## 2026-09-24 — TASK-260923-3hhfe2 revision 4

- IMPLEMENTATION: the accepted architecture reallocation supersedes the earlier modifier scope bound. The single native executor now performs typed stable sort with identity tie-breaking, grouping, top-level skip/take, result bounds, and projection after one bounded load; transport byte accounting remains in `TASK-260923-s86r49`.
- REGRESSION: `TestQueryModelFrozenContractCompleteness` executes an independently keyed witness for every frozen grammar production (`41/41`) and rejects token-preserving empty `satisfiesAll`/`satisfiesAny` branches.
- REGRESSION: `TestQueryModelFieldValueTaggedUnionRefusal` drives all `6/6` malformed scalar/array/presence payloads through `Schema.QueryModelAST`; the non-present-payload narrowing mutant fails with four named subtests.
- CAPABILITY: `TestQueryModelOperationCapabilityAndDiscovery` now executes the frozen `22/22`, including O01's six-clause one-load result and real paused retry/resume/recovery continuations on the same opaque request. The absent-as-full mutant fails at O04.
### 2026-09-24 — revision 5 closes capability discovery and lifecycle bypasses
- REGRESSION: `TestQueryModelOperationCapabilityAndDiscovery` now derives the frozen `5/5` surfaces and `22/22` case inventory, asserts the exact 11-key authorized discovery object and full eight-row `resultShapes` registry, refuses an empty capability catalog atomically, and compares the sealed capability inventory before/after retry, resume, and recovery.
- REGRESSION: `TestQueryModelDuplicatePagingAliasesRefusedBeforeLoad` drives duplicate `skip` and `take` aliases through `Schema.QueryModelAST`; both return `query_modifier` with zero snapshot loads.
- REGRESSION: `TestQueryModelOpaqueResponseLifecycle` and the O04 lifecycle matrix reject caller-forged continuation tags on fresh, paused, terminal, and published requests while retaining the exhaustive public attempt set.
- REGRESSION: `TestQueryModelRendererExternalASTRefusals` drives operation, argument, projection, predicate, sort, group, direction, control-byte, and identifier validation through `RenderQueryModel` before any invalid grammar is emitted.
- GATE: `TestQueryModelEffectiveLimitBinding` derives and executes the frozen `10/10` state matrix, including zero-default and 8191-byte atomic-refusal rows; the unreachable response/reserve cross-field branch is removed.
- ADVERSARY: narrowing mutants for unfiltered discovery, absent-as-full discovery, continuation-only capability manufacture, duplicate-take admission, forged non-empty attempt admission, token-preserving `sideways` rendering, and the 8192→8191 minimum each redden their named production-entry test.

### 2026-09-24 — revision 6 closes typed stored-data invariants

- REGRESSION: `TestQueryModelStoredEnumDomainAndScalarIdentityRefusal` drives `Schema.QueryModelAST` for rogue scalar-enum grouping/projection and rogue enum-array materialization. Both return atomic `predicate_data` after one bounded load; coverage is `2/2` stored shapes.
- REGRESSION: `Schema.RegisterQueryField` now refuses `Identity: true` on every `FieldScalarArray` before catalog insertion. The named test observes both the `predicate_type` registration refusal and the later `predicate_field` production refusal with zero loads, covering `2/2` public surfaces.
- ADVERSARY: a gate-preserving mutant that admits only the unregistered value `rogue` fails both enum subtests; a second gate-preserving mutant that admits only `scalarArray<identifier>` identity fails the registration subtest. Both expected-red runs exit 1, and the source is restored to its pre-mutant SHA-256 before green validation.

### 2026-09-24 — revision 7 closes inner scalar tagged-union bypass

- REGRESSION: `TestQueryModelFieldValueTaggedUnionRefusal` now drives `Schema.QueryModelAST` across all 36 non-zero inactive inner payload combinations (`18/18` scalar and `18/18` array-element), 12 valid controls, and the exact duplicate identifier bypass from review revision 6. Every malformed value returns atomic `predicate_data` after the bounded load.
- SECURITY: `validateFieldValue` validates the complete inner `ScalarValue` union before use, and identity/group keys are derived only from the active typed member. Two logical identifier values `"A"` can no longer be made distinct with hidden inactive payload bytes.
- ADVERSARY: a gate-preserving narrowing mutant admits only inactive `Int64` on `identifier`; the named scalar and array-element subtests fail with expected-red exit 1. The active-only identity key independently continues to reject the duplicate-id probe.

## 2026-09-24 — TASK-260923-s86r49 response transport

- IMPLEMENTATION: one request-owned ledger now stages each native result exactly once, renders grouped and ungrouped canonical JSON/compact output, carries the sealed schema/digest/mode across continuations, and permits exactly one terminal publication.
- ACCOUNTING: statement success uses `MaxResponseBytes - ErrorFramingReserveBytes`; later failures are withheld atomically, while the fixed 12-row terminal registry is checked against both reserve and whole-response bounds.
- SECURITY: trusted terminal source positions now require a private digest of the unchanged parsed AST. Directly constructed positions and exported positions mutated after `ParseQueryModel` are omitted; the parsed-position-integrity narrowing mutant publishes the forged maximum and fails the named lifecycle test.
- EVIDENCE: production-entry ratios are 18/18 typed mode cases plus 1/1 lexical case, 6/6 surfaces, 13/13 accounting cases, 8/8 refusals, 5/5 lifecycle attempts, 12/12 terminal rows, and 2/2 modes. All 22 narrowing mutants are killed; full, race, vet, build, and whitespace gates exit 0.

### Revision 2 — seal live execution dependencies across continuations

- REGRESSION: revision 1 retained the original `Schema` pointer and limit digest but dereferenced its replaceable snapshot loader and operation map on each statement. A public setter call after `QueryResponsePaused` could therefore publish one response assembled from original and replacement provider state.
- FIX: `CompileQueryModel` now captures the bounded snapshot loader, operation handler, and selector used by every compiled statement. Retry, renewal, resume, and recovery execute only those request-owned dependencies; compatibility setters still configure later requests.
- GATE: external-package `TestQueryModelContinuationUsesSealedExecutionDependencies` drives `BeginQueryModelResponse`, observes the public pause, replaces both loader and handler through exported APIs, and proves `4/4` continuation labels publish only original state. A narrowing mutant that keeps the pointer/digest guard but reads only the live loader fails all four named subtests.

### Revision 3 — seal caller-owned query model aliases

- REGRESSION: revision 2 still shallow-copied `QueryStatement`, so caller-owned `Take`, `Skip`, `GroupBy`, `Call.Args`, `Call.Fields`, `SortOrder`, and nested predicate storage remained reachable after a public pause. The reviewer reproduced it by changing the second statement from `take 1` to `take 2` before resume.
- FIX: `CompileQueryModel` now deep-clones the complete execution-relevant statement and expression graph before validation and compilation. The clone preserves internal expression graph topology for ordinary validation while retaining no caller-owned pointer or slice backing array.
- GATE: external-package `TestQueryModelContinuationUsesSealedCallerModel` observes `QueryResponsePaused`, mutates the caller's second-statement `Take`, and proves all `4/4` continuation labels publish the sealed one-row result. A narrowing mutant that preserves every schema/digest/loader/handler guard but aliases only `Take` fails all four named subtests with exit 1.

### Revision 4 — bound external-AST sealing before clone and digest

- REGRESSION: revision 3 deep-cloned and JSON-digested the complete caller-owned expression graph before the compiler reached `MaxExpressionNodes`. A request certain to refuse at node 129 could therefore force allocation and traversal proportional to an arbitrary rejected remainder.
- FIX: `CompileQueryModel` now seals and validates external statements in one bounded traversal before parsed-position digesting. Statement, sort, finite-set, expression-depth, expression-node, literal, and aggregate caller-model source budgets are checked before caller-sized allocation; cycles remain `predicate_syntax`, shared acyclic subtrees still count once per occurrence, and the accepted graph remains request-owned.
- GATE: `TestQueryModelExternalASTSealingIsBoundedByExpressionLimit` drives `Schema.BeginQueryModelResponse` with 129 and 1,000,001-node shapes. Both refuse with `predicate_limit`, zero snapshot loads, and equal measured compile allocation (`296B` in the green run). A narrowing mutant that retains the node-limit check but preallocates/traverses all children first fails the named test with `88,003,936B` versus `11,688B` and exit 1.

### Revision 5 — enforce frozen grammar at the public external-AST entry

- REGRESSION: revision 4 applied lexical and token validation only in `RenderQueryModel`; a caller-built model containing positional `Arg{Value: "bad\rvalue"}` bypassed that helper and reached the loader, handler, and successful publication through `Schema.QueryModelJSONASTWithMode`.
- FIX: the bounded sealing traversal now validates operation, argument, projection, sort, group, predicate-field, predicate-shape, and literal grammar after their sealed size checks and before any loader or handler call. String and programmatic entries therefore accept the same frozen grammar without a second renderer or unbounded traversal.
- GATE: `TestQueryModelProductionEntryRefusesMalformedExternalAST` drives `11/11` malformed classes through `Schema.QueryModelJSONASTWithMode`; every row returns `predicate_syntax` with zero loader and handler calls. A narrowing mutant keeps argument validation but omits positional values; only `positional_argument_value_raw_c0` fails, with expected-red exit 1.

## 2026-09-24 — TASK-260908-3hhvl2 integration candidate

- INTEGRATION: `TestQueryModelComposablePipelineAcrossPublicRenderModes` drives the public `Schema.QueryModelJSONASTWithMode` entry through nested `satisfiesAll`/`satisfiesAny`/`not`, bounded regex, deterministic sort, grouping, group pagination, and projection in JSON and compact modes (`2/2`). Each mode performs one bounded snapshot load and zero legacy-handler evaluations.
- ADVERSARY: the first gate-preserving `not(true)` mutant survived because `take 2` hid the illegally admitted third group. Expanding the fixture to `take 3` made the same mutant fail both named mode cases with exit 1; production source was restored to its pre-mutant SHA-256 before green validation.
- DOCUMENTATION: README and skill guidance now publish the canonical example, fail-closed discovery route, group-level paging shape, and one-operation-at-a-time legacy migration. The skill authoring validator exposed the pre-existing unsupported `triggers` frontmatter key; it was removed and the validator then exited 0 with `Skill is valid!`.
- RELEASE BOUND: this producer run prepares an uncommitted `agentquery/v1.7.0` candidate only. Tag creation, push, checksums of the immutable tag object, and release publication remain refused until independent review accepts the candidate and the authoritative remote inventory is rechecked at publication time.
- REVIEW REWORK: CR revision 1 found that Go's permissive RFC3339 parser admitted offset hour `24`, offset minute `60`, and comma fractions despite the frozen lexical subset. The shared timestamp gate now validates exact ASCII structure, fraction width, clock bounds, offset bounds, and negative-zero offset before calendar parsing, so text and caller-constructed AST routes cannot diverge.
- GATE: `TestQueryModelStrictTimestampContract` drives `10/10` invalid timestamp classes through both `ParseQueryModel` and `Schema.QueryModelAST`, with zero snapshot loads on the external route and four nearby valid controls. Narrowing only the offset-minute ceiling from `59` to `60` makes the named `offset_minute_60` cases fail on both production entries with expected-red exit 1; the source was restored from a byte-identical copy before validation.
