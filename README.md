# Agent-Facing API

Design pattern for building agent-optimized CLI query layers. Two-layer read approach that minimizes token overhead without adding infrastructure.

## The Problem

CLI tools built for humans produce verbose, formatted output with ANSI colors, alignment padding, and all fields included. When AI agents consume this output, they pay 1.5-5x more tokens than necessary. MCP servers fix the format but add ~2,200 tokens of session overhead (tool definitions) and don't support batching.

## The Solution

Two read layers on top of your existing CLI:

| Layer | Purpose | Output | Example |
|-------|---------|--------|---------|
| **Mini-Query DSL** | Structured reads | JSON or compact tabular | `mytool q 'get(ID) { status assignee }'` |
| **Scoped Grep** | Full-text search | JSON or grouped-by-file text | `mytool grep "pattern" --file progress.md` |

Writes stay as regular CLI commands. No new processes, no session overhead, no extra tool definitions.

### Why Not MCP?

- **Session overhead:** ~2,200 tokens of tool definitions loaded per session
- **No batching:** each query is a separate tool call (~80 tok framing each)
- **Break-even:** ~293 queries per session (unrealistic for typical agent work)
- **Identical output:** MCP and DSL return the same JSON when backed by the same field selection engine

See [references/comparison-example.md](references/comparison-example.md) for a detailed measurement on a real 346-element board.

## Output Modes

The library supports two output modes via `OutputMode`:

| Mode | Format | Token cost |
|------|--------|------------|
| `HumanReadable` (default) | Standard JSON | Baseline |
| `LLMReadable` | Compact tabular text | ~30-50% fewer tokens |

Set the default at schema creation with `WithOutputMode(agentquery.LLMReadable)`, or override per-call with the `--format` CLI flag (`json` or `compact`).

**Compact output examples:**

```
# List: CSV-style header + rows
id,name,status,assignee
task-1,Auth service refactor,in-progress,alice
task-2,Dashboard performance,todo,bob

# Single element: key:value pairs
id:task-1
name:Auth service refactor
status:in-progress

# Search: grouped by file
README.md
  3: matching line
  4  context line
```

## Quick Start

Read `SKILL.md` for the full pattern specification:

- **Layer 1: DSL** — syntax template, implementation checklist, token budgets
- **Layer 2: Grep** — scoping, filters, when to use vs DSL
- **Layer 3: CLI** — writes stay as commands
- **Anti-patterns** — common mistakes and their costs
- **Implementation guide** — architecture, parser tips, Go project structure

## AI Agent Skill Setup

This repo is a skill for AI coding agents (Claude Code, Codex CLI, and similar tools).

```bash
# Clone
git clone <repo-url> ~/src/skill-agent-facing-api

# Symlink for Claude Code
mkdir -p ~/.claude/skills
ln -s ~/src/skill-agent-facing-api ~/.claude/skills/agent-facing-api

# Symlink for Codex CLI
mkdir -p ~/.codex/skills
ln -s ~/src/skill-agent-facing-api ~/.codex/skills/agent-facing-api
```

## References

| File | Description |
|------|-------------|
| `SKILL.md` | Full pattern specification — DSL design, grep scoping, output modes, implementation guide |
| `assets/dsl-parser.go` | Reference implementation: tokenizer + recursive descent parser + AST |
| `assets/field-selector.go` | Reference implementation: field projection with presets |
| `assets/scoped-grep.go` | Reference implementation: scoped regex search with context lines |
| `assets/query-patterns.md` | Query catalog: inputs, expected JSON, anti-patterns |
| `references/comparison-example.md` | Real-world token measurement: MCP vs DSL vs Grep on a 346-element task board |

## License

MIT
