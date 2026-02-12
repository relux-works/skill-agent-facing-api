#!/usr/bin/env python3
"""
Measure token counts for synthetic payloads using tiktoken (cl100k_base).

Produces a comparison table showing:
- Raw token counts per variant per scale
- % savings: JSON -> compact-full
- % savings: compact-full -> compact-aliased (marginal benefit of abbreviation)
- Absolute token savings from aliases per scale
"""

import os
import tiktoken

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
SCALES = [5, 20, 100, 500]
VARIANTS = ["json", "compact-full", "compact-alias"]

enc = tiktoken.get_encoding("cl100k_base")


def count_tokens(text: str) -> int:
    return len(enc.encode(text))


def read_payload(variant: str, scale: int) -> str:
    path = os.path.join(SCRIPT_DIR, f"{variant}-{scale}.txt")
    with open(path, "r") as f:
        return f.read()


def main():
    # Collect measurements
    data = {}  # data[scale][variant] = token_count
    byte_sizes = {}  # byte_sizes[scale][variant] = byte_count

    for scale in SCALES:
        data[scale] = {}
        byte_sizes[scale] = {}
        for variant in VARIANTS:
            text = read_payload(variant, scale)
            tokens = count_tokens(text)
            data[scale][variant] = tokens
            byte_sizes[scale][variant] = len(text.encode("utf-8"))
            print(f"  {variant}-{scale}: {tokens:,} tokens ({len(text.encode('utf-8')):,} bytes)")

    print()

    # Build markdown report
    lines = []
    lines.append("# Field Alias Token Measurement Results")
    lines.append("")
    lines.append("**Date:** 2026-02-12")
    lines.append("**Encoding:** cl100k_base (GPT-4 / Claude-compatible BPE tokenizer)")
    lines.append("**Seed:** 42 (reproducible)")
    lines.append("")
    lines.append("## Methodology")
    lines.append("")
    lines.append("Generated synthetic task tracker payloads at 4 scales (5, 20, 100, 500 items)")
    lines.append("with 8 fields per item: id, name, status, assignee, description, priority, created, updated.")
    lines.append("")
    lines.append("Three format variants per scale:")
    lines.append("- **JSON**: Standard `json.dumps(items, indent=2)` — pretty-printed JSON array")
    lines.append("- **Compact-full**: CSV-style with full field names as header, data rows below")
    lines.append("- **Compact-alias**: Same CSV-style but header uses 1-char abbreviations (id->i, name->n, etc.)")
    lines.append("")
    lines.append("Token counts measured with `tiktoken` using `cl100k_base` encoding.")
    lines.append("")

    # Table 1: Raw counts
    lines.append("## 1. Raw Token Counts")
    lines.append("")
    lines.append("| Scale | JSON | Compact-Full | Compact-Alias | JSON bytes | Compact-Full bytes | Compact-Alias bytes |")
    lines.append("|------:|-----:|-------------:|--------------:|-----------:|-------------------:|--------------------:|")
    for scale in SCALES:
        j = data[scale]["json"]
        cf = data[scale]["compact-full"]
        ca = data[scale]["compact-alias"]
        jb = byte_sizes[scale]["json"]
        cfb = byte_sizes[scale]["compact-full"]
        cab = byte_sizes[scale]["compact-alias"]
        lines.append(f"| {scale} | {j:,} | {cf:,} | {ca:,} | {jb:,} | {cfb:,} | {cab:,} |")
    lines.append("")

    # Table 2: JSON -> Compact-Full savings
    lines.append("## 2. Savings: JSON to Compact-Full")
    lines.append("")
    lines.append("| Scale | JSON Tokens | Compact-Full Tokens | Saved | % Reduction |")
    lines.append("|------:|------------:|--------------------:|------:|------------:|")
    for scale in SCALES:
        j = data[scale]["json"]
        cf = data[scale]["compact-full"]
        saved = j - cf
        pct = (saved / j) * 100
        lines.append(f"| {scale} | {j:,} | {cf:,} | {saved:,} | {pct:.1f}% |")
    lines.append("")

    # Table 3: Compact-Full -> Compact-Alias savings (THE KEY METRIC)
    lines.append("## 3. Marginal Savings: Compact-Full to Compact-Alias (KEY METRIC)")
    lines.append("")
    lines.append("This measures the **incremental benefit** of abbreviating field names in the header,")
    lines.append("given that we already use compact tabular format.")
    lines.append("")
    lines.append("| Scale | Compact-Full Tokens | Compact-Alias Tokens | Tokens Saved | % Reduction | Bytes Saved |")
    lines.append("|------:|--------------------:|---------------------:|-------------:|------------:|------------:|")
    for scale in SCALES:
        cf = data[scale]["compact-full"]
        ca = data[scale]["compact-alias"]
        saved = cf - ca
        pct = (saved / cf) * 100 if cf > 0 else 0
        bytes_saved = byte_sizes[scale]["compact-full"] - byte_sizes[scale]["compact-alias"]
        lines.append(f"| {scale} | {cf:,} | {ca:,} | {saved:,} | {pct:.2f}% | {bytes_saved:,} |")
    lines.append("")

    # Table 4: Full comparison summary
    lines.append("## 4. Full Comparison Summary")
    lines.append("")
    lines.append("| Scale | JSON | Compact-Full | Compact-Alias | JSON->CF % | CF->CA % | JSON->CA % |")
    lines.append("|------:|-----:|-------------:|--------------:|-----------:|---------:|-----------:|")
    for scale in SCALES:
        j = data[scale]["json"]
        cf = data[scale]["compact-full"]
        ca = data[scale]["compact-alias"]
        j_cf = ((j - cf) / j) * 100
        cf_ca = ((cf - ca) / cf) * 100 if cf > 0 else 0
        j_ca = ((j - ca) / j) * 100
        lines.append(f"| {scale} | {j:,} | {cf:,} | {ca:,} | {j_cf:.1f}% | {cf_ca:.2f}% | {j_ca:.1f}% |")
    lines.append("")

    # Table 5: Tokens per item
    lines.append("## 5. Tokens Per Item (Amortized)")
    lines.append("")
    lines.append("| Scale | JSON/item | Compact-Full/item | Compact-Alias/item |")
    lines.append("|------:|----------:|------------------:|-------------------:|")
    for scale in SCALES:
        j = data[scale]["json"]
        cf = data[scale]["compact-full"]
        ca = data[scale]["compact-alias"]
        lines.append(f"| {scale} | {j/scale:.1f} | {cf/scale:.1f} | {ca/scale:.1f} |")
    lines.append("")

    # Analysis
    lines.append("## 6. Analysis")
    lines.append("")

    # Calculate averages for the key metric
    cf_ca_pcts = []
    cf_ca_abs = []
    j_cf_pcts = []
    for scale in SCALES:
        cf = data[scale]["compact-full"]
        ca = data[scale]["compact-alias"]
        j = data[scale]["json"]
        cf_ca_pcts.append(((cf - ca) / cf) * 100)
        cf_ca_abs.append(cf - ca)
        j_cf_pcts.append(((j - cf) / j) * 100)

    avg_j_cf = sum(j_cf_pcts) / len(j_cf_pcts)
    avg_cf_ca = sum(cf_ca_pcts) / len(cf_ca_pcts)

    lines.append("### JSON to Compact-Full: Large, Consistent Win")
    lines.append("")
    lines.append(f"Switching from JSON to compact tabular format saves **~{avg_j_cf:.0f}%** of tokens on average.")
    lines.append("This is a substantial reduction driven by eliminating:")
    lines.append("- Repeated field name keys on every item")
    lines.append("- JSON structural characters (`{{`, `}}`, `[`, `]`, `:`, `\"`)")
    lines.append("- Indentation whitespace")
    lines.append("")
    lines.append("The savings scale well: they remain consistent as item count grows,")
    lines.append("because the overhead is proportional to the number of items in JSON.")
    lines.append("")

    lines.append("### Compact-Full to Compact-Alias: Negligible Marginal Benefit")
    lines.append("")
    lines.append(f"Abbreviating field names in the header saves **~{avg_cf_ca:.2f}%** of tokens on average.")
    lines.append(f"In absolute terms, the savings are **{cf_ca_abs[0]} tokens** (5 items) to **{cf_ca_abs[-1]} tokens** (500 items).")
    lines.append("")
    lines.append("Why so small? Because in the compact format, field names appear **only once** — in the header row.")
    lines.append("The header `id,name,status,assignee,description,priority,created,updated` is a single line")
    lines.append("consuming a fixed number of tokens regardless of how many data rows follow.")
    lines.append("Abbreviating it to `i,n,s,a,d,p,c,u` saves those few tokens once, and that's it.")
    lines.append("")
    lines.append("The data rows (which dominate the payload) are identical in both variants.")
    lines.append("")

    lines.append("### Cost-Benefit Verdict")
    lines.append("")
    lines.append("| Factor | Assessment |")
    lines.append("|--------|-----------|")
    lines.append(f"| Token savings | {avg_cf_ca:.2f}% average — negligible |")
    lines.append(f"| Absolute savings at 500 items | {cf_ca_abs[-1]} tokens — trivial |")
    lines.append("| Readability cost | High — `i,n,s,a,d,p,c,u` is unreadable without a legend |")
    lines.append("| Implementation complexity | Moderate — need alias registry, mapping, docs |")
    lines.append("| Agent confusion risk | Non-trivial — agents may misinterpret abbreviated headers |")
    lines.append("| Schema discoverability | Degraded — header no longer self-documenting |")
    lines.append("")
    lines.append("**Recommendation: Do NOT implement field name aliases.**")
    lines.append("")
    lines.append("The marginal token savings are negligible compared to the readability and complexity costs.")
    lines.append("The big win is already captured by switching from JSON to compact tabular format.")
    lines.append("Further compression efforts should target the data values themselves (e.g., date format")
    lines.append("abbreviation, status code mapping) rather than the one-time header line, though even those")
    lines.append("are unlikely to be worth the tradeoff.")
    lines.append("")

    # Write report
    report_path = os.path.join(SCRIPT_DIR, "..", "260212_field-alias-token-measurements.md")
    report_path = os.path.normpath(report_path)
    with open(report_path, "w") as f:
        f.write("\n".join(lines))
    print(f"Report written to: {report_path}")

    # Also print summary
    print("\n=== SUMMARY ===")
    print(f"JSON -> Compact-Full:  ~{avg_j_cf:.0f}% token reduction (BIG WIN)")
    print(f"Compact-Full -> Alias: ~{avg_cf_ca:.2f}% token reduction (NEGLIGIBLE)")
    print(f"Absolute alias savings: {cf_ca_abs[0]}-{cf_ca_abs[-1]} tokens across scales")


if __name__ == "__main__":
    main()
