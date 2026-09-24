#!/usr/bin/env python3
"""Tests for the layer-ladder measurement gates and arithmetic.

Run from .research/layer-ladder:

    /usr/bin/python3 -m unittest discover -s tests -v
"""

import copy
import json
import os
import shutil
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
sys.path.insert(0, ROOT)

import measure  # noqa: E402

DIGEST_A = "a" * 64
DIGEST_B = "b" * 64


def make_manifest(tmpdir, digests=None, layers=("L0", "L1", "L4"), scales=(5, 20)):
    """Materialize a minimal but structurally valid fixture tree.

    digests maps (scale, layer) -> digest; anything unmapped gets DIGEST_A.
    """
    digests = digests or {}
    manifest = {
        "scales": list(scales),
        "layers": [{"id": layer, "name": layer.lower()} for layer in layers],
        "fixtures": [],
    }
    for scale in scales:
        scale_dir = os.path.join(tmpdir, "fixtures", str(scale))
        os.makedirs(scale_dir)
        for layer in layers:
            rel = os.path.join("fixtures", str(scale), layer + ".txt")
            body = "%s-%d payload" % (layer, scale)
            with open(os.path.join(tmpdir, rel), "w") as fh:
                fh.write(body)
            manifest["fixtures"].append({
                "scale": scale,
                "layer": layer,
                "path": rel,
                "bytes": len(body),
                "scenario_digest": digests.get((scale, layer), DIGEST_A),
            })
    return manifest


class ManifestGateTest(unittest.TestCase):
    """The manifest gate must refuse any fixture set that cannot support a
    same-data comparison. A positive control sits alongside every rejection so a
    gate that refused everything would not pass this suite."""

    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)

    def test_accepts_a_consistent_fixture_set(self):
        manifest = make_manifest(self.tmp)
        measure.validate_manifest(manifest, self.tmp)

    def test_accepts_the_real_generated_manifest(self):
        with open(os.path.join(ROOT, "fixtures", "manifest.json")) as fh:
            manifest = json.load(fh)
        measure.validate_manifest(manifest, ROOT)

    def test_rejects_missing_fixture_file(self):
        manifest = make_manifest(self.tmp)
        os.remove(os.path.join(self.tmp, manifest["fixtures"][-1]["path"]))
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)

    def test_rejects_byte_count_tamper(self):
        manifest = make_manifest(self.tmp)
        manifest["fixtures"][0]["bytes"] += 1
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)

    def test_rejects_digest_mismatch_at_last_scale(self):
        # The disagreement is only at the final scale. A gate that checks the
        # first scale and stops admits this.
        manifest = make_manifest(self.tmp, digests={(20, "L1"): DIGEST_B})
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)

    def test_rejects_digest_mismatch_on_last_layer(self):
        # The disagreement is only on the final layer of the ladder. A gate that
        # stops one layer short admits this.
        manifest = make_manifest(self.tmp, digests={(5, "L4"): DIGEST_B})
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)

    def test_rejects_missing_layer_at_one_scale(self):
        manifest = make_manifest(self.tmp)
        manifest["fixtures"] = [f for f in manifest["fixtures"]
                                if not (f["scale"] == 20 and f["layer"] == "L4")]
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)

    def test_rejects_fixture_for_undeclared_layer(self):
        manifest = make_manifest(self.tmp)
        rogue = copy.deepcopy(manifest["fixtures"][0])
        rogue["layer"] = "L9"
        manifest["fixtures"].append(rogue)
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)

    def test_rejects_manifest_without_layers(self):
        manifest = make_manifest(self.tmp)
        manifest["layers"] = []
        with self.assertRaises(measure.MeasurementError):
            measure.validate_manifest(manifest, self.tmp)


def ladder_steps(tokens, ids):
    steps = []
    for i in range(1, len(ids)):
        prev, cur = ids[i - 1], ids[i]
        steps.append({
            "step": "%s->%s" % (prev, cur),
            "denominator_tokens": tokens[prev],
            "incremental_saved_tokens": tokens[prev] - tokens[cur],
        })
    return steps


class LadderArithmeticTest(unittest.TestCase):
    """Per-step savings must close exactly against the base and final rungs."""

    TOKENS = {"L0": 1000, "L1": 700, "L3": 300, "L4": 180}
    IDS = ["L0", "L1", "L3", "L4"]

    def test_accepts_a_closing_ladder(self):
        measure.reconcile_ladder(ladder_steps(self.TOKENS, self.IDS), 1000, 180)

    def test_real_fixtures_reconcile(self):
        results = measure.run(ROOT)
        for scale, ladders in results["ladders"].items():
            for name, ladder in ladders.items():
                measure.reconcile_ladder(ladder["steps"], ladder["base_tokens"],
                                         ladder["final_tokens"])
                self.assertEqual(len(ladder["steps"]), len(ladder["layers"]) - 1,
                                 "scale %s ladder %s lost a step" % (scale, name))

    def test_rejects_off_by_one_incremental_sum(self):
        # One token unaccounted for. A tolerance here would let a whole dropped
        # or double-counted rung hide as rounding.
        steps = ladder_steps(self.TOKENS, self.IDS)
        steps[-1]["incremental_saved_tokens"] -= 1
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_ladder(steps, 1000, 180)

    def test_rejects_denominator_taken_from_the_base_instead_of_previous_rung(self):
        steps = ladder_steps(self.TOKENS, self.IDS)
        steps[1]["denominator_tokens"] = 1000
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_ladder(steps, 1000, 180)

    def test_rejects_zero_denominator(self):
        steps = ladder_steps(self.TOKENS, self.IDS)
        steps[0]["denominator_tokens"] = 0
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_ladder(steps, 1000, 180)

    def test_rejects_zero_base(self):
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_ladder([], 0, 0)

    def test_break_even_is_first_strictly_positive_query(self):
        # Arithmetic only. 535 against 42 was the rev2 published compact row and
        # is no longer one: see AmortizationAttributionTest for why a transition
        # between two agentquery rungs is charged nothing.
        # 535 one-time, 42 per query: 12 queries nets -31, 13 nets +11.
        self.assertEqual(measure.break_even(535, 42), 13)
        self.assertEqual(12 * 42 - 535 > 0, False)
        self.assertEqual(13 * 42 - 535 > 0, True)

    def test_break_even_is_none_when_there_is_nothing_to_amortize(self):
        self.assertIsNone(measure.break_even(535, 0))
        self.assertIsNone(measure.break_even(535, -3))

    def test_percentage_uses_the_stated_denominator(self):
        self.assertEqual(measure.pct(42, 107), 39.2523)


def valid_contract():
    with open(os.path.join(ROOT, "mcp", "contract.json")) as fh:
        return json.load(fh)


class MCPContractGateTest(unittest.TestCase):
    """A synthetic contract may be published; a synthetic contract dressed as a
    host measurement may not."""

    def test_accepts_the_generated_contract(self):
        measure.validate_mcp_contract(valid_contract())

    def test_rejects_contract_claiming_host_measured(self):
        contract = valid_contract()
        contract["host_measured"] = True
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)

    def test_rejects_contract_with_unset_host_measured(self):
        contract = valid_contract()
        del contract["host_measured"]
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)

    def test_rejects_contract_relabelled_as_measurement(self):
        contract = valid_contract()
        contract["evidence_class"] = "host-measurement"
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)

    def test_rejects_contract_without_protocol_version(self):
        contract = valid_contract()
        contract["protocol_version"] = ""
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)

    def test_rejects_contract_without_primary_source(self):
        contract = valid_contract()
        del contract["protocol_source"]
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)

    def test_rejects_contract_without_limitations(self):
        contract = valid_contract()
        contract["limitations"] = []
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)


class MCPContractShapeTest(unittest.TestCase):
    """The derived contract must expose the same capability as the DSL schema,
    including field projection — otherwise the comparison is a strawman."""

    def test_projecting_tools_accept_a_fields_argument(self):
        contract = valid_contract()
        for key, spec in contract["contracts"].items():
            for profile, rel in sorted(spec["tools_list_result_profiles"].items()):
                with open(os.path.join(ROOT, rel)) as fh:
                    tools = json.load(fh)["result"]["tools"]
                names = set(t["name"] for t in tools)
                self.assertEqual(len(names), spec["tool_count"],
                                 "%s/%s has duplicate tool names" % (key, profile))
                for tool in tools:
                    op = tool["name"].split("_", 1)[1]
                    props = tool["inputSchema"]["properties"]
                    if op in measure_projection_ops():
                        self.assertIn("fields", props,
                                      "%s/%s.%s cannot project" % (key, profile, tool["name"]))
                        self.assertEqual(props["fields"]["type"], "array")

    def test_derived_tools_match_the_real_schema_operations(self):
        with open(os.path.join(ROOT, "fixtures", "example-schema-response.json")) as fh:
            schema = json.load(fh)
        expected = set(schema["operations"]) | set(schema.get("mutationMetadata") or {})
        profiles = valid_contract()["contracts"]["example-cli"]["tools_list_result_profiles"]
        for profile, rel in sorted(profiles.items()):
            with open(os.path.join(ROOT, rel)) as fh:
                tools = json.load(fh)["result"]["tools"]
            derived = set(t["name"].split("_", 1)[1] for t in tools)
            self.assertEqual(derived, expected,
                             "derived %s MCP contract does not match the real operation surface"
                             % profile)


def measure_projection_ops():
    sys.path.insert(0, os.path.join(ROOT, "mcp"))
    import build_contract
    return build_contract.PROJECTING_OPERATIONS


if __name__ == "__main__":
    unittest.main()


class MCPEnvelopeOverheadTest(unittest.TestCase):
    """An MCP tool result embeds the payload as a JSON string, so payload quotes
    are escaped and charged twice. Measuring only one payload format hides that
    and yields a false "the envelope adds a fixed amount" claim."""

    @classmethod
    def setUpClass(cls):
        cls.results = measure.run(ROOT)
        cls.per_call = cls.results["mcp"]["per_call"]

    def test_mcp_overhead_is_not_constant_across_payload_formats(self):
        summary = self.results["mcp"]["per_call_overhead_tokens"]
        self.assertFalse(summary["constant_across_measured_payloads"],
                         "overhead reported as constant; at least two payload formats "
                         "must be priced before that can be claimed either way")
        self.assertGreater(summary["max"], summary["min"])

    def test_overhead_orders_with_payload_quote_count(self):
        for scale in self.results["scales"]:
            rows = [self.per_call["%d/%s" % (scale, layer)] for layer in ("L4", "L3", "L1")]
            quotes = [r["payload_double_quotes"] for r in rows]
            overhead = [r["mcp_minus_dsl_tokens"] for r in rows]
            self.assertEqual(quotes, sorted(quotes),
                             "scale %d: quote counts are not ordered L4<L3<L1" % scale)
            self.assertEqual(overhead, sorted(overhead),
                             "scale %d: overhead %s does not follow quote count %s"
                             % (scale, overhead, quotes))

    def test_quote_free_payload_pays_only_the_bare_envelope(self):
        # The compact layer carries no quotes, so its overhead is the envelope
        # alone and must not move with payload size.
        overheads = set(self.per_call["%d/L4" % scale]["mcp_minus_dsl_tokens"]
                        for scale in self.results["scales"])
        self.assertEqual(len(overheads), 1,
                         "quote-free payload overhead varied: %s" % sorted(overheads))

    def test_framing_excess_follows_quotes_not_payload_size(self):
        # The decisive contrast: the largest quote-free payload pays less framing
        # excess than the smallest quoted one. Overhead cannot be explained by
        # payload size.
        bare = self.per_call["5/L4"]["mcp_minus_dsl_tokens"]
        biggest_quote_free = self.per_call["500/L4"]
        smallest_quoted = self.per_call["5/L3"]
        self.assertGreater(biggest_quote_free["payload_tokens"],
                           smallest_quoted["payload_tokens"] * 50)
        self.assertEqual(biggest_quote_free["mcp_minus_dsl_tokens"] - bare, 0)
        self.assertGreater(smallest_quoted["mcp_minus_dsl_tokens"] - bare, 0)

    def test_doubling_the_quote_count_roughly_doubles_the_framing_excess(self):
        # L1 carries exactly twice L3's quotes at every scale. The excess ratio
        # approaches 2 as the payload grows; it is not exactly 2, because token
        # boundaries at the payload edges do not partition cleanly. Asserting a
        # band, and asserting that the band tightens, is what the data supports.
        bare = self.per_call["5/L4"]["mcp_minus_dsl_tokens"]
        ratios = []
        for scale in self.results["scales"]:
            l1, l3 = self.per_call["%d/L1" % scale], self.per_call["%d/L3" % scale]
            self.assertEqual(l1["payload_double_quotes"], 2 * l3["payload_double_quotes"])
            excess_l1 = l1["mcp_minus_dsl_tokens"] - bare
            excess_l3 = l3["mcp_minus_dsl_tokens"] - bare
            self.assertGreater(excess_l3, 0)
            ratios.append(excess_l1 / float(excess_l3))
        for r in ratios:
            self.assertTrue(2.0 <= r <= 2.3, "excess ratio %.4f outside 2.0-2.3" % r)
        self.assertTrue(ratios == sorted(ratios, reverse=True),
                        "ratio should tighten toward 2 as scale grows, got %s" % ratios)


def percentage_steps(tokens, ids):
    """A structurally valid ladder whose percentages are all correctly denominated."""
    steps = []
    for i in range(1, len(ids)):
        prev, cur = ids[i - 1], ids[i]
        saved = tokens[prev] - tokens[cur]
        cumulative = tokens[ids[0]] - tokens[cur]
        steps.append({
            "step": "%s->%s" % (prev, cur),
            "denominator_tokens": tokens[prev],
            "incremental_saved_tokens": saved,
            "incremental_saved_pct_of_previous": measure.pct(saved, tokens[prev]),
            "cumulative_saved_tokens": cumulative,
            "cumulative_saved_pct_of_base": measure.pct(cumulative, tokens[ids[0]]),
        })
    return steps


class LadderPercentageTest(unittest.TestCase):
    """The published percentage column must be denominated in the rung its own
    header names. reconcile_ladder checks token arithmetic only: a percentage
    denominated in the ladder base or in the after-value leaves every token
    count correct, closes the ladder exactly, and still publishes a wrong
    column. These tests drive build_ladder through measure.run over the real
    fixtures, not the pct helper in isolation.

    measure.run is called inside each test rather than in setUpClass so that a
    production refusal is attributed to the named test and not to class setup."""

    TOKENS = {"L0": 1000, "L1": 700, "L3": 300, "L4": 180}
    IDS = ["L0", "L1", "L3", "L4"]

    def test_published_percentages_are_denominated_in_the_stated_rung(self):
        results = measure.run(ROOT)
        checked = 0
        for scale, ladders in sorted(results["ladders"].items()):
            raw = results["tokens_by_scale"][scale]
            for name, ladder in sorted(ladders.items()):
                base = raw[ladder["layers"][0]]
                self.assertEqual(ladder["base_tokens"], base)
                for step in ladder["steps"]:
                    where = "scale %s ladder %s step %s" % (scale, name, step["step"])
                    # Anchor the denominator to the measured token table, not to
                    # the ladder's own self-report.
                    self.assertEqual(step["denominator_tokens"], raw[step["from"]],
                                     "%s: denominator is not the from-rung's token count" % where)
                    self.assertEqual(step["result_tokens"], raw[step["to"]], where)
                    saved = raw[step["from"]] - raw[step["to"]]
                    self.assertEqual(step["incremental_saved_tokens"], saved, where)
                    # Recomputed inline, not through measure.pct, so the helper
                    # and the call site cannot agree by both being wrong.
                    want = round(saved / float(raw[step["from"]]) * 100.0, 4)
                    self.assertEqual(step["incremental_saved_pct_of_previous"], want,
                                     "%s: published %s%%, but %d of %d is %s%%"
                                     % (where, step["incremental_saved_pct_of_previous"],
                                        saved, raw[step["from"]], want))
                    cumulative = base - raw[step["to"]]
                    self.assertEqual(step["cumulative_saved_tokens"], cumulative, where)
                    want_cum = round(cumulative / float(base) * 100.0, 4)
                    self.assertEqual(step["cumulative_saved_pct_of_base"], want_cum,
                                     "%s: cumulative published %s%%, want %s%%"
                                     % (where, step["cumulative_saved_pct_of_base"], want_cum))
                    checked += 1
        # 4 scales x 2 ladders x 4 steps. A silently shortened loop would not
        # reach this count.
        self.assertEqual(checked, 32, "expected 32 published steps, checked %d" % checked)

    def test_a_later_step_is_not_denominated_in_the_base(self):
        # The decisive case: at the first step the previous rung IS the base, so
        # a base-denominated column only diverges from the second step onward.
        results = measure.run(ROOT)
        diverged = 0
        for scale, ladders in sorted(results["ladders"].items()):
            for ladder in ladders.values():
                base = ladder["base_tokens"]
                for step in ladder["steps"][1:]:
                    as_base = measure.pct(step["incremental_saved_tokens"], base)
                    if as_base != step["incremental_saved_pct_of_previous"]:
                        diverged += 1
                    self.assertNotEqual(
                        step["incremental_saved_pct_of_previous"], as_base,
                        "scale %s step %s: published percentage equals the base-denominated "
                        "value, which is what the column must not be" % (scale, step["step"]))
        self.assertEqual(diverged, 24, "expected 24 later steps to diverge, got %d" % diverged)

    def test_accepts_correctly_denominated_percentages(self):
        measure.reconcile_percentages(percentage_steps(self.TOKENS, self.IDS), 1000)

    def test_rejects_base_denominated_percentage_on_a_later_step(self):
        # Only the second step is wrong. A gate that checks the first step and
        # stops admits exactly the defect this exists to catch.
        steps = percentage_steps(self.TOKENS, self.IDS)
        steps[1]["incremental_saved_pct_of_previous"] = measure.pct(
            steps[1]["incremental_saved_tokens"], 1000)
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_percentages(steps, 1000)

    def test_rejects_base_denominated_percentage_on_the_last_step(self):
        steps = percentage_steps(self.TOKENS, self.IDS)
        steps[-1]["incremental_saved_pct_of_previous"] = measure.pct(
            steps[-1]["incremental_saved_tokens"], 1000)
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_percentages(steps, 1000)

    def test_rejects_percentage_denominated_in_the_after_value(self):
        steps = percentage_steps(self.TOKENS, self.IDS)
        steps[1]["incremental_saved_pct_of_previous"] = measure.pct(
            steps[1]["incremental_saved_tokens"], 300)
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_percentages(steps, 1000)

    def test_rejects_cumulative_percentage_denominated_in_the_previous_rung(self):
        steps = percentage_steps(self.TOKENS, self.IDS)
        steps[2]["cumulative_saved_pct_of_base"] = measure.pct(
            steps[2]["cumulative_saved_tokens"], steps[2]["denominator_tokens"])
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_percentages(steps, 1000)

    def test_rejects_an_empty_ladder(self):
        with self.assertRaises(measure.MeasurementError):
            measure.reconcile_percentages([], 1000)


class AmortizationAttributionTest(unittest.TestCase):
    """A transition may only be charged the session-constant artifacts its
    destination rung needs and its source rung did not. Both rungs of every
    projection and format step here are agentquery output, so the schema()
    roundtrip is sunk on both sides and cancels; charging it inflates the
    break-even of a decision the agent never makes."""

    ARTIFACTS = {"agentquery-schema-response": {"tokens": 535},
                 "alias-legend": {"tokens": 21}}

    def test_published_entries_charge_only_what_the_transition_adds(self):
        results = measure.run(ROOT)
        expected = {
            "compact_vs_min_projected_json": ("L3", "L4", 0, []),
            "projection_vs_full_minified_json": ("L1", "L3", 0, []),
            "alias_vs_compact": ("L4", "L5", 21, ["alias-legend"]),
        }
        seen = 0
        for scale in results["scales"]:
            rows = results["amortization"][str(scale)]
            self.assertEqual(sorted(rows), sorted(expected))
            for key, (src, dst, cost, charged) in sorted(expected.items()):
                row = rows[key]
                where = "scale %d %s" % (scale, key)
                self.assertEqual((row["from_layer"], row["to_layer"]), (src, dst), where)
                self.assertEqual(row["one_time_cost_tokens"], cost, where)
                self.assertEqual(row["one_time_cost_artifacts"], charged, where)
                self.assertIn("agentquery-schema-response", row["sunk_on_both_sides"], where)
                # The net at every horizon must follow from the charged cost.
                for n, net in sorted(row["net_tokens_after_n_queries"].items()):
                    self.assertEqual(net, int(n) * row["per_query_saving_tokens"] - cost, where)
                seen += 1
        self.assertEqual(seen, 12, "expected 12 published amortization rows, saw %d" % seen)

    def test_schema_roundtrip_is_never_charged_to_a_dsl_internal_transition(self):
        # The exact published defect: the schema() response is 535 tokens, and no
        # transition between two agentquery rungs may carry it.
        results = measure.run(ROOT)
        schema_tokens = results["session_artifacts"]["agentquery-schema-response"]["tokens"]
        self.assertEqual(schema_tokens, 535)
        for scale in results["scales"]:
            for key, row in sorted(results["amortization"][str(scale)].items()):
                if key == "alias_vs_compact":
                    continue
                self.assertNotEqual(
                    row["one_time_cost_tokens"], schema_tokens,
                    "scale %d %s charges the sunk schema() roundtrip" % (scale, key))
                self.assertNotIn("agentquery-schema-response", row["one_time_cost_artifacts"])

    def test_incremental_cost_is_the_set_difference(self):
        self.assertEqual(measure.incremental_session_artifacts("L3", "L4"), ())
        self.assertEqual(measure.incremental_session_artifacts("L1", "L3"), ())
        self.assertEqual(measure.incremental_session_artifacts("L4", "L5"), ("alias-legend",))
        self.assertEqual(
            measure.incremental_session_cost_tokens("L4", "L5", self.ARTIFACTS), 21)
        self.assertEqual(
            measure.incremental_session_cost_tokens("L3", "L4", self.ARTIFACTS), 0)

    def test_accepts_correctly_attributed_entries(self):
        entries = {
            "compact": {"from_layer": "L3", "to_layer": "L4", "one_time_cost_tokens": 0},
            "alias": {"from_layer": "L4", "to_layer": "L5", "one_time_cost_tokens": 21},
        }
        measure.validate_amortization(entries, self.ARTIFACTS)

    def test_rejects_sunk_schema_charged_to_a_compact_transition(self):
        entries = {
            "alias": {"from_layer": "L4", "to_layer": "L5", "one_time_cost_tokens": 21},
            "compact": {"from_layer": "L3", "to_layer": "L4", "one_time_cost_tokens": 535},
        }
        with self.assertRaises(measure.MeasurementError):
            measure.validate_amortization(entries, self.ARTIFACTS)

    def test_rejects_sunk_schema_charged_to_a_projection_transition(self):
        # Sorted after "alias", so a gate that checks only the first entry
        # admits it.
        entries = {
            "alias": {"from_layer": "L4", "to_layer": "L5", "one_time_cost_tokens": 21},
            "projection": {"from_layer": "L1", "to_layer": "L3", "one_time_cost_tokens": 535},
        }
        with self.assertRaises(measure.MeasurementError):
            measure.validate_amortization(entries, self.ARTIFACTS)

    def test_rejects_an_alias_transition_charged_nothing(self):
        # The gate must reject under-charging too, not only over-charging.
        entries = {"alias": {"from_layer": "L4", "to_layer": "L5", "one_time_cost_tokens": 0}}
        with self.assertRaises(measure.MeasurementError):
            measure.validate_amortization(entries, self.ARTIFACTS)

    def test_rejects_an_entry_that_does_not_name_its_transition(self):
        entries = {"compact": {"one_time_cost_tokens": 0}}
        with self.assertRaises(measure.MeasurementError):
            measure.validate_amortization(entries, self.ARTIFACTS)

    def test_rejects_an_undeclared_layer(self):
        with self.assertRaises(measure.MeasurementError):
            measure.incremental_session_artifacts("L4", "L9")


def valid_probe():
    with open(os.path.join(ROOT, "mcp", "spec-probe.json")) as fh:
        return json.load(fh)


class SpecProbeFreshnessTest(unittest.TestCase):
    """The contract-shape gate can require a protocol revision to be present; it
    cannot know whether that revision is still current. Freshness rests on a
    dated primary-source read, and that read has to carry a control: an outage
    that 404s everything would otherwise make every real revision look absent,
    and a failed read is not the same fact as an absence."""

    def test_accepts_the_recorded_probe_and_the_current_pin(self):
        measure.validate_spec_probe(valid_probe(), valid_contract())

    def test_the_shipped_contract_is_pinned_to_the_probed_latest_revision(self):
        self.assertEqual(valid_contract()["protocol_version"],
                         valid_probe()["latest_revision"])

    def test_rejects_a_contract_pinned_to_a_superseded_revision(self):
        contract = valid_contract()
        contract["protocol_version"] = "2025-06-18"
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(valid_probe(), contract)

    def test_rejects_a_contract_pinned_to_the_revision_before_latest(self):
        contract = valid_contract()
        contract["protocol_version"] = "2025-11-25"
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(valid_probe(), contract)

    def test_rejects_a_contract_pinned_to_a_fabricated_future_revision(self):
        contract = valid_contract()
        contract["protocol_version"] = "2099-01-01"
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(valid_probe(), contract)

    def test_rejects_a_probe_whose_control_revision_resolved(self):
        # The site answered 200 for a revision that cannot exist, so it answers
        # for anything and the probe establishes nothing.
        probe = valid_probe()
        probe["control_absent_revision_status"] = 200
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(probe, valid_contract())

    def test_rejects_a_probe_whose_control_read_failed(self):
        # A 503 on the control is a failed read, not an absence.
        probe = valid_probe()
        probe["control_absent_revision_status"] = 503
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(probe, valid_contract())

    def test_rejects_a_probe_that_could_not_read_the_latest_revision(self):
        probe = valid_probe()
        probe["latest_revision_status"] = 404
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(probe, valid_contract())

    def test_rejects_an_undated_probe(self):
        probe = valid_probe()
        del probe["probed_at"]
        with self.assertRaises(measure.MeasurementError):
            measure.validate_spec_probe(probe, valid_contract())

    def test_contract_gate_requires_the_probe_to_be_cited(self):
        contract = valid_contract()
        del contract["spec_probe"]
        with self.assertRaises(measure.MeasurementError):
            measure.validate_mcp_contract(contract)


class MCPProfileMarginTest(unittest.TestCase):
    """"MCP's tool definitions cost more than schema()" has no profile-independent
    answer. Both profiles are published and the report must say so when the
    direction flips, instead of quoting whichever profile favours the argument."""

    def test_both_profiles_are_measured_for_every_contract(self):
        results = measure.run(ROOT)
        mcp = results["mcp"]
        self.assertEqual(sorted(mcp["profiles"]), ["minimal", "structured"])
        for key, row in sorted(mcp["session_constant_margin"].items()):
            self.assertEqual(sorted(row), ["minimal", "structured"], key)
            for profile, m in sorted(row.items()):
                self.assertGreater(m["mcp_tools_list_tokens"], 0, "%s/%s" % (key, profile))
                self.assertEqual(m["mcp_minus_dsl_tokens"],
                                 m["mcp_tools_list_tokens"] - m["dsl_schema_tokens"])
                self.assertEqual(m["mcp_cheaper"],
                                 m["mcp_tools_list_tokens"] < m["dsl_schema_tokens"])

    def test_the_scenario_margin_inverts_between_profiles(self):
        # This is the finding: on the minimal profile MCP is cheaper than
        # schema(); on the structured profile it is not. A single number cannot
        # carry the claim in either direction.
        results = measure.run(ROOT)
        row = results["mcp"]["session_constant_margin"]["scenario"]
        self.assertTrue(row["minimal"]["mcp_cheaper"])
        self.assertFalse(row["structured"]["mcp_cheaper"])
        self.assertGreater(row["structured"]["mcp_tools_list_tokens"],
                           row["minimal"]["mcp_tools_list_tokens"])

    def test_no_profile_reaches_the_repositorys_dead_weight_claim(self):
        # SKILL.md claims MCP tool definitions cost ~2,000-3,000 tokens. Nothing
        # measured here supports that, on either profile, for either contract.
        results = measure.run(ROOT)
        for key, row in sorted(results["mcp"]["session_constant_margin"].items()):
            for profile, m in sorted(row.items()):
                self.assertLess(m["mcp_tools_list_tokens"], 2000,
                                "%s/%s reaches the dead-weight band" % (key, profile))

    def test_both_profiles_still_carry_field_projection(self):
        # The structured profile must not quietly become a different capability.
        contract = valid_contract()
        for key, spec in sorted(contract["contracts"].items()):
            for profile, rel in sorted(spec["tools_list_result_profiles"].items()):
                with open(os.path.join(ROOT, rel)) as fh:
                    tools = json.load(fh)["result"]["tools"]
                self.assertEqual(len(tools), spec["tool_count"], "%s/%s" % (key, profile))
                for tool in tools:
                    op = tool["name"].split("_", 1)[1]
                    if op in measure_projection_ops():
                        self.assertIn("fields", tool["inputSchema"]["properties"],
                                      "%s/%s %s cannot project" % (key, profile, tool["name"]))
