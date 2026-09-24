# layer-ladder

Reproducible offline measurement of where an agent-facing API's token saving
actually comes from, one representation layer at a time.

Findings and their limits: [`../260924_layer-ladder-measurement.md`](../260924_layer-ladder-measurement.md).
Generated report: [`RESULTS.md`](RESULTS.md) and [`results.json`](results.json).

## What it measures

One scenario — a sprint stand-up roll-call needing 4 of a record's 8 fields —
rendered into six layers over identical rows, then tokenized with
`cl100k_base`. Each layer is measured against the layer below it, so
minification, projection, compact formatting and header aliases each get their
own denominator instead of sharing one aggregate percentage.

Two ladder orderings are reported (minify-then-project and
project-then-minify). They reach the same payload and the same cumulative
saving, and attribute the credit very differently. That difference is the point.

## Layout

| Path | What |
|---|---|
| `scenario.go` | scenario definition, fixture loading, the agentquery schema |
| `ladder.go` | layer contract, renderers, decoders, and the two gates |
| `ladder_test.go` | Go suite: preservation and alias gates, positive controls |
| `cmd/genfixtures` | renders every layer through the real agentquery entry points |
| `measure.py` | tokenizes, validates, computes ladders and amortization, writes the report |
| `tests/test_measure.py` | Python suite: manifest, arithmetic and MCP-contract gates |
| `mcp/build_contract.py` | derives MCP tool contracts from the real `schema()` output, in a `minimal` and a `structured` profile |
| `mcp/spec-probe.json` | dated primary-source read of which MCP revision is current, with an absent-revision control |
| `mutants/run.py` | narrowing-mutant harness |
| `fixtures/` | rendered payloads, `schema()` responses, `manifest.json` |

Input data is `../synthetic-payloads/json-{5,20,100,500}.txt`, unchanged.

## Running

```bash
go run ./cmd/genfixtures
/usr/bin/python3 mcp/build_contract.py
/usr/bin/python3 measure.py
```

`measure.py` needs `tiktoken`. On this workstation only `/usr/bin/python3`
carries it (0.14.0); Homebrew's `python3` does not.

## Checks

```bash
go test ./... -count=1
/usr/bin/python3 -m unittest discover -s tests
/usr/bin/python3 mutants/run.py
```

### Gates

Seven gates stand between a tampered fixture and a published number:

| Gate | Where | Refuses |
|---|---|---|
| preservation | `ladder.go` `VerifyPreservation` | a layer whose decoded records differ from the canonical ones in count, order, field set or value |
| alias | `ladder.go` `ApplyAliasHeader` | an alias header that drops a column, collapses two columns onto one alias, or does not shorten |
| manifest | `measure.py` `validate_manifest` | a missing or resized fixture, a missing layer, and any scale whose layers disagree about which records they carry |
| contract labelling | `measure.py` `validate_mcp_contract` | an MCP section that omits its evidence class, protocol revision, primary source, spec probe or limitations — or claims to be a host measurement |
| ladder closure | `measure.py` `reconcile_ladder` | a ladder whose per-step savings do not close exactly against the base and final rungs |
| percentage denominator | `measure.py` `reconcile_percentages` | a published percentage not denominated in the rung its column header names — the token counts can all be right and the column still wrong |
| amortization attribution | `measure.py` `validate_amortization` | a transition charged a one-time artifact its source rung already held, which is sunk on both sides and cancels |
| spec freshness | `measure.py` `validate_spec_probe` | a contract pinned to a superseded protocol revision, and a probe whose absent-revision control resolved or failed to read |

`reconcile_ladder` has no rounding tolerance, on purpose: a tolerance lets a
dropped rung hide as rounding. `validate_spec_probe` exists because a shape gate
can require a revision to be *present* but cannot know whether it is *current*;
freshness has to rest on a dated read, and that read needs a control so a failed
read is never mistaken for an absence.

### Mutants

`mutants/run.py` copies the checkout, weakens one gate per mutant so it admits
exactly one member of the class it must reject, and requires a named test to
fail. Deleting a gate would only prove the gate exists; narrowing it proves the
bound is tested. All seventeen mutants must be killed.

M10-M17 cover the gates added after review: the percentage denominator handed to
`pct` inside `build_ladder` (M10), `reconcile_percentages` step coverage at the
first and last step (M11, M12), the sunk-cost cancellation in
`incremental_session_artifacts` (M13), `validate_amortization` entry coverage
(M14), spec freshness and its control (M15, M16), and MCP profile coverage
(M17). M15 and M16 are narrowings, not deletions: under both, the gate still
rejects a fabricated future revision, a failed control read and an undated
probe, and admits only the stale-pin and resolved-control classes.
