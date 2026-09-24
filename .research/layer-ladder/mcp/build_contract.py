#!/usr/bin/env python3
"""Derive synthetic MCP tool contracts from real agentquery schema() output.

This does NOT measure a running MCP host. It builds the smallest faithful
`tools/list` result that exposes the SAME operations, the SAME parameters and
the SAME descriptions the agentquery schema already publishes, re-shaped into
the MCP `Tool` structure. Deriving the contract instead of inventing one keeps
the comparison from being a strawman: neither side is given extra operations,
longer descriptions or a richer contract than the other.

Protocol shape follows the current MCP specification revision, "Server
Features / Tools". The revision is not hardcoded as a belief: mcp/spec-probe.json
records a dated primary-source read of which revision /specification/latest
resolves to, together with a control that proves the probe can tell an absent
revision from a soft 404. measure.py refuses a contract pinned to anything other
than the probed latest revision.

Two contracts are emitted, each in two profiles:

  scenario    — the 3-operation schema this measurement's scenario registers.
  example-cli — the 9-operation contract the shipped example CLI publishes
                (6 read operations plus 3 mutations). A realistic tool surface.

  minimal     — only the fields the Tool interface requires plus the ones the
                agentquery schema already publishes (name, title, description,
                inputSchema, annotations). This is a FLOOR, not a typical
                server: it is byte-identical under 2025-06-18 and 2026-07-28
                because both revisions require only name and inputSchema.
  structured  — the same tools plus the outputSchema a server would publish to
                return structured content, which is the surface the current
                revision expands. Derived from the same schema() field list, so
                it is still not a strawman.

Run from .research/layer-ladder:

    /usr/bin/python3 mcp/build_contract.py
"""

import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)

PROTOCOL_VERSION = "2026-07-28"
SPEC_URL = "https://modelcontextprotocol.io/specification/2026-07-28/server/tools"
SPEC_PROBE = "mcp/spec-probe.json"

# Operations that return records and therefore accept a projection.
PROJECTING_OPERATIONS = {"get", "list"}

# minimal is a floor; structured is what the current revision's structured-content
# surface expects a projecting read tool to publish.
PROFILES = ("minimal", "structured")

JSON_TYPES = {"string": "string", "int": "integer", "bool": "boolean"}


def json_type_for(param_type):
    return JSON_TYPES.get(param_type, "string")


def build_input_schema(meta, fields, presets, projecting):
    properties = {}
    required = []
    for param in meta.get("parameters", []):
        name = param["name"]
        prop = {
            "type": json_type_for(param.get("type", "string")),
            "description": param.get("description", ""),
        }
        if param.get("default") is not None:
            prop["default"] = param["default"]
        if param.get("enum"):
            prop["enum"] = param["enum"]
        properties[name] = prop
        # agentquery read parameters carry "optional"; mutation parameters carry
        # "required". Absent optional on a read parameter means required.
        if param.get("required") is True or param.get("optional") is False:
            required.append(name)
    if projecting:
        # MCP can carry field projection: it is an ordinary tool argument.
        # Whether a given server exposes it is a server design choice, not a
        # protocol limitation.
        properties["fields"] = {
            "type": "array",
            "items": {"type": "string", "enum": list(fields) + list(presets)},
            "description": "Field projection: return only these fields or presets. Omit for the default field set.",
        }
    schema = {"type": "object", "properties": properties}
    if required:
        schema["required"] = sorted(set(required))
    return schema


def build_output_schema(schema, op):
    """The structured-content schema a server would publish for a record op.

    Derived from the same schema() field list the DSL already publishes, so the
    structured profile gains no information the minimal profile was denied.
    """
    record = {
        "type": "object",
        "properties": dict((name, {"type": "string"}) for name in schema.get("fields", [])),
    }
    if op == "list":
        return {
            "type": "object",
            "properties": {
                "records": {"type": "array", "items": record},
                "total": {"type": "integer", "description": "Number of matching records."},
            },
            "required": ["records"],
        }
    return record


def build_tools(schema, prefix, profile):
    fields = schema.get("fields", [])
    presets = sorted(schema.get("presets", {}) or {})
    read_meta = schema.get("operationMetadata", {}) or {}
    mutation_meta = schema.get("mutationMetadata", {}) or {}

    tools = []
    for op in sorted(schema.get("operations", [])):
        meta = read_meta.get(op, {})
        projecting = op in PROJECTING_OPERATIONS
        tool = {
            "name": prefix + op,
            "title": meta.get("description") or op,
            "description": meta.get("description", ""),
            "inputSchema": build_input_schema(meta, fields, presets, projecting),
        }
        if profile == "structured" and projecting:
            tool["outputSchema"] = build_output_schema(schema, op)
        tools.append(tool)
    for op in sorted(mutation_meta):
        meta = mutation_meta[op]
        tool = {
            "name": prefix + op,
            "title": meta.get("description") or op,
            "description": meta.get("description", ""),
            "inputSchema": build_input_schema(meta, fields, presets, False),
        }
        annotations = {}
        if meta.get("destructive") is not None:
            annotations["destructiveHint"] = bool(meta["destructive"])
        if meta.get("idempotent") is not None:
            annotations["idempotentHint"] = bool(meta["idempotent"])
        if annotations:
            tool["annotations"] = annotations
        tools.append(tool)
    return tools


CONTRACTS = [
    {
        "key": "scenario",
        "schema_file": "schema-response.json",
        "prefix": "tasks_",
        "note": "The exact operation surface this measurement's scenario registers.",
    },
    {
        "key": "example-cli",
        "schema_file": "example-schema-response.json",
        "prefix": "taskdemo_",
        "note": "The shipped example CLI's operation surface: 6 read operations plus 3 mutations.",
    },
]


def main():
    out = {
        "evidence_class": "synthetic-contract",
        "protocol_version": PROTOCOL_VERSION,
        "protocol_source": SPEC_URL,
        "host_measured": False,
        "sdk_reference": "shape matches the MCP specification revision 2026-07-28 (Tool interface read from schema/2026-07-28/schema.ts); no SDK was executed and no host transcript was captured",
        "spec_probe": SPEC_PROBE,
        "projection_supported": sorted(PROJECTING_OPERATIONS),
        "profiles": list(PROFILES),
        "profile_notes": {
            "minimal": "Required Tool fields plus what agentquery schema() already publishes. A floor, identical under 2025-06-18 and 2026-07-28.",
            "structured": "The same tools plus the outputSchema a current-revision server publishes for structured content, derived from the same schema() field list.",
        },
        "limitations": [
            "tools/list is constructed here, not captured from a running server.",
            "No host was measured: real hosts add their own framing, tool-use scaffolding and system text around tool definitions, none of which is counted.",
            "Token counts are of the wire JSON only, under cl100k_base.",
            "The tool result payload is byte-identical to the DSL layer it carries, so payload cost is not a differentiator; only session-constant definition cost and per-call envelope differ.",
            "The minimal profile carries only the fields the Tool interface requires plus those agentquery already publishes. It is a floor. Both 2025-06-18 and 2026-07-28 require only name and inputSchema, so the minimal figure does not move with the revision pin - and for the same reason it does not represent what a current-revision server typically publishes.",
            "The structured profile adds the outputSchema the current revision's structured-content surface expects, derived from the same schema() field list. Real servers may also carry icons, _meta and richer annotations, none of which are counted, so the structured figure is itself a lower bound on a fully-populated current-revision contract.",
        ],
        "contracts": {},
    }

    for spec in CONTRACTS:
        schema_path = os.path.join(ROOT, "fixtures", spec["schema_file"])
        if not os.path.exists(schema_path):
            print("missing %s; run: go run ./cmd/genfixtures" % schema_path, file=sys.stderr)
            return 1
        with open(schema_path) as fh:
            schema = json.load(fh)

        profiles = {}
        tool_names = None
        for profile in PROFILES:
            tools = build_tools(schema, spec["prefix"], profile)
            list_result = {"jsonrpc": "2.0", "id": 1, "result": {"tools": tools}}
            rel = "mcp/tools-list-result-%s-%s.json" % (spec["key"], profile)
            with open(os.path.join(ROOT, rel), "w") as fh:
                json.dump(list_result, fh, indent=2)
                fh.write("\n")
            profiles[profile] = rel
            tool_names = [t["name"] for t in tools]
            print("wrote %s (%d tools, %s profile)" % (rel, len(tools), profile))

        out["contracts"][spec["key"]] = {
            "tools_list_result": profiles["minimal"],
            "tools_list_result_profiles": profiles,
            "derived_from": "fixtures/" + spec["schema_file"],
            "tool_count": len(tool_names),
            "tool_names": tool_names,
            "note": spec["note"],
        }

    with open(os.path.join(HERE, "contract.json"), "w") as fh:
        json.dump(out, fh, indent=2)
        fh.write("\n")
    print("wrote mcp/contract.json")
    return 0


if __name__ == "__main__":
    sys.exit(main())
