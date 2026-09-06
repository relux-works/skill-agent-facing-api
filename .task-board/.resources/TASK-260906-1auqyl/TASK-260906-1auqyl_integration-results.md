# TASK-260906-1auqyl — integration run results (BLOCKED)

Run: `RUN-260906-53109d`. Control root: `/Users/alexis/src/relux-works/skill-agent-facing-api`.
CR: `CR-TASK-260906-1auqyl-1` rev 1, candidate tree `43349d7f86160623b02f9dd63f85b6a7e457855b`, base `656ad0a`.

## Outcome

**Integration refused. Trunk was NOT moved. No landing commit, no board-state commit, no tag, no push.**
No integration transaction was opened — the refusal happens before the transaction record, so there is
nothing to dispose of and nothing to roll back.

## 0. Relation to the previous revision of this note

The earlier revision of this resource recorded a *different* refusal from an earlier
run: `integration_checkout_busy` / `marker=REBASE_HEAD`, a false positive caused by
five-week-old `.git/REBASE_HEAD` residue in the control root (git leaves that file
behind after a successful rebase and after `--abort`; the authoritative in-progress
markers are the `.git/rebase-merge` / `.git/rebase-apply` *directories*). That
residue is gone from the control root now, and this run reproduced no
`integration_checkout_busy`:

```
$ ls -d .git/rebase-merge .git/rebase-apply .git/MERGE_HEAD .git/REBASE_HEAD .git/CHERRY_PICK_HEAD
ls: .git/rebase-apply: No such file or directory
ls: .git/rebase-merge: No such file or directory
ls: .git/MERGE_HEAD: No such file or directory
ls: .git/REBASE_HEAD: No such file or directory
ls: .git/CHERRY_PICK_HEAD: No such file or directory
```

The upstream fix in `skill-project-management`
(`internal/integration/checkout.go:79` still stats `REBASE_HEAD` as an in-progress
marker) remains outstanding and is unrelated to the blocker below. Clearing it
simply advanced the run to the **next** gate, which is what this note records.

## 1. The integrate command, exactly as briefed

```
$ task-board worktree integrate STORY-260906-1mz2ft --cr TASK-260906-1auqyl --revision 1 \
    --commit-time 2026-09-05T21:15:00+03:00
integration_base_moved: the local integration checkout is not at the freshly observed protected authority OID
  head_oid: c02ddfb241eddcf8cb43157fb35d10970c2fdc43
  protected_oid: 656ad0a73dbffc92e732ed95c39a7ae3197f28f1
  protected_ref: refs/heads/main
exit code: 1
```

Typed error code: `integration_base_moved`.

## 2. Root cause — verified, not inferred

Local `main` is **one commit ahead of the published protected authority**:

```
$ git show-ref | grep -E 'refs/heads/main|refs/remotes/origin'
c02ddfb241eddcf8cb43157fb35d10970c2fdc43 refs/heads/main
656ad0a73dbffc92e732ed95c39a7ae3197f28f1 refs/remotes/origin/HEAD
656ad0a73dbffc92e732ed95c39a7ae3197f28f1 refs/remotes/origin/main

$ git rev-list --left-right --count origin/main...main
0	1
```

The extra local commit is:

```
c02ddfb G 2026-09-05 21:10:00 +0300 alexis <alexis@relux.works>
chore: task-board config (worktree isolation validation suite for the agentquery and example modules)
 task-board.config.json | 11 +++++++++++
```

It adds `spawn.worktree_isolation.integration_base_branch` + the four validation commands.

Source confirmation that the gate's operand is the **remote** authority, not the local branch tip
(`skill-project-management/tools/board-cli`):

- `internal/integration/integrate.go:264-268` — refuses when `state.HeadOID != trunk.OID`, reporting
  `trunk.OID` as `protected_oid` and `trunk.UpstreamMerge` as `protected_ref` (hence the misleading
  `refs/heads/main` label on a value that is actually the fetched remote OID).
- `internal/worktree/trunk.go:255-285` — `ResolveConfiguredProtectedAuthority` builds the trunk from
  `ResolveProtectedAuthority`'s freshly fetched tuple; `trunk.OID = tuple.FetchedOID`.

So the refusal is **correct behavior**: the control root carries an unpublished commit on the protected
branch, and the gate will not land on top of a trunk state the remote has never seen.

## 3. Why this run cannot resolve it

The only two exits are:

1. **Publish `c02ddfb` to `origin/main`** — a push. Explicitly forbidden by the brief item 3
   ("Do NOT push, do NOT tag, do NOT touch the remote") and gated on the human go-ahead.
2. **Reset local `main` back to `656ad0a`** — discards a signed commit that is not this task's, and is
   a destructive rewrite of the protected branch. Not authorized, and it would also remove the
   `worktree_isolation.validation` config the integrate gate itself requires
   (`RequireValidationConfigured`, `integrate.go:241`).

Note the coupling in (2): the same commit that blocks the base check is the one that supplies the
validation configuration the gate demands. There is no local-only ordering that satisfies both.
`c02ddfb` has to reach the remote.

Per brief item 4, no workaround was attempted.

## 4. Recommended orchestrator step (after human go-ahead)

```
git -C /Users/alexis/src/relux-works/skill-agent-facing-api push origin main   # publishes c02ddfb
```

Then re-run the integrate verbatim. With `HEAD == origin/main == c02ddfb`, the base check passes; the
CR's `base_oid` `656ad0a` is now behind trunk, so §6.2 `Reparent` runs, `changed_paths` (9, all under
`agentquery/`) do not overlap `task-board.config.json`, so the CR reparents rather than going stale,
and the framework re-runs validation against the reparented tree before landing. Publication of the
landing commit and `agentquery/v1.6.0` stays the orchestrator's separate step.

## 5. Candidate-tree validation (run in the managed worktree, foreground, real exit codes)

The accepted delta itself is green. Commands run in
`.temp/STORY-260906-1mz2ft/worktree`, which holds the candidate tree as uncommitted work:

| Command (module) | Output | Exit |
| --- | --- | ---: |
| `go build ./...` (agentquery) | (no output) | 0 |
| `go vet ./...` (agentquery) | (no output) | 0 |
| `gofmt -l .` (agentquery) | (no output — no unformatted files) | 0 |
| `go test ./... -count=1` (agentquery) | `ok github.com/relux-works/skill-agent-facing-api/agentquery 1.365s`<br>`ok github.com/relux-works/skill-agent-facing-api/agentquery/cobraext 0.987s` | 0 |
| `go build ./... && go vet ./...` (example) | (no output) | 0 |

This is a re-run of the same four gates the board's `worktree_isolation.validation` suite would execute;
it is evidence that the blocker is purely the trunk-publication state, not the delta.

## 6. State left behind

- `main` in the control root: unmoved at `c02ddfb` (no landing commit, no board-state commit).
- No tag created, no remote contacted.
- No integration transaction record (`task-board worktree transaction show STORY-260906-1mz2ft` →
  "No integration transaction is recorded").
- CR `TASK-260906-1auqyl` rev 1 still `accepted`, `repository_delta=present`, 9 changed paths.
- Managed worktree still dirty with the candidate tree, lease still held by `RUN-260906-53109d`.

## 7. Candidate-tree integrity after this run

Running `go build ./...` in `example/` (a single `main` package) wrote an `example/example`
binary into the managed worktree, which moved the working-tree hash to
`1afb53afaae4d4ed03ca95318271bdbe507b0357`. The artifact was deleted and the tree
re-verified against the accepted revision:

```
$ rm -f example/example
$ GIT_INDEX_FILE=/tmp/ci2 git read-tree HEAD && GIT_INDEX_FILE=/tmp/ci2 git add -A . \
    && GIT_INDEX_FILE=/tmp/ci2 git write-tree
43349d7f86160623b02f9dd63f85b6a7e457855b
```

Exact match with accepted CR rev 1. The worktree index was never touched; the work is
still uncommitted. `LOGBOOK.md` is part of the accepted candidate tree, so nothing was
written into it from this run — the findings live here instead.

## 8. Board status — could not be moved, and why

Both terminal moves are refused by the board guard while the element is `integrating`:

```
$ task-board m 'set_status(TASK-260906-1auqyl, status=blocked)'
$ task-board handoff TASK-260906-1auqyl --role developer
cannot move TASK-260906-1auqyl from integrating to ...: accepted work remains
integrating until integration evidence is applied
```

Accepted work stays `integrating` until an integration transaction applies evidence, and
this run never got one open (the refusal fires before the transaction record). The task is
therefore parked at `integrating` with the evidence attached, awaiting the push decision.
