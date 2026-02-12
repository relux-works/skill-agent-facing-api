# Research: Pagination & Count Discoverability

**Date:** 2026-02-13
**Context:** STORY-260213-e59yqs — should pagination/count be exposed via schema() or just in SKILL.md?

## Current State

### schema() returns:
- Operation names (sorted list)
- Field names (in registration order)
- Preset definitions (name -> expanded field list)
- Default field preset

### schema() does NOT expose:
- Operation parameters (names, types, defaults, constraints)
- Parameter descriptions
- Examples
- Return value structure

### How params work today:
- Parser accepts any param name — no validation at parse time
- Validation is fully delegated to operation handlers
- Handlers manually extract params from `ctx.Statement.Args`
- No registry of valid params per operation

## Decision: Both (schema primary, SKILL.md supplementary)

### Runtime discovery via schema() — PRIMARY
- Agent calls `schema()` once per session, learns everything
- Self-documenting — no prompt bloat
- +200 tokens per schema() call, but saves 3-5K tokens from not needing full SKILL.md in prompt
- Net savings: ~2,800-4,800 tokens per session
- Aligns with "agent should understand from schema() alone" goal

### SKILL.md — SUPPLEMENTARY
- Human-readable tutorial/reference
- Examples for agents to learn patterns
- Not the authority — schema() is

## Implementation Design

New types needed:
```go
type ParameterDef struct {
    Name        string `json:"name"`
    Type        string `json:"type"`       // "string", "int", "bool"
    Optional    bool   `json:"optional"`
    Default     any    `json:"default,omitempty"`
    Description string `json:"description,omitempty"`
}

type OperationMetadata struct {
    Description string         `json:"description,omitempty"`
    Parameters  []ParameterDef `json:"parameters,omitempty"`
    Examples    []string       `json:"examples,omitempty"`
}
```

New registration method:
```go
func (s *Schema[T]) OperationWithMetadata(name string, handler OperationHandler[T], meta OperationMetadata)
```

Backwards compatible — existing `Operation()` works unchanged, metadata is optional.

## Related Stories
- STORY-260213-kgw7l0 — skip/take pagination
- STORY-260213-ajyfs9 — count operation
- STORY-260213-e59yqs — schema introspection (this research)
- STORY-260213-2dgd4o — update skill docs
