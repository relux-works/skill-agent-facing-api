# Integration brief — TASK-260906-1auqyl (accepted CR rev 1)

You are the tracked integration run for the accepted Change Request CR-TASK-260906-1auqyl-1 revision 1 (candidate tree 43349d7f86160623b02f9dd63f85b6a7e457855b, base 656ad0a). Do exactly this, in the control root (this repository's main working tree):

1. task-board worktree integrate STORY-260906-1mz2ft --cr TASK-260906-1auqyl --revision 1 --commit-time 2026-09-06T21:00:00+03:00
   The commit time is required by version_control.confirm and follows the owner policy (previous day, after 20:00 MSK).
2. Verify the result: `git log -2 --format='%h %G? %ci %s'` on main must show the squash landing commit and the board-state commit, both signature status G, author alexis <alexis@relux.works>. `git write-tree`-equivalent check: `git rev-parse HEAD~1^{tree}` for the landing commit must equal the candidate tree 43349d7f86160623b02f9dd63f85b6a7e457855b when the board commit is on top (otherwise compare HEAD^{tree} of the landing commit itself).
3. Do NOT push, do NOT tag, do NOT touch the remote. Publication (push main, tag agentquery/v1.6.0) is the orchestrator's step after a human go-ahead.
4. If integrate refuses, do not work around it: record the exact refusal in the results note and hand off as blocked with the typed error.
5. Attach TASK-260906-1auqyl_integration-results.md (commands, outputs, commit OIDs, signature verification) and hand off: task-board handoff TASK-260906-1auqyl --role developer.
