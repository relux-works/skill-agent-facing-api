#!/usr/bin/env python3
"""Measure the token cost of every layer of one task scenario's response.

Every layer carries the same scenario records; only the field set and the
serialization change. The point of the ladder is that each rung is measured
against the rung below it, with its own denominator, instead of attributing one
aggregate saving to whichever technique the article happens to be selling.

Input:  fixtures/manifest.json plus the fixtures the Go generator wrote.
Output: results.json and RESULTS.md next to this file.

Run from .research/layer-ladder:

    /usr/bin/python3 measure.py
    /usr/bin/python3 measure.py --check      # validate and compute, write nothing
"""

from __future__ import print_function

import argparse
import hashlib
import json
import math
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ENCODING_NAME = "cl100k_base"

# Ladder A minifies before it projects; ladder B projects before it minifies.
# They end at the same payload, so the cumulative saving is identical and the
# per-step attribution is not. Publishing only one of them silently credits one
# technique with the other's saving.
# Payload formats priced through the MCP envelope. Varying the format, not just
# the scale, is what exposes JSON-escaping cost inside a tool result.
MCP_PAYLOAD_LAYERS = ("L1", "L3", "L4")

LADDERS = {
    "A_minify_then_project": ["L0", "L1", "L3", "L4", "L5"],
    "B_project_then_minify": ["L0", "L2", "L3", "L4", "L5"],
}

# What an agent must already hold, as session-constant text, in order to USE a
# rung. Every rung of this ladder is agentquery output, so one schema()
# roundtrip is the prerequisite of all of them and is sunk on both sides of
# every projection and every format change. Only L5's alias legend is genuinely
# new at its own transition. Charging a sunk artifact to a transition inflates
# its break-even and prices a decision the agent never makes.
LAYER_SESSION_PREREQUISITES = {
    "L0": ("agentquery-schema-response",),
    "L1": ("agentquery-schema-response",),
    "L2": ("agentquery-schema-response",),
    "L3": ("agentquery-schema-response",),
    "L4": ("agentquery-schema-response",),
    "L5": ("agentquery-schema-response", "alias-legend"),
}


class MeasurementError(Exception):
    """A refusal: the inputs cannot support an honest measurement."""


# --- gates ------------------------------------------------------------------


def validate_manifest(manifest, root):
    """Refuse a fixture set that cannot support a same-data comparison.

    Rejects: a missing fixture file, a fixture whose bytes no longer match the
    manifest, a layer that is declared but not rendered at some scale, an
    unknown layer id, and — the important one — any scale where the layers
    disagree about which scenario records they carry.
    """
    declared = [layer["id"] for layer in manifest.get("layers", [])]
    if not declared:
        raise MeasurementError("manifest declares no layers")
    scales = manifest.get("scales") or []
    if not scales:
        raise MeasurementError("manifest declares no scales")

    by_scale = {}
    for entry in manifest.get("fixtures", []):
        layer = entry["layer"]
        if layer not in declared:
            raise MeasurementError("fixture references undeclared layer %r" % layer)
        by_scale.setdefault(entry["scale"], {})[layer] = entry

    for scale in scales:
        present = by_scale.get(scale)
        if not present:
            raise MeasurementError("scale %d has no fixtures" % scale)
        missing = [layer for layer in declared if layer not in present]
        if missing:
            raise MeasurementError("scale %d is missing layers %s" % (scale, ",".join(missing)))

        digests = {}
        for layer in declared:
            entry = present[layer]
            path = os.path.join(root, entry["path"])
            if not os.path.exists(path):
                raise MeasurementError("scale %d layer %s: fixture %s does not exist" % (scale, layer, entry["path"]))
            actual = os.path.getsize(path)
            if actual != entry["bytes"]:
                raise MeasurementError(
                    "scale %d layer %s: fixture is %d bytes, manifest records %d"
                    % (scale, layer, actual, entry["bytes"])
                )
            digests[layer] = entry["scenario_digest"]

        consensus = sorted(set(digests.values()))
        if len(consensus) != 1:
            offenders = ", ".join(
                "%s=%s" % (layer, digests[layer][:12]) for layer in declared
            )
            raise MeasurementError(
                "scale %d: layers do not carry the same scenario records (%s)" % (scale, offenders)
            )


def validate_spec_probe(probe, contract):
    """Refuse an MCP section pinned to a superseded protocol revision.

    A contract-shape gate can require `protocol_version` to be PRESENT; it
    cannot know whether the named revision is still current, so freshness has to
    be asserted against a dated primary-source read instead. mcp/spec-probe.json
    is that read.

    The probe carries its own control: a revision that cannot exist must come
    back 404. Without that control an outage that 404s every request would make
    every real revision look absent, and "absent" is not the same fact as "the
    read failed". A control that resolves means the site answers for anything
    and the probe proves nothing either way.
    """
    for field in ("probed_at", "latest_revision", "latest_revision_status",
                  "control_absent_revision", "control_absent_revision_status"):
        if not probe.get(field):
            raise MeasurementError("spec probe does not record %s" % field)
    if probe["control_absent_revision_status"] != 404:
        raise MeasurementError(
            "spec probe control %s returned %s, want 404; the probe cannot distinguish an "
            "absent revision from a failed read, so no revision claim may rest on it"
            % (probe["control_absent_revision"], probe["control_absent_revision_status"])
        )
    if probe["latest_revision_status"] != 200:
        raise MeasurementError(
            "spec probe read the latest revision %s as %s, not 200; the read failed and "
            "freshness is unknown" % (probe["latest_revision"], probe["latest_revision_status"])
        )
    pinned = contract.get("protocol_version")
    if pinned != probe["latest_revision"]:
        raise MeasurementError(
            "mcp contract is pinned to revision %r but the probe of %s resolves the current "
            "revision to %r" % (pinned, probe["probed_at"], probe["latest_revision"])
        )


def validate_mcp_contract(contract):
    """Refuse an MCP section that could be mistaken for a host measurement."""
    if contract.get("evidence_class") != "synthetic-contract":
        raise MeasurementError(
            "mcp contract must declare evidence_class=synthetic-contract, got %r"
            % contract.get("evidence_class")
        )
    if contract.get("host_measured") is not False:
        raise MeasurementError(
            "mcp contract claims host_measured=%r; nothing here measures a host"
            % contract.get("host_measured")
        )
    if not contract.get("protocol_version"):
        raise MeasurementError("mcp contract must name the protocol revision it follows")
    if not contract.get("protocol_source"):
        raise MeasurementError("mcp contract must cite the primary protocol source")
    if not contract.get("limitations"):
        raise MeasurementError("mcp contract must state its limitations")
    if not contract.get("contracts"):
        raise MeasurementError("mcp contract declares no tool contracts")
    if not contract.get("spec_probe"):
        raise MeasurementError("mcp contract must cite the dated spec probe its revision rests on")


def reconcile_ladder(steps, base_tokens, final_tokens):
    """Refuse a ladder whose per-step arithmetic does not close.

    Each step must be denominated in the rung below it, every denominator must
    be non-zero, and the steps must account for the whole base->final delta with
    no slack. A rounding tolerance here would let a lost or double-counted
    layer pass as a rounding artefact.
    """
    if base_tokens <= 0:
        raise MeasurementError("ladder base has %d tokens" % base_tokens)
    running = base_tokens
    for step in steps:
        if step["denominator_tokens"] <= 0:
            raise MeasurementError("step %s has denominator %d" % (step["step"], step["denominator_tokens"]))
        if step["denominator_tokens"] != running:
            raise MeasurementError(
                "step %s is denominated in %d tokens but the previous rung is %d"
                % (step["step"], step["denominator_tokens"], running)
            )
        running = running - step["incremental_saved_tokens"]
    if running != final_tokens:
        raise MeasurementError(
            "incremental savings sum to %d tokens, final rung is %d" % (running, final_tokens)
        )


def reconcile_percentages(steps, base_tokens):
    """Refuse a ladder whose published percentages are not denominated in the
    rung their own column header names.

    reconcile_ladder checks token arithmetic and nothing else. A percentage
    denominated in the ladder base, or in the after-value, leaves every token
    count correct, closes the ladder exactly, and still publishes a wrong
    column under a header that claims the previous rung. The percentage is the
    number the article quotes, so it needs its own gate tying it to its stated
    denominator.
    """
    if base_tokens <= 0:
        raise MeasurementError("ladder base has %d tokens" % base_tokens)
    if not steps:
        raise MeasurementError("ladder has no steps to check")
    for row in steps:
        denom = row["denominator_tokens"]
        if denom <= 0:
            raise MeasurementError("step %s has denominator %d" % (row["step"], denom))
        want = pct(row["incremental_saved_tokens"], denom)
        if row["incremental_saved_pct_of_previous"] != want:
            raise MeasurementError(
                "step %s publishes %s%% of the previous rung, but %d saved of %d is %s%%"
                % (row["step"], row["incremental_saved_pct_of_previous"],
                   row["incremental_saved_tokens"], denom, want)
            )
        want_cum = pct(row["cumulative_saved_tokens"], base_tokens)
        if row["cumulative_saved_pct_of_base"] != want_cum:
            raise MeasurementError(
                "step %s publishes %s%% of the base, but %d saved of %d is %s%%"
                % (row["step"], row["cumulative_saved_pct_of_base"],
                   row["cumulative_saved_tokens"], base_tokens, want_cum)
            )


def incremental_session_artifacts(from_layer, to_layer):
    """The session-constant artifacts the destination rung needs and the source
    rung did not already need.

    This is the whole of Finding 2: an artifact both rungs require is sunk on
    both sides of the transition and cancels out of its break-even.
    """
    for layer in (from_layer, to_layer):
        if layer not in LAYER_SESSION_PREREQUISITES:
            raise MeasurementError("no session prerequisites declared for layer %r" % layer)
    held = LAYER_SESSION_PREREQUISITES[from_layer]
    needed = LAYER_SESSION_PREREQUISITES[to_layer]
    return tuple(name for name in needed if name not in held)


def incremental_session_cost_tokens(from_layer, to_layer, session_artifacts):
    total = 0
    for name in incremental_session_artifacts(from_layer, to_layer):
        if name not in session_artifacts:
            raise MeasurementError(
                "transition %s->%s charges unmeasured session artifact %r" % (from_layer, to_layer, name)
            )
        total += session_artifacts[name]["tokens"]
    return total


def validate_amortization(entries, session_artifacts):
    """Refuse an amortization row that charges a cost already sunk on both sides.

    Every row names the transition it prices. The only one-time cost it may
    charge is the cost of what the destination rung needs and the source rung
    did not. A row that charges the schema() roundtrip to a transition between
    two agentquery rungs is pricing a decision the agent never makes.
    """
    if not entries:
        raise MeasurementError("no amortization entries to validate")
    for key in sorted(entries):
        entry = entries[key]
        for field in ("from_layer", "to_layer", "one_time_cost_tokens"):
            if field not in entry:
                raise MeasurementError("amortization %r does not declare %s" % (key, field))
        want = incremental_session_cost_tokens(
            entry["from_layer"], entry["to_layer"], session_artifacts)
        if entry["one_time_cost_tokens"] != want:
            held = ", ".join(LAYER_SESSION_PREREQUISITES[entry["from_layer"]]) or "nothing"
            raise MeasurementError(
                "amortization %r charges %d one-time tokens to %s->%s, but %s is already held "
                "at %s; the incremental one-time cost is %d"
                % (key, entry["one_time_cost_tokens"], entry["from_layer"], entry["to_layer"],
                   held, entry["from_layer"], want)
            )


# --- measurement ------------------------------------------------------------


def get_encoder():
    try:
        import tiktoken
    except ImportError:
        raise MeasurementError(
            "tiktoken is not importable; this measurement needs the %s encoding" % ENCODING_NAME
        )
    return tiktoken.get_encoding(ENCODING_NAME), getattr(tiktoken, "__version__", "unknown")


def count_tokens(enc, text):
    return len(enc.encode(text))


def read_text(root, rel):
    with open(os.path.join(root, rel)) as fh:
        return fh.read()


def pct(saved, denominator):
    return round((saved / float(denominator)) * 100.0, 4)


def build_ladder(name, layer_ids, tokens):
    steps = []
    for i in range(1, len(layer_ids)):
        prev, cur = layer_ids[i - 1], layer_ids[i]
        denom = tokens[prev]
        saved = denom - tokens[cur]
        steps.append({
            "step": "%s->%s" % (prev, cur),
            "from": prev,
            "to": cur,
            "denominator_tokens": denom,
            "result_tokens": tokens[cur],
            "incremental_saved_tokens": saved,
            "incremental_saved_pct_of_previous": pct(saved, denom),
            "cumulative_saved_tokens": tokens[layer_ids[0]] - tokens[cur],
            "cumulative_saved_pct_of_base": pct(tokens[layer_ids[0]] - tokens[cur], tokens[layer_ids[0]]),
        })
    reconcile_ladder(steps, tokens[layer_ids[0]], tokens[layer_ids[-1]])
    reconcile_percentages(steps, tokens[layer_ids[0]])
    return {"name": name, "layers": layer_ids, "base_tokens": tokens[layer_ids[0]],
            "final_tokens": tokens[layer_ids[-1]], "steps": steps}


def break_even(one_time_cost_tokens, per_query_saving_tokens):
    """Smallest whole query count whose saving strictly exceeds the one-time cost."""
    if per_query_saving_tokens <= 0:
        return None
    return int(math.floor(one_time_cost_tokens / float(per_query_saving_tokens))) + 1


def amortize(label, from_layer, to_layer, session_artifacts, per_query_saving_tokens, horizons):
    """Price one transition against the one-time cost that transition adds.

    The one-time cost is derived from the transition, not passed in, so a row
    cannot be charged an artifact the agent was already holding before it.
    """
    charged = incremental_session_artifacts(from_layer, to_layer)
    sunk = tuple(name for name in LAYER_SESSION_PREREQUISITES[to_layer]
                 if name in LAYER_SESSION_PREREQUISITES[from_layer])
    one_time_cost_tokens = incremental_session_cost_tokens(from_layer, to_layer, session_artifacts)
    net = {}
    for n in horizons:
        net[str(n)] = n * per_query_saving_tokens - one_time_cost_tokens
    return {
        "label": label,
        "from_layer": from_layer,
        "to_layer": to_layer,
        "one_time_cost_artifacts": list(charged),
        "sunk_on_both_sides": list(sunk),
        "one_time_cost_tokens": one_time_cost_tokens,
        "per_query_saving_tokens": per_query_saving_tokens,
        "break_even_queries": break_even(one_time_cost_tokens, per_query_saving_tokens),
        "net_tokens_after_n_queries": net,
    }


def measure_mcp(enc, root, contract, payload_by_scale):
    """Price the MCP shape of the same capability against the DSL shape.

    Both sides carry a byte-identical result payload, so the payload is not a
    differentiator. What differs is the session-constant contract the agent must
    hold and the per-call envelope around the payload.
    """
    validate_mcp_contract(contract)
    with open(os.path.join(root, contract["spec_probe"])) as fh:
        probe = json.load(fh)
    validate_spec_probe(probe, contract)

    session = {}
    for key, spec in sorted(contract["contracts"].items()):
        profiles = {}
        for profile, rel in sorted(spec["tools_list_result_profiles"].items()):
            profiles[profile] = count_tokens(enc, read_text(root, rel))
        session[key] = {
            "tool_count": spec["tool_count"],
            "tools_list_result_tokens": profiles["minimal"],
            "tools_list_result_tokens_by_profile": profiles,
            "derived_from": spec["derived_from"],
            "note": spec["note"],
        }

    # agentquery's session-constant equivalent: one schema() roundtrip, and only
    # if the agent introspects at all.
    dsl_session = {}
    for key, rel in (("scenario", "fixtures/schema-response.json"),
                     ("example-cli", "fixtures/example-schema-response.json")):
        dsl_session[key] = count_tokens(enc, read_text(root, rel))

    mcp_request = json.dumps({
        "jsonrpc": "2.0", "id": 2, "method": "tools/call",
        "params": {"name": "tasks_list",
                   "arguments": {"fields": ["id", "status", "assignee", "priority"]}},
    }, separators=(",", ":"))
    dsl_call = "taskdemo q 'list() { id status assignee priority }' --format compact"
    request_tokens = count_tokens(enc, mcp_request)
    dsl_call_tokens = count_tokens(enc, dsl_call)

    # The overhead is measured per payload FORMAT, not only per scale. An MCP
    # tool result embeds the payload as a JSON string, so every quote in the
    # payload is escaped and charged again. A JSON payload therefore pays more
    # framing than a compact one carrying the same records, and a table that
    # varies only the scale would miss that entirely.
    per_call = {}
    for scale in sorted(payload_by_scale):
        for layer, payload in sorted(payload_by_scale[scale].items()):
            mcp_result = json.dumps({
                "jsonrpc": "2.0", "id": 2,
                "result": {"content": [{"type": "text", "text": payload}], "isError": False},
            }, separators=(",", ":"))
            payload_tokens = count_tokens(enc, payload)
            result_tokens = count_tokens(enc, mcp_result)
            mcp_total = request_tokens + result_tokens
            dsl_total = dsl_call_tokens + payload_tokens
            per_call["%d/%s" % (scale, layer)] = {
                "scale": scale,
                "layer": layer,
                "payload_tokens": payload_tokens,
                "payload_double_quotes": payload.count('"'),
                "mcp_request_tokens": request_tokens,
                "mcp_result_envelope_and_payload_tokens": result_tokens,
                "mcp_total_tokens": mcp_total,
                "dsl_call_string_tokens": dsl_call_tokens,
                "dsl_total_tokens": dsl_total,
                "mcp_minus_dsl_tokens": mcp_total - dsl_total,
            }

    overheads = sorted(set(v["mcp_minus_dsl_tokens"] for v in per_call.values()))

    # The claim "MCP's tool definitions cost more than schema()" has no
    # profile-independent answer, so report the margin per profile instead of
    # quoting whichever one happens to favour the argument.
    margin = {}
    for key, s_row in sorted(session.items()):
        dsl = dsl_session[key]
        margin[key] = {}
        for profile, tokens in sorted(s_row["tools_list_result_tokens_by_profile"].items()):
            margin[key][profile] = {
                "mcp_tools_list_tokens": tokens,
                "dsl_schema_tokens": dsl,
                "mcp_minus_dsl_tokens": tokens - dsl,
                "mcp_cheaper": tokens < dsl,
            }

    return {
        "evidence_class": contract["evidence_class"],
        "host_measured": contract["host_measured"],
        "protocol_version": contract["protocol_version"],
        "spec_probe": {
            "path": contract["spec_probe"],
            "probed_at": probe["probed_at"],
            "latest_revision": probe["latest_revision"],
            "control_absent_revision": probe["control_absent_revision"],
            "control_absent_revision_status": probe["control_absent_revision_status"],
        },
        "profiles": contract["profiles"],
        "profile_notes": contract["profile_notes"],
        "session_constant_margin": margin,
        "protocol_source": contract["protocol_source"],
        "sdk_reference": contract["sdk_reference"],
        "projection_supported_by_contract": contract["projection_supported"],
        "limitations": contract["limitations"],
        "session_constant_mcp_tools_list": session,
        "session_constant_dsl_schema_response": dsl_session,
        "per_call": per_call,
        "per_call_overhead_tokens": {
            "min": overheads[0],
            "max": overheads[-1],
            "constant_across_measured_payloads": len(overheads) == 1,
        },
    }


def run(root):
    enc, tiktoken_version = get_encoder()

    manifest_path = os.path.join(root, "fixtures", "manifest.json")
    if not os.path.exists(manifest_path):
        raise MeasurementError("missing %s; run: go run ./cmd/genfixtures" % manifest_path)
    with open(manifest_path) as fh:
        manifest = json.load(fh)

    validate_manifest(manifest, root)

    fixtures = {}
    for entry in manifest["fixtures"]:
        fixtures.setdefault(entry["scale"], {})[entry["layer"]] = entry

    tokens_by_scale = {}
    payload_by_scale = {}
    for scale in manifest["scales"]:
        tokens = {}
        for layer, entry in fixtures[scale].items():
            text = read_text(root, entry["path"])
            tokens[layer] = count_tokens(enc, text)
            if layer in MCP_PAYLOAD_LAYERS:
                payload_by_scale.setdefault(scale, {})[layer] = text
        tokens_by_scale[scale] = tokens

    ladders = {}
    for scale in manifest["scales"]:
        ladders[str(scale)] = {
            name: build_ladder(name, ids, tokens_by_scale[scale])
            for name, ids in sorted(LADDERS.items())
        }

    schema_tokens = {}
    for artifact in manifest["schema_artifacts"]:
        schema_tokens[artifact["name"]] = {
            "tokens": count_tokens(enc, read_text(root, artifact["path"])),
            "bytes": artifact["bytes"],
            "note": artifact["note"],
        }

    horizons = [1, 5, 10, 20, 50, 100]
    amortization = {}
    for scale in manifest["scales"]:
        tokens = tokens_by_scale[scale]
        entries = {
            "compact_vs_min_projected_json": amortize(
                "L3->L4: same agentquery session, same schema() already read; "
                "selecting the compact format adds no one-time cost",
                "L3", "L4", schema_tokens,
                tokens["L3"] - tokens["L4"],
                horizons,
            ),
            "alias_vs_compact": amortize(
                "L4->L5: paid against the alias legend, which L4 does not need",
                "L4", "L5", schema_tokens,
                tokens["L4"] - tokens["L5"],
                horizons,
            ),
            "projection_vs_full_minified_json": amortize(
                "L1->L3: same agentquery session, same schema() already read; "
                "writing a projected query adds no one-time cost",
                "L1", "L3", schema_tokens,
                tokens["L1"] - tokens["L3"],
                horizons,
            ),
        }
        validate_amortization(entries, schema_tokens)
        amortization[str(scale)] = entries

    with open(os.path.join(root, "mcp", "contract.json")) as fh:
        contract = json.load(fh)
    mcp = measure_mcp(enc, root, contract, payload_by_scale)

    return {
        "tokenizer": {"encoding": ENCODING_NAME, "tiktoken_version": tiktoken_version,
                      "python": sys.version.split()[0]},
        "scenario": manifest["scenario"],
        "scenario_query": manifest["scenario_query"],
        "full_query": manifest["full_query"],
        "full_fields": manifest["full_fields"],
        "scenario_fields": manifest["scenario_fields"],
        "source_payloads": manifest["source_payloads"],
        "layers": manifest["layers"],
        "scales": manifest["scales"],
        "tokens_by_scale": {str(k): v for k, v in tokens_by_scale.items()},
        "ladders": ladders,
        "session_artifacts": schema_tokens,
        "amortization": amortization,
        "mcp": mcp,
    }


# --- report -----------------------------------------------------------------


def render_markdown(results):
    L = []
    a = L.append
    a("# Layer-ladder token measurement")
    a("")
    a("Generated by `measure.py`. Every number below is a `%s` token count of a "
      "checked-in fixture; nothing here is a model call, a host transcript or an estimate."
      % results["tokenizer"]["encoding"])
    a("")
    a("- Tokenizer: `%s`, tiktoken %s, Python %s"
      % (results["tokenizer"]["encoding"], results["tokenizer"]["tiktoken_version"],
         results["tokenizer"]["python"]))
    a("- Source data: `%s` (the checked-in 2026-02-12 synthetic fixtures, unchanged)"
      % results["source_payloads"])
    a("- Scenario: %s" % results["scenario"])
    a("- Full record: `%s`" % ", ".join(results["full_fields"]))
    a("- Scenario needs: `%s`" % ", ".join(results["scenario_fields"]))
    a("- Production query: `%s`" % results["scenario_query"])
    a("")
    a("## Layers")
    a("")
    a("| Layer | Name | Fields | Produced by | Evidence class |")
    a("|---|---|---|---|---|")
    for layer in results["layers"]:
        a("| %s | %s | %s | `%s` | %s |" % (layer["id"], layer["name"], layer["fields"],
                                            layer["production_path"], layer["evidence_class"]))
    a("")
    a("## Raw token counts")
    a("")
    ids = [l["id"] for l in results["layers"]]
    a("| Scale | " + " | ".join(ids) + " |")
    a("|---:|" + "|".join(["---:"] * len(ids)) + "|")
    for scale in results["scales"]:
        row = results["tokens_by_scale"][str(scale)]
        a("| %d | %s |" % (scale, " | ".join("{:,}".format(row[i]) for i in ids)))
    a("")
    a("## Incremental ladders")
    a("")
    a("Two orderings reach the same final payload. The cumulative saving is identical; "
      "the attribution per step is not. Quoting one ordering's step as \"the saving from X\" "
      "credits X with whatever the other ordering already removed.")
    a("")
    for scale in results["scales"]:
        a("### Scale %d" % scale)
        a("")
        for name in sorted(results["ladders"][str(scale)]):
            ladder = results["ladders"][str(scale)][name]
            a("**%s** — %s" % (name, " -> ".join(ladder["layers"])))
            a("")
            a("| Step | Denominator (tokens) | Result (tokens) | Saved | % of previous rung | Cumulative % of L0 |")
            a("|---|---:|---:|---:|---:|---:|")
            for s in ladder["steps"]:
                a("| %s | %s | %s | %s | %.2f%% | %.2f%% |"
                  % (s["step"], "{:,}".format(s["denominator_tokens"]),
                     "{:,}".format(s["result_tokens"]), "{:,}".format(s["incremental_saved_tokens"]),
                     s["incremental_saved_pct_of_previous"], s["cumulative_saved_pct_of_base"]))
            a("")
    a("## Session-constant artifacts")
    a("")
    a("| Artifact | Tokens | Bytes | Note |")
    a("|---|---:|---:|---|")
    for name, info in sorted(results["session_artifacts"].items()):
        a("| %s | %s | %s | %s |" % (name, "{:,}".format(info["tokens"]),
                                     "{:,}".format(info["bytes"]), info["note"]))
    a("")
    a("## Amortization")
    a("")
    a("A per-response saving only becomes a session saving after it has paid off the "
      "one-time contract the agent had to read **to make that particular move**. The "
      "one-time cost of a transition is what its destination rung requires and its "
      "source rung did not: an artifact both rungs need is sunk on both sides and "
      "cancels. Every rung of this ladder is agentquery output, so the `schema()` "
      "roundtrip is a prerequisite of all of them and is not incremental to any "
      "projection or format step. It is reported above as a session constant instead.")
    a("")
    a("| Scale | Comparison | Transition | Charged | Sunk on both sides | One-time cost | Saving / query | Break-even | Net @10 | Net @100 |")
    a("|---:|---|---|---|---|---:|---:|---:|---:|---:|")
    for scale in results["scales"]:
        for key in sorted(results["amortization"][str(scale)]):
            am = results["amortization"][str(scale)][key]
            be = am["break_even_queries"]
            a("| %d | %s | %s->%s | %s | %s | %s | %s | %s | %s | %s |"
              % (scale, key, am["from_layer"], am["to_layer"],
                 ", ".join(am["one_time_cost_artifacts"]) or "nothing",
                 ", ".join(am["sunk_on_both_sides"]) or "nothing",
                 "{:,}".format(am["one_time_cost_tokens"]),
                 "{:,}".format(am["per_query_saving_tokens"]),
                 "never" if be is None else "{:,}".format(be),
                 "{:+,}".format(am["net_tokens_after_n_queries"]["10"]),
                 "{:+,}".format(am["net_tokens_after_n_queries"]["100"])))
    a("")
    a("This ladder has no non-DSL rung, so it cannot price DSL *adoption*. The question "
      "\"is the DSL worth its schema() roundtrip against a REST client that needs no "
      "contract at all\" is a different comparison, against a baseline that is not "
      "measured here.")
    a("")
    mcp = results["mcp"]
    a("## MCP comparison (%s, host_measured=%s)" % (mcp["evidence_class"], mcp["host_measured"]))
    a("")
    a("Protocol revision `%s` — %s" % (mcp["protocol_version"], mcp["protocol_source"]))
    a("")
    pr = mcp["spec_probe"]
    a("Revision freshness is not asserted, it is probed: `%s` records a %s read in which "
      "`/specification/latest` resolves to `%s`, with control revision `%s` returning "
      "`%s` so an absent revision cannot be confused with a failed read. `measure.py` "
      "refuses any contract pinned to something other than the probed revision."
      % (pr["path"], pr["probed_at"], pr["latest_revision"], pr["control_absent_revision"],
         pr["control_absent_revision_status"]))
    a("")
    a("The tool contracts are *derived from* the real `schema()` output, so both sides expose "
      "the same operations, parameters and descriptions. The `fields` argument on the projecting "
      "tools is the point: **MCP carries field projection fine** — it is an ordinary tool argument. "
      "Projection is a server design choice, not a protocol capability.")
    a("")
    a("The session-constant comparison has no profile-independent answer, so both profiles "
      "are published. `minimal` carries only what the `Tool` interface requires plus what "
      "`schema()` already publishes; `structured` adds the `outputSchema` the current "
      "revision's structured-content surface expects, derived from the same field list.")
    a("")
    for profile in mcp["profiles"]:
        a("- `%s` — %s" % (profile, mcp["profile_notes"][profile]))
    a("")
    a("| Contract | Tools | Profile | MCP `tools/list` result (tokens) | agentquery `schema()` response (tokens) | MCP − DSL | MCP cheaper? |")
    a("|---|---:|---|---:|---:|---:|---|")
    for key in sorted(mcp["session_constant_margin"]):
        row = mcp["session_constant_margin"][key]
        count = mcp["session_constant_mcp_tools_list"][key]["tool_count"]
        for profile in mcp["profiles"]:
            m = row[profile]
            a("| %s | %d | %s | %s | %s | %s | %s |"
              % (key, count, profile, "{:,}".format(m["mcp_tools_list_tokens"]),
                 "{:,}".format(m["dsl_schema_tokens"]),
                 "{:+,}".format(m["mcp_minus_dsl_tokens"]),
                 "yes" if m["mcp_cheaper"] else "no"))
    a("")
    flips = [key for key, row in sorted(mcp["session_constant_margin"].items())
             if len(set(row[p]["mcp_cheaper"] for p in mcp["profiles"])) > 1]
    if flips:
        a("**The direction of this comparison is not stable across profiles.** For %s the "
          "answer to \"is MCP's tool-definition cost larger than the DSL's?\" flips between "
          "the minimal and the structured profile. No single number here settles it, and a "
          "claim in either direction has to name the profile it rests on."
          % ", ".join("`%s`" % f for f in flips))
    else:
        a("The direction of this comparison is the same in both profiles for every contract "
          "measured here.")
    a("")
    a("Per-call overhead is measured per payload *format*, not only per scale. An MCP "
      "tool result embeds the payload as a JSON string, so every quote in the payload is "
      "escaped and charged a second time. A JSON payload therefore pays more framing than "
      "a compact payload carrying the same records.")
    a("")
    a("| Scale | Payload layer | `\"` in payload | Payload | MCP request | MCP result envelope+payload | MCP total | DSL call+payload | MCP − DSL |")
    a("|---:|---|---:|---:|---:|---:|---:|---:|---:|")
    for key in sorted(mcp["per_call"], key=lambda k: (int(k.split("/")[0]), k.split("/")[1])):
        c = mcp["per_call"][key]
        a("| %d | %s | %s | %s | %s | %s | %s | %s | %s |"
          % (c["scale"], c["layer"], "{:,}".format(c["payload_double_quotes"]),
             "{:,}".format(c["payload_tokens"]), "{:,}".format(c["mcp_request_tokens"]),
             "{:,}".format(c["mcp_result_envelope_and_payload_tokens"]),
             "{:,}".format(c["mcp_total_tokens"]), "{:,}".format(c["dsl_total_tokens"]),
             "{:+,}".format(c["mcp_minus_dsl_tokens"])))
    a("")
    ov = mcp["per_call_overhead_tokens"]
    a("Measured overhead range: **%+d to %+d tokens** per call. Constant across the "
      "measured payloads: **%s**."
      % (ov["min"], ov["max"], "yes" if ov["constant_across_measured_payloads"] else "no"))
    a("")
    a("Limitations of the MCP section:")
    a("")
    for lim in mcp["limitations"]:
        a("- %s" % lim)
    a("")
    return "\n".join(L).rstrip("\n") + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="validate and compute without writing")
    parser.add_argument("--root", default=HERE)
    args = parser.parse_args()

    try:
        results = run(args.root)
    except MeasurementError as exc:
        print("measure.py refused: %s" % exc, file=sys.stderr)
        return 2

    if args.check:
        print("ok: %d scales, %d layers, ladders reconcile"
              % (len(results["scales"]), len(results["layers"])))
        return 0

    with open(os.path.join(args.root, "results.json"), "w") as fh:
        json.dump(results, fh, indent=2, sort_keys=True)
        fh.write("\n")
    with open(os.path.join(args.root, "RESULTS.md"), "w") as fh:
        fh.write(render_markdown(results))
    print("wrote results.json and RESULTS.md")
    return 0


if __name__ == "__main__":
    sys.exit(main())
