# Integration retry brief

This run owns only the accepted `CR-TASK-260923-hyh12u-2` revision 2 landing
for `STORY-260923-371o14`.

The prior integration owner `RUN-260923-929408` was rejected before ref
movement because `version_control.confirm` is enabled, no automatic
`commit_time_policy` is configured, and it supplied no explicit commit time.
Its reviewer write-boundary marker has since been cleared after operator review.

Do not edit source or documentation, do not republish the Change Request, and
do not run `task-board handoff`. From this tracked producer-bound run, execute:

```bash
task-board worktree integrate STORY-260923-371o14 \
  --cr TASK-260923-hyh12u \
  --revision 2 \
  --commit-time 2026-09-22T20:01:00+03:00
```

The timestamp is the previous day after 20:00 MSK, matching the configured
owner policy. If integration succeeds, verify the transaction, Task/Story
statuses, landed commits, signatures, and exact changed paths. Attach a new
task-scoped outcome named `TASK-260923-hyh12u_integration-retry-results.md`.
If it refuses, attach the exact refusal and current transaction state instead.
Do not push, tag, or clean the workspace.
