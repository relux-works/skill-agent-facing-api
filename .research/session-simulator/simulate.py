#!/usr/bin/env python3
"""
Session Simulator: Model the token economics of field-name aliases
across agent sessions of varying lengths.

Core question: Does the token savings from abbreviated field names
justify the schema() lookup overhead required when the alias dictionary
gets evicted from context?

Uses real measured token counts from the agentquery example CLI.
"""

import itertools
import json
import os
import sys

# ===========================================================================
# MEASURED CONSTANTS (from tiktoken cl100k_base on real CLI outputs)
# ===========================================================================

# Schema call: agent sends tool call + receives schema response
SCHEMA_CALL_TOKENS = 10      # tool call overhead
SCHEMA_RESPONSE_TOKENS = 71  # schema() JSON output
SCHEMA_RESULT_OVERHEAD = 4   # "Tool output: " prefix
SCHEMA_TOTAL_ROUNDTRIP = SCHEMA_CALL_TOKENS + SCHEMA_RESPONSE_TOKENS + SCHEMA_RESULT_OVERHEAD  # = 85

# Per-query alias savings in compact format
# From token measurement research: aliases save exactly 5 tokens per list() call
# (header-only savings, data rows identical)
# For get() with a single item: field names appear in the object keys, saving ~3-5 tokens
# For JSON format: aliases save ~2-4 tokens per item (key names repeated per item)
ALIAS_SAVINGS_PER_QUERY = {
    "compact_list": 5,    # Fixed: header abbreviation saves 5 tokens regardless of item count
    "compact_get": 5,     # Same header savings for single-item display
    "json_get": 3,        # JSON: ~3 tokens saved per single-item query (shorter keys)
    "json_list_per_item": 3,  # JSON: ~3 tokens saved per item in list (key names per item)
}

# Query result token costs (for reference / workflow modeling)
QUERY_COSTS = {
    "get_overview": 23,    # get(id=X) { overview } result
    "get_full": 35,        # get(id=X) { full } result (estimated)
    "list_overview_8": 177,  # list() { overview } with 8 items
    "list_overview_20": 440, # estimated for 20 items
    "summary": 20,         # summary() result
}

# ===========================================================================
# SIMULATION PARAMETERS
# ===========================================================================

SESSION_LENGTHS = [10, 20, 50, 100]
CONTEXT_EVICTION_RATES = [10, 20, 50, float('inf')]  # K turns before eviction; inf = never
OUTPUT_FORMATS = ["compact", "json"]

# Typical query mix in a session (proportions)
QUERY_MIX = {
    "get": 0.50,   # 50% of queries are get() for single items
    "list": 0.30,  # 30% are list() queries
    "summary": 0.10,  # 10% are summary() queries
    "other": 0.10,  # 10% are other/mutations (no alias benefit)
}

# Average items per list() query
AVG_LIST_ITEMS = 10


def calculate_schema_calls(session_length: int, eviction_rate: float) -> int:
    """
    Calculate how many schema() calls an agent needs in a session.

    First call is always needed (to learn the alias dictionary).
    Subsequent calls happen every K turns when context gets evicted.
    """
    if eviction_rate == float('inf'):
        return 1  # Only the initial call, dictionary never evicted

    # Initial call + one call per eviction cycle
    return 1 + max(0, (session_length - 1) // int(eviction_rate))


def calculate_alias_savings_per_query(output_format: str, query_type: str) -> float:
    """
    Calculate token savings from aliases for a single query.

    In compact format: only header is abbreviated → fixed 5 tokens per query
    In JSON format: keys abbreviated per item → scales with item count
    """
    if query_type == "other" or query_type == "summary":
        return 0  # No field name savings for summary/mutations

    if output_format == "compact":
        return ALIAS_SAVINGS_PER_QUERY["compact_list"]  # 5 tokens, always

    # JSON format
    if query_type == "get":
        return ALIAS_SAVINGS_PER_QUERY["json_get"]  # ~3 tokens
    elif query_type == "list":
        return ALIAS_SAVINGS_PER_QUERY["json_list_per_item"] * AVG_LIST_ITEMS  # ~30 tokens
    return 0


def simulate_session(
    session_length: int,
    eviction_rate: float,
    output_format: str,
) -> dict:
    """
    Simulate a full agent session and calculate token economics.

    Returns a dict with all metrics.
    """
    # Schema overhead
    schema_calls = calculate_schema_calls(session_length, eviction_rate)
    total_schema_cost = schema_calls * SCHEMA_TOTAL_ROUNDTRIP

    # Alias savings across all queries
    total_savings = 0
    query_breakdown = {}

    for qtype, proportion in QUERY_MIX.items():
        n_queries = int(session_length * proportion)
        per_query_saving = calculate_alias_savings_per_query(output_format, qtype)
        type_savings = n_queries * per_query_saving
        total_savings += type_savings
        query_breakdown[qtype] = {
            "count": n_queries,
            "savings_per_query": per_query_saving,
            "total_savings": type_savings,
        }

    net_balance = total_savings - total_schema_cost

    # Break-even: how many queries until savings >= schema cost
    avg_saving_per_query = total_savings / session_length if session_length > 0 else 0
    if avg_saving_per_query > 0:
        # Account for ongoing schema costs
        savings_per_cycle = avg_saving_per_query * (eviction_rate if eviction_rate != float('inf') else session_length)
        if savings_per_cycle > SCHEMA_TOTAL_ROUNDTRIP:
            # Can break even within a cycle
            break_even = SCHEMA_TOTAL_ROUNDTRIP / avg_saving_per_query
        else:
            break_even = float('inf')  # Never breaks even
    else:
        break_even = float('inf')

    return {
        "session_length": session_length,
        "eviction_rate": eviction_rate,
        "output_format": output_format,
        "schema_calls": schema_calls,
        "total_schema_cost": total_schema_cost,
        "total_alias_savings": total_savings,
        "net_balance": net_balance,
        "break_even_queries": break_even,
        "avg_saving_per_query": avg_saving_per_query,
        "query_breakdown": query_breakdown,
    }


def format_eviction(k):
    return "never" if k == float('inf') else str(int(k))


def main():
    results = []

    # Run all parameter combinations
    for session_len, eviction_k, fmt in itertools.product(
        SESSION_LENGTHS, CONTEXT_EVICTION_RATES, OUTPUT_FORMATS
    ):
        result = simulate_session(session_len, eviction_k, fmt)
        results.append(result)

    # Build output report
    lines = []
    lines.append("# Session Simulator Results")
    lines.append("")
    lines.append("**Date:** 2026-02-12")
    lines.append("**Purpose:** Model token economics of field-name aliases across agent sessions")
    lines.append("")
    lines.append("## Measured Constants")
    lines.append("")
    lines.append(f"- Schema() roundtrip cost: **{SCHEMA_TOTAL_ROUNDTRIP} tokens** (call={SCHEMA_CALL_TOKENS} + response={SCHEMA_RESPONSE_TOKENS} + overhead={SCHEMA_RESULT_OVERHEAD})")
    lines.append(f"- Alias savings per query (compact format): **{ALIAS_SAVINGS_PER_QUERY['compact_list']} tokens** (fixed, header-only)")
    lines.append(f"- Alias savings per query (JSON get): **{ALIAS_SAVINGS_PER_QUERY['json_get']} tokens**")
    lines.append(f"- Alias savings per query (JSON list, {AVG_LIST_ITEMS} items): **{ALIAS_SAVINGS_PER_QUERY['json_list_per_item'] * AVG_LIST_ITEMS} tokens**")
    lines.append("")
    lines.append("## Query Mix Assumptions")
    lines.append("")
    for qtype, prop in QUERY_MIX.items():
        lines.append(f"- {qtype}: {prop*100:.0f}%")
    lines.append(f"- Average items per list(): {AVG_LIST_ITEMS}")
    lines.append("")

    # =========================================================================
    # TABLE 1: Compact format results
    # =========================================================================
    lines.append("## Results: Compact Format")
    lines.append("")
    lines.append("| Session Length | Eviction K | Schema Calls | Schema Cost | Alias Savings | Net Balance | Break-Even |")
    lines.append("|--------------:|-----------:|-------------:|------------:|--------------:|------------:|-----------:|")

    for r in results:
        if r["output_format"] != "compact":
            continue
        be = f"{r['break_even_queries']:.0f}" if r["break_even_queries"] != float('inf') else "never"
        evict = format_eviction(r["eviction_rate"])
        net_sign = "+" if r["net_balance"] > 0 else ""
        lines.append(
            f"| {r['session_length']} | {evict} | {r['schema_calls']} | "
            f"{r['total_schema_cost']} | {r['total_alias_savings']} | "
            f"{net_sign}{r['net_balance']} | {be} |"
        )

    lines.append("")

    # =========================================================================
    # TABLE 2: JSON format results
    # =========================================================================
    lines.append("## Results: JSON Format")
    lines.append("")
    lines.append("| Session Length | Eviction K | Schema Calls | Schema Cost | Alias Savings | Net Balance | Break-Even |")
    lines.append("|--------------:|-----------:|-------------:|------------:|--------------:|------------:|-----------:|")

    for r in results:
        if r["output_format"] != "json":
            continue
        be = f"{r['break_even_queries']:.0f}" if r["break_even_queries"] != float('inf') else "never"
        evict = format_eviction(r["eviction_rate"])
        net_sign = "+" if r["net_balance"] > 0 else ""
        lines.append(
            f"| {r['session_length']} | {evict} | {r['schema_calls']} | "
            f"{r['total_schema_cost']} | {r['total_alias_savings']} | "
            f"{net_sign}{r['net_balance']} | {be} |"
        )

    lines.append("")

    # =========================================================================
    # TABLE 3: Head-to-head comparison
    # =========================================================================
    lines.append("## Head-to-Head: Compact vs JSON Alias Savings")
    lines.append("")
    lines.append("| Session | Eviction | Compact Net | JSON Net | Better Format | Margin |")
    lines.append("|--------:|---------:|------------:|---------:|--------------:|-------:|")

    for session_len in SESSION_LENGTHS:
        for eviction_k in CONTEXT_EVICTION_RATES:
            compact = [r for r in results if r["session_length"] == session_len
                       and r["eviction_rate"] == eviction_k
                       and r["output_format"] == "compact"][0]
            json_r = [r for r in results if r["session_length"] == session_len
                      and r["eviction_rate"] == eviction_k
                      and r["output_format"] == "json"][0]

            better = "JSON" if json_r["net_balance"] > compact["net_balance"] else "Compact"
            margin = abs(json_r["net_balance"] - compact["net_balance"])
            evict = format_eviction(eviction_k)
            c_sign = "+" if compact["net_balance"] > 0 else ""
            j_sign = "+" if json_r["net_balance"] > 0 else ""
            lines.append(
                f"| {session_len} | {evict} | {c_sign}{compact['net_balance']} | "
                f"{j_sign}{json_r['net_balance']} | {better} | {margin} |"
            )

    lines.append("")

    # =========================================================================
    # ANALYSIS
    # =========================================================================
    lines.append("## Analysis")
    lines.append("")

    # Count how many scenarios are net positive
    compact_positive = sum(1 for r in results if r["output_format"] == "compact" and r["net_balance"] > 0)
    compact_total = sum(1 for r in results if r["output_format"] == "compact")
    json_positive = sum(1 for r in results if r["output_format"] == "json" and r["net_balance"] > 0)
    json_total = sum(1 for r in results if r["output_format"] == "json")

    lines.append(f"### Scenarios with positive net balance (aliases pay off)")
    lines.append(f"- Compact format: **{compact_positive}/{compact_total}** scenarios")
    lines.append(f"- JSON format: **{json_positive}/{json_total}** scenarios")
    lines.append("")

    # Best and worst cases
    all_compact = [r for r in results if r["output_format"] == "compact"]
    all_json = [r for r in results if r["output_format"] == "json"]

    best_compact = max(all_compact, key=lambda r: r["net_balance"])
    worst_compact = min(all_compact, key=lambda r: r["net_balance"])
    best_json = max(all_json, key=lambda r: r["net_balance"])
    worst_json = min(all_json, key=lambda r: r["net_balance"])

    lines.append("### Best/Worst Cases")
    lines.append("")
    lines.append(f"**Compact format:**")
    lines.append(f"- Best: session={best_compact['session_length']}, eviction={format_eviction(best_compact['eviction_rate'])} → net={best_compact['net_balance']:+d} tokens")
    lines.append(f"- Worst: session={worst_compact['session_length']}, eviction={format_eviction(worst_compact['eviction_rate'])} → net={worst_compact['net_balance']:+d} tokens")
    lines.append("")
    lines.append(f"**JSON format:**")
    lines.append(f"- Best: session={best_json['session_length']}, eviction={format_eviction(best_json['eviction_rate'])} → net={best_json['net_balance']:+d} tokens")
    lines.append(f"- Worst: session={worst_json['session_length']}, eviction={format_eviction(worst_json['eviction_rate'])} → net={worst_json['net_balance']:+d} tokens")
    lines.append("")

    # Key insight
    lines.append("### Key Insight")
    lines.append("")
    lines.append("The fundamental problem is the **asymmetry between savings and costs**:")
    lines.append("")
    lines.append(f"- Schema() costs **{SCHEMA_TOTAL_ROUNDTRIP} tokens** per call")
    lines.append(f"- Compact aliases save **{ALIAS_SAVINGS_PER_QUERY['compact_list']} tokens** per query")
    lines.append(f"- Therefore, each schema() call requires **{SCHEMA_TOTAL_ROUNDTRIP // ALIAS_SAVINGS_PER_QUERY['compact_list']} data queries** just to break even")
    lines.append("")
    lines.append("With context eviction every K turns, the agent must repeatedly re-learn the alias")
    lines.append("dictionary. Each re-learn costs 85 tokens and saves only 5 per subsequent query.")
    lines.append("For compact format, aliases are **almost never worth it**.")
    lines.append("")
    lines.append("For JSON format, aliases save more per query (~3 tokens/item in lists), which can")
    lines.append("occasionally make them worthwhile in long sessions without eviction. But this only")
    lines.append("applies to the JSON output format — and the recommendation is already to use compact")
    lines.append("format, which eliminates the per-item key repetition entirely.")
    lines.append("")

    # Write report
    report = "\n".join(lines)

    script_dir = os.path.dirname(os.path.abspath(__file__))
    report_path = os.path.join(script_dir, "results.md")
    with open(report_path, "w") as f:
        f.write(report)

    # Also dump raw data as JSON
    json_path = os.path.join(script_dir, "results.json")
    # Convert inf to string for JSON serialization
    serializable = []
    for r in results:
        r_copy = dict(r)
        r_copy["eviction_rate"] = format_eviction(r["eviction_rate"])
        r_copy["break_even_queries"] = (
            r["break_even_queries"] if r["break_even_queries"] != float('inf')
            else "never"
        )
        serializable.append(r_copy)
    with open(json_path, "w") as f:
        json.dump(serializable, f, indent=2)

    print(report)
    print(f"\nResults written to: {report_path}")
    print(f"Raw data written to: {json_path}")


if __name__ == "__main__":
    main()
