## Status
done

## Assigned To
agent-facade

## Created
2026-02-11T13:21:30Z

## Last Update
2026-02-11T13:42:38Z

## Blocked By
- TASK-260211-2864qy

## Blocks
- TASK-260211-2wg0cc

## Checklist
(empty)

## Notes
Verified: Query() already uses parserConfig() from schema. query.go line 12 calls Parse(input, s.parserConfig()). executeStatement builds selector and passes loader. No changes needed - already fully wired.
