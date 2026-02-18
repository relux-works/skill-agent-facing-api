# CLI Read/Write Patterns Research

Research into how established CLI tools and DSLs handle both read and write operations.
Focus: patterns transferable to an agent-facing query DSL library.

---

## 1. kubectl (Kubernetes CLI)

### Read Operations
- `kubectl get <resource>` — list/retrieve resources, supports `-o json`, `-o yaml`, `-o wide`, `-o jsonpath=...`
- `kubectl describe <resource>` — human-readable detailed view (not machine-parseable)
- `kubectl logs`, `kubectl top` — specialized reads

### Write Operations

**Imperative creates:**
```bash
kubectl create deployment my-dep --image=nginx
kubectl run my-pod --image=nginx
```

**Declarative apply (the primary pattern):**
```bash
kubectl apply -f deployment.yaml      # single file
kubectl apply -f ./manifests/         # directory
kubectl apply -f -                    # stdin
kubectl apply -k ./kustomize-dir/     # kustomize
```

**Patch (in-place field updates):**
```bash
# Strategic merge patch (default) — merge-aware, knows Kubernetes schema
kubectl patch deployment my-dep -p '{"spec":{"replicas":3}}'

# JSON merge patch (RFC 7386) — simpler, replaces at object level
kubectl patch deployment my-dep --type=merge -p '{"spec":{"template":{"spec":{"containers":[{"name":"nginx","image":"nginx:1.21"}]}}}}'

# JSON patch (RFC 6902) — list of discrete operations
kubectl patch pod my-pod --type='json' -p='[{"op":"replace","path":"/spec/containers/0/image","value":"nginx:1.21"}]'
```

**Delete:**
```bash
kubectl delete -f deployment.yaml
kubectl delete deployment my-dep
kubectl delete pods --all -n my-namespace
```

### Safety Mechanisms

**Dry-run (three modes):**
- `--dry-run=none` — default, actually executes
- `--dry-run=client` — local validation only, no server contact. Prints what would be sent
- `--dry-run=server` — sends to API server for full validation (admission webhooks, defaults) but does NOT persist

```bash
kubectl apply -f deployment.yaml --dry-run=server -o yaml
kubectl create deployment my-dep --image=nginx --dry-run=client -o yaml
```

**Diff (preview changes before apply):**
```bash
kubectl diff -f deployment.yaml
# Exit code: 0 = no changes, 1 = changes found, >1 = error
```

**Recommended validation workflow:**
```bash
kubectl apply --dry-run=server -f manifest.yaml   # validate
kubectl diff -f manifest.yaml                       # see what changes
kubectl apply -f manifest.yaml                      # actually apply
```

**Confirmation:** kubectl does NOT prompt for confirmation on destructive operations by default. Deletion is immediate. The `--grace-period` and `--cascade` flags control behavior but don't add confirmation.

### Server-Side Apply (SSA) — Field Ownership

SSA tracks which "manager" (actor) owns each field. Conflicts arise when two managers try to set the same field to different values.

- Without `--force-conflicts`: returns a conflict error
- With `--force-conflicts`: takes ownership, overrides the other manager's value
- Not sending a field = releasing ownership (field gets deleted if no other manager)

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Declarative intent** | `apply -f` — "make it look like this", not "do this step" |
| **Three-mode dry-run** | Client (local), Server (validated), None (execute) |
| **Diff before apply** | Show exactly what would change |
| **Structured patch types** | Strategic merge, JSON merge, JSON patch — different granularity |
| **stdin support** | `-f -` for piping, great for automation |
| **Output format control** | `-o json`, `-o yaml`, `-o jsonpath` on both reads and writes |
| **Field ownership** | SSA tracks who set what, detects conflicts |

---

## 2. Terraform CLI

### Two-Phase Mutation Pattern

Terraform's core innovation: **plan then apply** as a mandatory two-phase workflow.

**Phase 1 — Plan (preview):**
```bash
terraform plan                    # show what would change
terraform plan -out=plan.tfplan   # save plan to file
terraform plan -destroy           # preview destruction
```

Plan output uses explicit symbols:
- `+` create
- `-` destroy
- `~` update in-place
- `-/+` destroy and recreate (tainted/forced)

Summary line: `Plan: 2 to add, 1 to change, 0 to destroy.`

**Phase 2 — Apply (execute):**
```bash
terraform apply                   # creates a new plan, prompts for confirmation
terraform apply plan.tfplan       # applies a saved plan (no prompt)
terraform apply -auto-approve     # skip confirmation prompt
```

**Destroy:**
```bash
terraform destroy                 # prompts: "Do you really want to destroy all resources?"
terraform destroy -auto-approve   # skip prompt
```

### State Management

- Terraform maintains a **state file** (JSON) mapping declared resources to real-world IDs
- State is the source of truth for what exists
- `terraform refresh` — sync state with reality
- `terraform state mv/rm/import` — manual state manipulation
- State locking prevents concurrent modifications

### Safety Mechanisms

| Mechanism | Description |
|-----------|-------------|
| **Mandatory plan** | Cannot apply without seeing what will change |
| **Confirmation prompt** | Interactive "yes" required (unless `-auto-approve`) |
| **Saved plans** | `plan -out=file` + `apply file` ensures exact execution |
| **State locking** | Prevents concurrent modifications |
| **`-target`** | Limit operations to specific resources |
| **Lifecycle rules** | `prevent_destroy = true` in config blocks accidental deletion |

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Two-phase commit** | Preview (plan) then execute (apply) — never blind writes |
| **Saved plan files** | Plan can be serialized and applied later |
| **Explicit change symbols** | `+` `-` `~` make change types instantly visible |
| **Auto-approve flag** | Automation can skip prompts explicitly |
| **Destruction as special mode** | `destroy` is a separate command/mode, not just "delete" |

---

## 3. gh (GitHub CLI)

### Read Operations
```bash
gh issue list
gh issue view 42
gh pr list --json number,title,author
gh pr view 123 --json files,reviews
gh api repos/owner/repo/issues
```

**Structured output:**
```bash
gh issue list --json number,title,state     # JSON with field selection
gh issue list --json number,title --jq '.[].title'  # jq filter
gh issue list --json number --template '{{range .}}{{.number}}{{end}}'
```

**Field discovery:** `gh issue list --json` (no argument) — lists available fields.

### Write Operations

**Create with flags (non-interactive):**
```bash
gh issue create --title "Bug" --body "Description" --label bug
gh pr create --title "Feature" --body "Details" --reviewer user1
gh release create v1.0.0 --title "Release" --notes "Changelog"
```

**Create interactively (no flags):**
```bash
gh issue create   # prompts for title, body, labels, etc.
gh pr create      # prompts for title, body, reviewers, etc.
```

**The rule:** providing flags switches to non-interactive mode. Omitting flags triggers interactive prompts. This is the "progressive disclosure" pattern.

**Edit/update:**
```bash
gh issue edit 42 --title "New title" --add-label priority
gh pr edit 123 --add-reviewer user2
gh issue close 42
gh pr merge 123 --squash
```

**GraphQL mutations (escape hatch for everything):**
```bash
gh api graphql -f query='mutation { addLabelsToLabelable(input: {labelableId: "ID", labelIds: ["LID"]}) { clientMutationId } }'
```

**Body from file/stdin:**
```bash
gh issue create --title "Bug" --body-file description.md
gh pr comment 123 --body-file - < comment.txt   # stdin with "-"
```

### Safety Mechanisms

- No dry-run for mutations (creates are idempotent-ish)
- Destructive operations (`delete`) require `--yes` flag or interactive confirmation
- `--dry-run` not available on most write commands

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Progressive disclosure** | Flags = scripting mode, no flags = interactive |
| **Body from file/stdin** | `--body-file` and `-` for stdin |
| **Field selection on reads** | `--json field1,field2` — agent picks fields |
| **Field discovery** | `--json` with no args lists available fields |
| **Verb-noun structure** | `gh <noun> <verb>` — `issue create`, `pr merge` |
| **GraphQL escape hatch** | `gh api graphql` for anything not in the CLI |

---

## 4. jq / yq

### jq — JSON Query and Transform

**Read (query):**
```bash
jq '.name' file.json
jq '.users[] | select(.age > 30) | .name' file.json
jq '[.items[] | {id, name}]' file.json
```

**Transform (mutation expressions):**
```bash
jq '.name = "new-name"' file.json                         # set field
jq '.users += [{"name": "new"}]' file.json                # append
jq 'del(.users[] | select(.name == "old"))' file.json     # delete
jq '.users[] |= if .name == "alice" then .age = 31 else . end' file.json  # conditional update
jq 'walk(if type == "string" then ascii_downcase else . end)' file.json   # recursive transform
```

**In-place (jq does NOT support native in-place):**
```bash
# Workaround with sponge:
jq '.name = "new"' file.json | sponge file.json

# Workaround with temp file:
jq '.name = "new"' file.json > tmp.json && mv tmp.json file.json

# Workaround with subshell:
cat <<< "$(jq '.name = "new"' file.json)" > file.json
```

**Key insight:** jq uses the SAME expression language for reads and writes. `.name` reads, `.name = "val"` writes. The `=` operator is the mutation boundary.

### yq — YAML/JSON/XML/TOML Processor

**Read:**
```bash
yq '.metadata.name' file.yaml
yq '.spec.containers[0].image' file.yaml
```

**Write (in-place supported natively):**
```bash
yq -i '.metadata.labels.env = "prod"' file.yaml          # set
yq -i 'del(.metadata.annotations)' file.yaml             # delete
yq -i '.spec.replicas = 3' file.yaml                     # update
yq -i '.spec.containers += [{"name":"sidecar"}]' file.yaml  # append
yq -i '(.. | select(has("image"))).image = "nginx:latest"' file.yaml  # recursive
```

**The `-i` flag is the mutation boundary** — same expression, but `-i` makes it write back.

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Unified expression language** | Same syntax for read and write — assignment operator is the boundary |
| **`-i` flag for mutation** | Explicit opt-in to side effects |
| **Selector + operator** | `.path = value` — path expression selects, operator mutates |
| **Conditional updates** | `if/then/else`, `select()` — targeted mutations |
| **Recursive transforms** | `walk()`, `..` — apply mutations across nested structures |
| **No confirmation** | jq/yq are tools, not interactive apps — trust the caller |

---

## 5. SQL CLI Tools (psql, sqlite3)

### Unified DSL for Reads and Writes

SQL is the canonical example of a single DSL with both read and write operations:

```sql
-- Reads
SELECT * FROM users WHERE status = 'active';
SELECT count(*) FROM orders;

-- Writes
INSERT INTO users (name, email) VALUES ('alice', 'alice@example.com');
UPDATE users SET status = 'inactive' WHERE last_login < '2024-01-01';
DELETE FROM orders WHERE status = 'cancelled';
```

**The verb is the operation type indicator.** No separate "mode" or command — the SQL statement itself declares intent.

### Transaction Support

```sql
BEGIN;
  UPDATE accounts SET balance = balance - 100 WHERE id = 1;
  UPDATE accounts SET balance = balance + 100 WHERE id = 2;
COMMIT;
-- or ROLLBACK; to undo
```

**Savepoints for nested undo:**
```sql
BEGIN;
  INSERT INTO orders (product_id, qty) VALUES (1, 5);
  SAVEPOINT before_risky;
    DELETE FROM inventory WHERE product_id = 1;
  ROLLBACK TO before_risky;   -- undo just the delete
COMMIT;                        -- the insert persists
```

### Safety Mechanisms

**psql:**
- `--single-transaction` / `-1` — wraps entire script in a transaction
- `ON_ERROR_ROLLBACK` — savepoint per statement in interactive mode
- `ON_ERROR_STOP` — abort script on first error
- No built-in confirmation for destructive operations
- `\set AUTOCOMMIT off` — manual transaction control

**sqlite3:**
- `.bail on` — stop on first error (batch mode)
- Interactive vs batch mode detected by TTY
- `-batch` flag forces non-interactive
- No confirmation prompts for DELETE/DROP

**Key insight:** SQL CLIs do NOT confirm destructive operations. The assumption is: if you wrote the SQL, you meant it. Safety comes from transactions (atomicity, rollback) not from prompts.

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Verb-first grammar** | `SELECT`, `INSERT`, `UPDATE`, `DELETE` — operation type is the first token |
| **Same DSL, same parser** | Reads and writes share one grammar, one parser |
| **Transactions as safety** | Atomic batches with rollback, not confirmation prompts |
| **Savepoints** | Partial rollback within a transaction |
| **WHERE clause filtering** | Same predicate syntax for reads (SELECT WHERE) and writes (UPDATE WHERE, DELETE WHERE) |
| **RETURNING clause** | Writes can return data: `INSERT ... RETURNING id, name` |
| **Batch isolation** | Error in one statement can stop or continue (configurable) |

---

## 6. etcdctl

### Read/Write Operations

**Read:**
```bash
etcdctl get mykey
etcdctl get --prefix /config/          # prefix scan
etcdctl get foo bar                     # range [foo, bar)
etcdctl get --rev=42 mykey              # historical read
```

**Write:**
```bash
etcdctl put mykey "myvalue"
etcdctl put mykey "newvalue"            # overwrite (no create/update distinction)
```

**Delete:**
```bash
etcdctl del mykey
etcdctl del --prefix /config/           # prefix delete
etcdctl del foo bar                     # range delete
```

### Transaction Support (txn)

etcdctl has a mini-DSL for atomic compare-and-swap transactions:

```bash
etcdctl txn --interactive
# compares:
value("user1") = "bad"

# success requests (if all compares true):
del user1

# failure requests (if any compare false):
put user1 "still-good"
```

**Non-interactive (for scripting):**
```bash
etcdctl txn <<<'
mod("key1") > "0"

put key1 "overwrote-key1"

put key1 "created-key1"
'
```

**Transaction grammar:**
```
<Txn>    ::= <CMP>* "\n" <THEN> "\n" <ELSE> "\n"
<CMP>    ::= value|mod|create|version("key") <|=|> "value"
<THEN>   ::= get|put|del commands
<ELSE>   ::= get|put|del commands
```

### Safety Mechanisms

- No confirmation prompts
- No dry-run
- Transactions provide atomicity (compare-and-swap)
- Watch (`etcdctl watch`) for monitoring changes
- Lease-based expiry for automatic cleanup

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Flat verb-noun** | `get`, `put`, `del` — minimal, no nesting |
| **No create/update split** | `put` is upsert — simplifies the API |
| **Compare-and-swap transactions** | Atomic conditional mutations |
| **Prefix operations** | `--prefix` works on reads AND deletes |
| **Revision-based reads** | Historical state access |
| **Interactive/non-interactive txn** | Same grammar, different input modes |

---

## 7. Other Notable CLIs

### Redis CLI (redis-cli)

**Flat command model:**
```bash
SET key "value"           # write
GET key                   # read
DEL key                   # delete
HSET hash field "value"   # hash write
HGET hash field           # hash read
HGETALL hash              # hash read all
```

**Transactions:**
```bash
MULTI                     # begin transaction
SET key1 "a"
SET key2 "b"
EXEC                      # commit (or DISCARD to rollback)
```

**Lua scripting for complex mutations:**
```bash
EVAL "redis.call('set', KEYS[1], ARGV[1])" 1 mykey myvalue
```

**Pattern:** every command is a single verb. No nesting. MULTI/EXEC for batching. Extremely flat, extremely scriptable.

### Consul CLI

```bash
consul kv put redis/config/maxconns 25   # write
consul kv get redis/config/maxconns       # read
consul kv get -detailed redis/config/maxconns  # read with metadata
consul kv delete -recurse redis/           # delete tree
consul kv export > backup.json             # export
consul kv import < backup.json             # import
```

**Pattern:** `<namespace> <verb> <key> [value]` — similar to etcdctl but with KV namespace prefix.

### HashiCorp Vault CLI

```bash
vault kv put secret/myapp password=s3cret   # write (key=value pairs)
vault kv get secret/myapp                     # read
vault kv get -format=json secret/myapp        # structured read
vault kv delete secret/myapp                  # soft delete (versioned)
vault kv destroy -versions=1,2 secret/myapp   # permanent destroy
vault kv rollback -version=1 secret/myapp     # rollback to version
vault kv list secret/                         # list keys
```

**Pattern:** versioned mutations with soft delete / hard destroy / rollback. `-format=json` for machine consumption. Key=value pairs as inline input.

### Pulumi CLI

**Same two-phase pattern as Terraform, different vocabulary:**
```bash
pulumi preview           # plan — show what would change
pulumi up                # apply — execute changes (with confirmation)
pulumi up --yes          # skip confirmation
pulumi destroy           # destroy all resources
pulumi destroy --preview-only  # show what would be destroyed
```

**Pattern:** preview/up mirrors Terraform's plan/apply. `--yes` is the auto-approve flag. `--preview-only` is the preview-without-executing flag.

### CUE Language

CUE is interesting as a validation/unification tool:

```bash
cue eval config.cue        # evaluate and show result
cue export config.cue      # export as concrete JSON/YAML
cue vet config.cue data.yaml  # validate data against schema
```

**Pattern:** CUE doesn't mutate — it validates and transforms. The "mutation" is in the unification: you combine constraints and data, CUE tells you if they're compatible. Relevant for the validation phase of mutations.

---

## 8. GraphQL Mutations (Protocol-Level Pattern)

### Read/Write Separation in the Type System

```graphql
# Schema declares both query and mutation types
type Query {
  user(id: ID!): User
  users(filter: UserFilter): [User!]!
}

type Mutation {
  createUser(input: CreateUserInput!): CreateUserPayload!
  updateUser(id: ID!, input: UpdateUserInput!): UpdateUserPayload!
  deleteUser(id: ID!): DeleteUserPayload!
}
```

### Key Design Properties

1. **Explicit separation:** Queries and mutations are different root types. You cannot accidentally call a mutation thinking it's a query.

2. **Serial execution of mutations:** In a batch `{ a; b; c }`, query fields execute in parallel. Mutation fields execute in series. This prevents race conditions.

3. **Input types are dedicated:** `CreateUserInput` is a separate type from `User`. This allows write validation to differ from read shapes.

4. **Return types include the result:** Mutations return the affected object, allowing the caller to read back what was created/modified without a separate query.

5. **Naming convention:** Verb-first: `createUser`, `updateUser`, `deleteUser`. The verb signals the mutation type.

6. **Introspection:** `__schema { mutationType { fields { name args { name type { ... } } } } }` — mutations are fully discoverable.

### Key Patterns for DSL Design

| Pattern | Description |
|---------|-------------|
| **Type-level separation** | Queries and mutations are distinct root types |
| **Serial mutation execution** | Prevents race conditions in batches |
| **Dedicated input types** | Write shapes differ from read shapes |
| **Return affected data** | Mutation response includes the result |
| **Full introspection** | Mutations discoverable via schema |
| **Verb-first naming** | `createUser`, not `userCreate` |

---

## 9. Cross-Cutting Analysis: Patterns for Agent-Facing DSL

### How Tools Distinguish Reads from Writes

| Approach | Tools | Mechanism |
|----------|-------|-----------|
| **Separate commands** | kubectl, gh, etcdctl, redis, consul, vault | `get` vs `apply/put/create` |
| **Verb in grammar** | SQL, GraphQL | `SELECT` vs `INSERT/UPDATE/DELETE`, `query` vs `mutation` |
| **Assignment operator** | jq, yq | `.field` (read) vs `.field = val` (write) |
| **Type-level separation** | GraphQL | `Query` type vs `Mutation` type |
| **Separate phases** | Terraform, Pulumi | `plan` (read diff) vs `apply` (write) |

### Safety Mechanism Taxonomy

| Level | Mechanism | Tools |
|-------|-----------|-------|
| **0 — None** | Trust the caller | jq, yq, redis-cli, etcdctl |
| **1 — Dry-run** | Preview what would happen | kubectl (`--dry-run=client/server`) |
| **2 — Diff** | Show exact changes | kubectl (`diff`), terraform (`plan`) |
| **3 — Confirmation** | Interactive yes/no | terraform (`apply`), gh (`delete`) |
| **4 — Two-phase** | Saved plan + separate apply | terraform (`plan -out` + `apply`) |
| **5 — Ownership** | Track who owns what field | kubectl SSA (field managers) |
| **6 — Transaction** | Atomic, rollbackable batch | SQL, etcdctl (`txn`), redis (`MULTI/EXEC`) |

### Input Format Patterns

| Format | Tools | When |
|--------|-------|------|
| **Inline key=value** | vault, consul, etcdctl | Simple single-field writes |
| **JSON body (flag)** | kubectl (`-p '{...}'`) | Structured inline mutations |
| **File reference** | kubectl (`-f file.yaml`), gh (`--body-file`) | Large/complex inputs |
| **Stdin** | kubectl (`-f -`), gh (`--body-file -`), SQL | Piping, automation |
| **Flags** | gh (`--title`, `--body`) | Structured, discoverable inputs |
| **Dedicated input types** | GraphQL | Type-safe structured input |
| **Expression language** | jq, yq, SQL | Same language for read and write |

### Write Discoverability Patterns

| Approach | Tools | Mechanism |
|----------|-------|-----------|
| **Help/subcommands** | kubectl, gh, vault | `--help` lists all verbs including writes |
| **Schema introspection** | GraphQL | `__schema { mutationType { ... } }` |
| **`--json` with no args** | gh | Lists available fields for output |
| **Operation metadata** | agentquery (current) | `schema()` lists operations with parameters |
| **Convention** | SQL | Everyone knows `INSERT`, `UPDATE`, `DELETE` |

### Response Format Patterns

| Pattern | Tools | Description |
|---------|-------|-------------|
| **Return affected object** | GraphQL, SQL (RETURNING), vault | Write response includes new state |
| **Return diff/plan** | Terraform, kubectl diff | Show what changed |
| **Return status only** | etcdctl, redis | OK/error, no data |
| **Configurable output** | kubectl (`-o json`), gh (`--json`), vault (`-format=json`) | Caller chooses format |

---

## 10. Transferable Patterns for agentquery Mutation Design

### Most Relevant Patterns (ranked by fit)

**1. GraphQL-style type separation (HIGH)**
- Mutations as a distinct category from queries in the Schema
- Discoverable via `schema()` introspection
- Verb-first naming convention (`create_task`, `update_status`, `delete_task`)

**2. SQL-style unified grammar (HIGH)**
- Same parser handles reads and writes
- The operation name (verb) determines if it's a read or write
- Same predicate/filter syntax for targeting items in writes

**3. kubectl dry-run pattern (HIGH)**
- `--dry-run` flag (or `dry_run=true` arg) to preview mutation effects
- Returns what would change without executing
- Client-side vs server-side validation levels

**4. Terraform plan/apply inspiration (MEDIUM)**
- Mutation could return a "plan" (diff of what would change)
- Separate "confirm" step for destructive operations
- Change symbols (`+` create, `~` update, `-` delete) in response

**5. jq/yq assignment operator (MEDIUM)**
- Consider if mutations could use an assignment-like syntax within the existing expression language
- `.field = value` is intuitive for updates

**6. GraphQL input types (MEDIUM)**
- Dedicated input shapes for mutations (vs read field selectors)
- Allows different validation for writes

**7. SQL RETURNING clause (HIGH)**
- Mutations return the affected item(s) with field projection
- `create_task(name="foo") { id, name, status }` — create and return fields

**8. etcdctl compare-and-swap (LOW-MEDIUM)**
- Conditional mutations: "update only if current value matches"
- Useful for conflict detection

**9. Batch mutation isolation from current reads (HIGH)**
- Same pattern agentquery already uses for read batches
- Error in one mutation doesn't abort others
- Inline errors in batch response

**10. kubectl patch types (LOW)**
- Different merge strategies may be overkill for a DSL
- But the concept of "merge" vs "replace" semantics is relevant for updates
