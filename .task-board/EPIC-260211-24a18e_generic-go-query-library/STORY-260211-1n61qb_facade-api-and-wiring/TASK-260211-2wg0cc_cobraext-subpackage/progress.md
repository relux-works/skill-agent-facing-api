## Status
done

## Assigned To
agent-facade

## Created
2026-02-11T13:21:35Z

## Last Update
2026-02-11T13:42:38Z

## Blocked By
- TASK-260211-209q5s

## Blocks
- TASK-260211-r8c2oy

## Checklist
(empty)

## Notes
Starting implementation. Creating cobraext/ subpackage with QueryCommand, SearchCommand, AddCommands.
Done: Created cobraext/ subpackage with QueryCommand[T], SearchCommand[T], AddCommands[T]. Added cobra dep. Tests pass for q command, grep command (with --file, -i, -C flags), and AddCommands.
