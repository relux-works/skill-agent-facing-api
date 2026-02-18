# Agent-Facing Mutation Patterns: Research

Research into how write operations (mutations/actions) are defined, discovered, and invoked across major agent-facing interfaces and frameworks.

---

## 1. MCP (Model Context Protocol) Tool Definitions

**Source**: [MCP Specification 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)

MCP is the most relevant reference because it explicitly separates reads (Resources) from writes (Tools) and provides the richest annotation system for mutation behavior.

### Tool Definition Schema

```json
{
  "name": "update_task",
  "title": "Update Task",
  "description": "Updates an existing task's fields. Only provided fields are changed.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "id": { "type": "string", "description": "Task ID" },
      "status": { "type": "string", "enum": ["todo", "in_progress", "done"] },
      "title": { "type": "string" }
    },
    "required": ["id"]
  },
  "outputSchema": {
    "type": "object",
    "properties": {
      "id": { "type": "string" },
      "status": { "type": "string" },
      "title": { "type": "string" },
      "updated_at": { "type": "string" }
    },
    "required": ["id", "status", "title", "updated_at"]
  },
  "annotations": {
    "readOnlyHint": false,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": false
  }
}
```

### Key Design Patterns

**Resources vs Tools separation**:
- **Resources** = application-controlled, read-only data sources. Fetched via `resources/read` with URIs. The host application decides when to incorporate them as context.
- **Tools** = model-controlled actions. The LLM discovers and invokes them autonomously. This is where mutations live.

**Tool Annotations** (mutation behavior hints):
| Annotation | Type | Default | Meaning |
|---|---|---|---|
| `readOnlyHint` | bool | false | Tool does not modify its environment |
| `destructiveHint` | bool | true | Tool may perform destructive/irreversible updates |
| `idempotentHint` | bool | false | Repeated calls with same args have no additional effect |
| `openWorldHint` | bool | true | Tool interacts with external entities beyond the server |

These are **hints**, not guarantees. Clients SHOULD NOT trust them from untrusted servers. But for a CLI tool that we control, they provide a clean way to categorize operations.

**Discovery**: `tools/list` request with pagination. Dynamic list changes notified via `notifications/tools/list_changed`.

**Invocation**: `tools/call` with `{ name, arguments }`. Arguments is a flat JSON object matching `inputSchema`.

**Error handling** (two layers):
1. Protocol errors (JSON-RPC): unknown tool, invalid args, server errors
2. Tool execution errors: returned in result with `isError: true` and human-readable text content

**Output schema** (optional): Tools can declare `outputSchema` for structured results. When present, `structuredContent` field contains typed JSON alongside the `content` text blocks.

**Result content types**: text, image, audio, resource_link (URI to fetch later), embedded resource (inline data).

### Transferable Patterns
- **Flat inputSchema**: MCP recommends keeping schemas flat to reduce token count and LLM cognitive load
- **Annotations for behavior classification**: readOnly/destructive/idempotent is a clean taxonomy
- **Dual error layers**: schema validation errors vs runtime execution errors
- **Optional output schema**: gradual adoption, backwards compatible
- **Discovery is paginated and dynamic**: tools can change over time

---

## 2. OpenAI Function Calling / Tool Use

**Source**: [OpenAI Function Calling Guide](https://platform.openai.com/docs/guides/function-calling)

### Tool Definition Format

```json
{
  "type": "function",
  "function": {
    "name": "create_task",
    "description": "Creates a new task in the project. Returns the created task with generated ID.",
    "parameters": {
      "type": "object",
      "properties": {
        "title": {
          "type": "string",
          "description": "Task title, should be concise and actionable"
        },
        "status": {
          "type": "string",
          "enum": ["todo", "in_progress", "done"],
          "description": "Initial status. Defaults to 'todo' if omitted."
        },
        "priority": {
          "type": "integer",
          "minimum": 1,
          "maximum": 5,
          "description": "Priority level, 1=lowest, 5=highest"
        }
      },
      "required": ["title"],
      "additionalProperties": false
    },
    "strict": true
  }
}
```

### Key Design Patterns

**Strict mode** (`strict: true`):
- Guarantees the LLM's output conforms exactly to the schema (via constrained decoding)
- Requires `additionalProperties: false` on every object
- All fields must be listed in `required` (use `"type": ["string", "null"]` for optional)
- Incompatible with parallel function calling
- Recommended always for production

**Parallel function calling**:
- Model can emit multiple `tool_use` blocks in a single response
- All results returned in a single user message
- Can be disabled with `parallel_tool_calls: false`
- Tension with strict mode: can't use both simultaneously

**How the LLM decides which function to call**:
- Model receives all function schemas + conversation history
- No annotations for mutation behavior (unlike MCP)
- Decision is entirely based on `name`, `description`, and `parameters` descriptions
- OpenAI recommends max 10-20 tools per call for accuracy

**Result format**: Results are returned as plain strings in a message with `role: "tool"` and matched by `tool_call_id`.

**No output schema**: Unlike MCP, OpenAI doesn't have a formal output schema. The model just receives the string result and interprets it.

### Transferable Patterns
- **Strict mode as default**: constrained output eliminates malformed invocations
- **additionalProperties: false**: prevents the model from inventing parameters
- **Rich parameter constraints**: enum, minimum/maximum, pattern -- all help the model generate valid input
- **Description quality is everything**: OpenAI docs emphasize 3-4+ sentences per tool description
- **Parallel calls**: batch multiple independent operations in one turn

---

## 3. Anthropic Tool Use

**Source**: [Anthropic Tool Use Documentation](https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use)

### Tool Definition Format

```json
{
  "name": "update_task",
  "description": "Updates fields of an existing task. Only provided fields are modified; omitted fields remain unchanged. The task must exist -- returns an error if the ID is not found. Use get_task first if unsure about the task ID.",
  "input_schema": {
    "type": "object",
    "properties": {
      "id": { "type": "string", "description": "The task identifier (e.g., 'task-42')" },
      "status": { "type": "string", "enum": ["todo", "in_progress", "done"] },
      "title": { "type": "string", "description": "New title for the task" }
    },
    "required": ["id"]
  },
  "input_examples": [
    { "id": "task-42", "status": "done" },
    { "id": "task-7", "title": "Updated title", "status": "in_progress" }
  ]
}
```

### Key Design Patterns

**input_examples**: Anthropic's unique addition. Concrete example inputs help the model understand complex tools. Schema-validated (invalid examples rejected with 400). Adds ~20-200 tokens per example.

**Content block architecture**: Unlike OpenAI's string-based results, Anthropic uses typed content blocks:
- `tool_use` block (from model): `{ id, name, input }`
- `tool_result` block (from user): `{ tool_use_id, content, is_error }`
- Content can be text, image, or document blocks -- multimodal results

**tool_choice control**:
- `auto` (default): model decides
- `any`: must use at least one tool
- `tool`: force a specific tool
- `none`: no tool use

**Parallel tool use**: Built-in, with all results in a single user message. Can disable with `disable_parallel_tool_use`.

**Error handling**:
- `is_error: true` in tool_result for execution errors
- Model retries 2-3 times with corrections for invalid parameters
- Error content should be human-readable with corrective guidance

**Strict tool use**: Available via `strict: true` on tool definitions, guaranteeing schema-conformant inputs.

### Differences from OpenAI
- `input_schema` not `parameters` (naming)
- `input_examples` field (Anthropic only)
- Content blocks instead of plain strings for results
- `tool_choice` instead of `function_call` (semantic difference)
- Built-in tool runner SDK for automatic loop management

### Transferable Patterns
- **input_examples as documentation**: concrete examples are more helpful than abstract schema alone
- **Multimodal results**: tool results can include structured data, not just strings
- **Retry with correction**: error messages should guide the model to fix its invocation
- **Force tool use**: useful for ensuring a mutation is always routed through a specific handler

---

## 4. LangChain / LangGraph Tools

**Source**: [LangChain Tools](https://docs.langchain.com/oss/python/langchain/tools), [LangChain Reference](https://reference.langchain.com/python/langchain/tools/)

### StructuredTool Definition

```python
from langchain_core.tools import StructuredTool
from pydantic import BaseModel, Field

class UpdateTaskInput(BaseModel):
    """Input for updating an existing task."""
    id: str = Field(description="Task identifier")
    status: str | None = Field(None, description="New status",
                                enum=["todo", "in_progress", "done"])
    title: str | None = Field(None, description="New title")

class UpdateTaskOutput(BaseModel):
    """Result of task update."""
    id: str
    status: str
    title: str
    updated: bool

def update_task(id: str, status: str = None, title: str = None) -> dict:
    """Updates an existing task's fields. Only provided fields change."""
    # ... implementation

update_tool = StructuredTool.from_function(
    func=update_task,
    name="update_task",
    description="Updates an existing task. Returns the updated task.",
    args_schema=UpdateTaskInput,
    return_direct=False
)
```

### Key Design Patterns

**Pydantic as the schema layer**: `args_schema` is a Pydantic model that provides:
- JSON Schema generation (sent to the LLM)
- Runtime input validation (before the function executes)
- Type coercion and default values
- Rich field descriptions via `Field(description=...)`

**ValidationNode in LangGraph**: Dedicated graph node that validates tool call arguments against Pydantic schemas before execution. Returns validation errors back to the model for retry.

**Tool discovery via bind_tools()**: Attaches tool schemas to the LLM instance. The model receives all bound tool schemas in its context.

**@tool decorator**: Simplest path -- docstring becomes description, type hints become schema:
```python
@tool
def delete_task(id: str) -> str:
    """Permanently deletes a task. This action cannot be undone.

    Args:
        id: The task identifier to delete
    """
```

**Output schema**: `return_direct` flag controls whether the tool output goes directly to the user or back through the model.

### Transferable Patterns
- **Schema-as-code**: define schema in the same language as the handler, auto-generate JSON Schema
- **Validation as a separate concern**: ValidationNode pattern separates validation from execution
- **Docstrings as descriptions**: leverage existing documentation conventions
- **Return type schema**: typed outputs improve downstream processing

---

## 5. Agent-Facing API Design Principles for Writes

**Sources**: [Anthropic: Writing Tools for Agents](https://www.anthropic.com/engineering/writing-tools-for-agents), [Anthropic: Building Effective Agents](https://www.anthropic.com/research/building-effective-agents), [Google ADK Safety](https://google.github.io/adk-docs/safety/)

### Principle 1: Consolidate Multi-Step Operations

Don't expose raw CRUD. Instead, expose intent-level operations:

| Bad (raw CRUD) | Good (intent-level) |
|---|---|
| `list_tasks`, `update_task`, `save_task` | `complete_task(id)` |
| `get_project`, `get_members`, `add_member` | `invite_to_project(project_id, user_email)` |
| `read_file`, `modify_line`, `write_file` | `apply_edit(file, old_text, new_text)` |

Fewer tool calls = fewer tokens, fewer chances for error, faster completion.

### Principle 2: Token-Efficient Mutation Descriptions

**Schema size matters for mutations just as much as for queries**:
- Flat schemas over nested ones (reduce token count)
- Use `enum` aggressively to constrain string values
- Semantic field names reduce need for long descriptions
- Default values in descriptions eliminate common parameters

**Response format for mutation results**:
- Return the affected entity (not just "ok") -- the model needs it for subsequent reasoning
- Use compact format (key:value pairs, not verbose JSON)
- Include what changed, not the full entity

### Principle 3: Structured > Natural Language for Mutations

Agents perform dramatically better with structured mutation formats:

```
# Structured (good) -- the model fills a schema
update_task(id="task-1", status="done")

# Natural language (bad) -- ambiguous, requires parsing
"Mark task 1 as done"
```

JSON Schema gives the model a "fill in the blanks" template. The constrained output space reduces hallucination and invalid parameters.

### Principle 4: Confirmation / Safety Patterns

**MCP annotations approach**: Declare `destructiveHint: true` and let the client decide to prompt for confirmation.

**Dry-run / preview pattern**:
```
# Agent calls mutation in preview mode
update_task(id="task-1", status="done", dry_run=true)
# Response: { "preview": { "changes": [{"field":"status","from":"in_progress","to":"done"}] } }
# Agent or human confirms
update_task(id="task-1", status="done", dry_run=false)
```

**Google ADK before_tool_call callback**: Hook that runs before every tool call. Can inspect args, validate policies, and block execution.

**Idempotency keys**: Every mutation should accept an optional idempotency key to make retries safe.

### Principle 5: Error Messages as Corrective Guidance

Bad: `Error: invalid input`
Good: `Error: status must be one of [todo, in_progress, done], got "complete". Did you mean "done"?`

The error message should contain enough information for the agent to self-correct on retry.

### Principle 6: Poka-Yoke (Mistake-Proofing)

Design parameters so invalid invocations are structurally impossible:
- Use `enum` instead of free-text for categorical values
- Use `required` for mandatory fields
- Use absolute identifiers instead of relative references
- Validate at schema level, not just at runtime

---

## 6. Semantic Kernel / AutoGen (Microsoft Agent Framework)

**Sources**: [Semantic Kernel Plugins](https://learn.microsoft.com/en-us/semantic-kernel/concepts/plugins/), [AutoGen Tools](https://microsoft.github.io/autogen/stable//user-guide/core-user-guide/components/tools.html)

### Semantic Kernel Plugin Definition

```csharp
public class TaskPlugin
{
    [KernelFunction("update_task")]
    [Description("Updates an existing task's fields. Only provided fields are changed.")]
    public async Task<TaskModel?> UpdateTaskAsync(
        [Description("The task ID")] string id,
        [Description("New status")] string? status = null,
        [Description("New title")] string? title = null)
    {
        // ... implementation
    }

    [KernelFunction("delete_task")]
    [Description("Permanently deletes a task. Cannot be undone.")]
    public async Task<bool> DeleteTaskAsync(
        [Description("The task ID to delete")] string id)
    {
        // ... implementation
    }
}
```

### AutoGen Tool Definition

```python
from autogen_core.tools import FunctionTool

async def update_task(
    id: str,
    status: Annotated[str | None, "New status (todo/in_progress/done)"] = None,
    title: Annotated[str | None, "New title for the task"] = None
) -> dict:
    """Updates an existing task's fields."""
    # ... implementation

update_tool = FunctionTool(update_task, description="Updates a task's fields")
# Schema auto-generated from type annotations
print(update_tool.schema)  # JSON Schema
```

### Key Design Patterns

**Annotation-driven schema generation**:
- `[Description("...")]` in C# / `Annotated[type, "desc"]` in Python
- `[KernelFunction("name")]` for explicit naming
- Complex input objects auto-serialized to JSON Schema
- Return types also serializable for output schema

**Recommendations from Microsoft**:
- Keep schemas flat, minimize parameters
- Use primitive types where possible (reduce schema complexity)
- Provide Description for every non-obvious property
- Use snake_case for function/parameter names (LLMs trained on Python)
- Max 10-20 tools per invocation
- **Local state pattern**: store large data locally, pass state IDs to tools instead of full data (token efficiency)

**Plugin as logical grouping**: A plugin bundles related functions. In CLI terms, this maps to a command group (e.g., `tasks` plugin contains `list`, `get`, `create`, `update`, `delete`).

**FunctionChoiceBehavior**: Controls how the kernel routes tool calls:
- `Auto()` -- model decides
- `Required()` -- must call a function
- Per-function control possible

### Transferable Patterns
- **Convention over configuration**: type annotations + docstrings generate the entire schema
- **Plugin = command group**: natural mapping to CLI subcommands
- **State ID pattern**: return/accept IDs instead of full objects to reduce token usage
- **Return the modified entity**: mutations should return the result so the model has ground truth

---

## Synthesis: How a CLI DSL Should Describe and Accept Mutations

### Universal Patterns Across All Frameworks

| Pattern | MCP | OpenAI | Anthropic | LangChain | Semantic Kernel |
|---|---|---|---|---|---|
| JSON Schema for inputs | Yes | Yes | Yes | Yes (via Pydantic) | Yes (via annotations) |
| Output schema | Optional | No | No | Optional | Inferred |
| Behavior annotations | Yes (4 hints) | No | No | No | No |
| Parallel invocation | N/A | Yes | Yes | Yes | Yes |
| Strict/constrained output | N/A | Yes | Yes | Via validation | N/A |
| Input examples | N/A | No | Yes | No | No |
| Error as guidance | All | All | All | All | All |
| Discovery/introspection | tools/list | API payload | API payload | bind_tools | kernel.Plugins |

### Recommended Design for agentquery Mutations

Based on this research, a CLI DSL mutation system should:

1. **Use the same DSL syntax as queries** with a verb convention:
   ```
   # Query (read)
   list(status=todo) { overview }

   # Mutation (write) -- same syntax, different operation name
   update(task-1, status=done)
   create(title="New task", status=todo)
   delete(task-1)
   ```

2. **Declare mutations with metadata** (following MCP annotations pattern):
   ```go
   schema.Mutation("update", handler, MutationMetadata{
       Description: "Updates task fields. Only provided fields change.",
       Parameters: []ParameterDef{
           {Name: "id", Required: true, Description: "Task ID"},
           {Name: "status", Description: "New status", Enum: []string{"todo","in_progress","done"}},
           {Name: "title", Description: "New task title"},
       },
       ReadOnly: false,
       Destructive: false,
       Idempotent: true,
   })
   ```

3. **Expose in schema() introspection** alongside operations:
   ```json
   {
     "operations": ["list", "get", "count", "summary"],
     "mutations": ["create", "update", "delete"],
     "mutationMetadata": {
       "update": {
         "description": "...",
         "parameters": [...],
         "destructive": false,
         "idempotent": true
       }
     }
   }
   ```

4. **Return the affected entity** (not just "ok"):
   ```json
   {"id":"task-1","status":"done","title":"Fix bug","updated_at":"2026-02-18T12:00:00Z"}
   ```

5. **Support dry-run via convention** (`dry_run=true` parameter):
   ```
   update(task-1, status=done, dry_run=true)
   # Returns: {"preview":{"changes":[{"field":"status","from":"in_progress","to":"done"}]}}
   ```

6. **Provide clear error messages with corrective guidance**:
   ```json
   {"error":{"code":"invalid_value","message":"status must be one of [todo, in_progress, done]","field":"status","got":"complete","suggestion":"did you mean 'done'?"}}
   ```

7. **Support batching** (following existing query batching with `;`):
   ```
   update(task-1, status=done); update(task-2, status=done)
   ```

8. **Keep the grammar unchanged** -- mutations use the same `operation(params)` syntax. The distinction is in the operation's metadata (readOnly vs mutation).
