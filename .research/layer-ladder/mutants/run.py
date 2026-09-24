#!/usr/bin/env python3
"""Narrowing-mutant harness for the layer-ladder gates.

Each mutant leaves its gate in place and weakens it just enough to admit one
member of the class the gate exists to reject. A mutant is KILLED when the named
test fails and the rest of the suite still runs; it SURVIVES when the suite
stays green, which means the gate's bound is not actually tested.

The harness runs the behavioural suites — `go test ./...` and the Python
unittest suite — not a static check over the source.

Run from .research/layer-ladder:

    /usr/bin/python3 mutants/run.py
    /usr/bin/python3 mutants/run.py --only M1
"""

from __future__ import print_function

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
LADDER = os.path.dirname(HERE)
CHECKOUT = os.path.dirname(os.path.dirname(LADDER))
PY_BIN = "/usr/bin/python3"

MUTANTS = [
    {
        "id": "M1",
        "file": ".research/layer-ladder/ladder.go",
        "gate": "VerifyPreservation record coverage",
        "narrows_to": "compares only the first record; every later record is admitted unchecked",
        "find": "\tcomparedRecords := canonical\n",
        "replace": "\tcomparedRecords := canonical[:1]\n",
        "expect_fail": "TestVerifyPreservationRejectsAlteredLastRecord",
        "suite": "go",
    },
    {
        "id": "M2",
        "file": ".research/layer-ladder/ladder.go",
        "gate": "VerifyPreservation field coverage",
        "narrows_to": "compares only the id field; status, assignee and priority are admitted unchecked",
        "find": "\tcomparedFields := ScenarioFields\n",
        "replace": "\tcomparedFields := ScenarioFields[:1]\n",
        "expect_fail": "TestVerifyPreservationRejectsAlteredPriorityValue",
        "suite": "go",
    },
    {
        "id": "M3",
        "file": ".research/layer-ladder/ladder.go",
        "gate": "ApplyAliasHeader shortening check",
        "narrows_to": "admits an alias exactly as long as the column it replaces",
        "find": "\t\tif len(alias) >= len(col) {\n",
        "replace": "\t\tif len(alias) > len(col) {\n",
        "expect_fail": "TestApplyAliasHeaderRejectsNonShorteningAlias",
        "suite": "go",
    },
    {
        "id": "M4",
        "file": ".research/layer-ladder/ladder.go",
        "gate": "ApplyAliasHeader clash detection",
        "narrows_to": "only the first column's alias is remembered, so a clash between later columns is admitted",
        "find": "\t\tseen[alias] = col\n",
        "replace": "\t\tif len(seen) == 0 {\n\t\t\tseen[alias] = col\n\t\t}\n",
        "expect_fail": "TestApplyAliasHeaderRejectsDuplicateAliasBetweenLaterColumns",
        "suite": "go",
    },
    {
        "id": "M5",
        "file": ".research/layer-ladder/measure.py",
        "gate": "validate_manifest scale coverage",
        "narrows_to": "validates only the first scale; later scales are admitted unchecked",
        "find": "    for scale in scales:\n",
        "replace": "    for scale in scales[:1]:\n",
        "expect_fail": "test_rejects_digest_mismatch_at_last_scale",
        "suite": "python",
    },
    {
        "id": "M6",
        "file": ".research/layer-ladder/measure.py",
        "gate": "validate_manifest digest consensus coverage",
        "narrows_to": "the final layer is left out of the same-records consensus",
        "find": "        consensus = sorted(set(digests.values()))\n",
        "replace": "        consensus = sorted(set(digests[layer] for layer in declared[:-1]))\n",
        "expect_fail": "test_rejects_digest_mismatch_on_last_layer",
        "suite": "python",
    },
    {
        "id": "M7",
        "file": ".research/layer-ladder/measure.py",
        "gate": "validate_mcp_contract host_measured check",
        "narrows_to": "admits an explicit host_measured=true claim while still rejecting a missing value",
        "find": '    if contract.get("host_measured") is not False:\n',
        "replace": '    if contract.get("host_measured") not in (False, True):\n',
        "expect_fail": "test_rejects_contract_claiming_host_measured",
        "suite": "python",
    },
    {
        "id": "M8",
        "file": ".research/layer-ladder/measure.py",
        "gate": "reconcile_ladder closure check",
        "narrows_to": "tolerates a five-token discrepancy between the summed steps and the final rung",
        "find": "    if running != final_tokens:\n",
        "replace": "    if abs(running - final_tokens) > 5:\n",
        "expect_fail": "test_rejects_off_by_one_incremental_sum",
        "suite": "python",
    },
    {
        "id": "M9",
        "file": ".research/layer-ladder/measure.py",
        "gate": "MCP per-call overhead coverage",
        "narrows_to": "prices only the quote-free compact payload, so the envelope looks like a fixed cost",
        "find": 'MCP_PAYLOAD_LAYERS = ("L1", "L3", "L4")\n',
        "replace": 'MCP_PAYLOAD_LAYERS = ("L4",)\n',
        "expect_fail": "test_mcp_overhead_is_not_constant_across_payload_formats",
        "suite": "python",
    },
    {
        "id": "M10",
        "file": ".research/layer-ladder/measure.py",
        "gate": "build_ladder percentage denominator (the published number)",
        "narrows_to": "the step percentage is denominated in the ladder base instead of the previous rung; every token count stays correct and the ladder still closes",
        "find": '            "incremental_saved_pct_of_previous": pct(saved, denom),\n',
        "replace": '            "incremental_saved_pct_of_previous": pct(saved, tokens[layer_ids[0]]),\n',
        "expect_fail": "test_published_percentages_are_denominated_in_the_stated_rung",
        "suite": "python",
    },
    {
        "id": "M11",
        "file": ".research/layer-ladder/measure.py",
        "gate": "reconcile_percentages step coverage",
        "narrows_to": "checks only the first step, where the previous rung IS the base, so every later mis-denominated percentage is admitted",
        "find": "    for row in steps:\n",
        "replace": "    for row in steps[:1]:\n",
        "expect_fail": "test_rejects_base_denominated_percentage_on_a_later_step",
        "suite": "python",
    },
    {
        "id": "M12",
        "file": ".research/layer-ladder/measure.py",
        "gate": "reconcile_percentages final-step coverage",
        "narrows_to": "drops the last step from the percentage check, so the final rung's published percentage is unguarded",
        "find": "    for row in steps:\n",
        "replace": "    for row in steps[:-1]:\n",
        "expect_fail": "test_rejects_base_denominated_percentage_on_the_last_step",
        "suite": "python",
    },
    {
        "id": "M13",
        "file": ".research/layer-ladder/measure.py",
        "gate": "incremental_session_artifacts sunk-cost cancellation",
        "narrows_to": "treats the first artifact the source rung already holds as not held, so the schema() roundtrip is re-charged to a transition between two agentquery rungs — the exact rev2 defect",
        "find": "    held = LAYER_SESSION_PREREQUISITES[from_layer]\n",
        "replace": "    held = LAYER_SESSION_PREREQUISITES[from_layer][1:]\n",
        "expect_fail": "test_published_entries_charge_only_what_the_transition_adds",
        "suite": "python",
    },
    {
        "id": "M14",
        "file": ".research/layer-ladder/measure.py",
        "gate": "validate_amortization entry coverage",
        "narrows_to": "validates only the first comparison, so a later row charging a sunk artifact is admitted",
        "find": "    for key in sorted(entries):\n",
        "replace": "    for key in sorted(entries)[:1]:\n",
        "expect_fail": "test_rejects_sunk_schema_charged_to_a_projection_transition",
        "suite": "python",
    },
    {
        "id": "M15",
        "file": ".research/layer-ladder/measure.py",
        "gate": "validate_spec_probe revision freshness",
        "narrows_to": "still rejects a fabricated future revision but admits any superseded one",
        "find": '    if pinned != probe["latest_revision"]:\n',
        "replace": '    if pinned > probe["latest_revision"]:\n',
        "expect_fail": "test_rejects_a_contract_pinned_to_a_superseded_revision",
        "suite": "python",
    },
    {
        "id": "M16",
        "file": ".research/layer-ladder/measure.py",
        "gate": "validate_spec_probe absent-revision control",
        "narrows_to": "still rejects a failed control read but admits a control that RESOLVED, i.e. a probe that cannot tell an absent revision from a catch-all",
        "find": '    if probe["control_absent_revision_status"] != 404:\n',
        "replace": '    if probe["control_absent_revision_status"] not in (404, 200):\n',
        "expect_fail": "test_rejects_a_probe_whose_control_revision_resolved",
        "suite": "python",
    },
    {
        "id": "M17",
        "file": ".research/layer-ladder/measure.py",
        "gate": "MCP session-constant profile coverage",
        "narrows_to": "prices only the minimal profile, so the margin inversion under the current revision's structured-output surface is never seen",
        "find": '        for profile, tokens in sorted(s_row["tools_list_result_tokens_by_profile"].items()):\n',
        "replace": '        for profile, tokens in sorted(s_row["tools_list_result_tokens_by_profile"].items())[:1]:\n',
        "expect_fail": "test_both_profiles_are_measured_for_every_contract",
        "suite": "python",
    },
]


def copy_checkout(dest):
    shutil.copytree(
        CHECKOUT, dest,
        ignore=shutil.ignore_patterns(".git", ".temp", ".task-board", "*.log"),
    )


def run_suite(root, suite):
    ladder = os.path.join(root, ".research", "layer-ladder")
    if suite == "go":
        cmd = ["go", "test", "./...", "-count=1", "-v"]
    else:
        cmd = [PY_BIN, "-m", "unittest", "discover", "-s", "tests", "-v"]
    proc = subprocess.Popen(cmd, cwd=ladder, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT)
    out, _ = proc.communicate()
    return proc.returncode, out.decode("utf-8", "replace")


def test_failed(output, suite, name):
    if suite == "go":
        return ("--- FAIL: %s" % name) in output
    # unittest -v prints "name (module.Class) ... FAIL" or an ERROR/FAIL block.
    for line in output.splitlines():
        if line.startswith("FAIL: " + name) or line.startswith("ERROR: " + name):
            return True
        if line.startswith(name + " ") and line.rstrip().endswith(("FAIL", "ERROR")):
            return True
    return False


def baseline():
    results = {}
    for suite in ("go", "python"):
        code, out = run_suite(CHECKOUT, suite)
        results[suite] = code
        if code != 0:
            print("baseline %s suite is not green (exit %d); fix that first" % (suite, code),
                  file=sys.stderr)
            print(out[-2000:], file=sys.stderr)
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--only", action="append", default=None, help="mutant id to run")
    parser.add_argument("--json", default=None, help="write a machine-readable report here")
    args = parser.parse_args()

    base = baseline()
    if any(code != 0 for code in base.values()):
        return 2
    print("baseline: go exit 0, python exit 0")

    selected = [m for m in MUTANTS if not args.only or m["id"] in args.only]
    report = []
    survivors = 0

    for mutant in selected:
        tmp = tempfile.mkdtemp(prefix="mutant-%s-" % mutant["id"])
        work = os.path.join(tmp, "checkout")
        try:
            copy_checkout(work)
            target = os.path.join(work, mutant["file"])
            with open(target) as fh:
                src = fh.read()
            count = src.count(mutant["find"])
            if count != 1:
                report.append(dict(mutant, outcome="NOT_APPLIED",
                                   detail="anchor matched %d times, want 1" % count))
                survivors += 1
                print("%s NOT APPLIED (anchor matched %d times)" % (mutant["id"], count))
                continue
            with open(target, "w") as fh:
                fh.write(src.replace(mutant["find"], mutant["replace"]))

            code, out = run_suite(work, mutant["suite"])
            killed = code != 0 and test_failed(out, mutant["suite"], mutant["expect_fail"])
            outcome = "KILLED" if killed else "SURVIVED"
            if not killed:
                survivors += 1
            report.append(dict(mutant, outcome=outcome, suite_exit=code))
            print("%s %s by %s (suite exit %d) — %s"
                  % (mutant["id"], outcome, mutant["expect_fail"], code, mutant["gate"]))
        finally:
            shutil.rmtree(tmp, ignore_errors=True)

    print("\n%d mutants, %d killed, %d survived" % (len(selected), len(selected) - survivors, survivors))
    if args.json:
        with open(args.json, "w") as fh:
            json.dump({"mutants": report, "survivors": survivors}, fh, indent=2)
            fh.write("\n")
    return 1 if survivors else 0


if __name__ == "__main__":
    sys.exit(main())
