## Status
done

## Assigned To
agent-facade

## Created
2026-02-11T13:21:31Z

## Last Update
2026-02-11T13:42:38Z

## Blocked By
- TASK-260211-2864qy

## Blocks
- TASK-260211-r8c2oy

## Checklist
(empty)

## Notes
Starting implementation. Blocked by 2864qy (to-review) but proceeding with code changes.
Done: Added Search() and SearchJSON() methods to Schema[T] in schema.go. Thin wrappers that delegate to package-level functions passing schema's dataDir and extensions.
