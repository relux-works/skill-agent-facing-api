## Status
done

## Assigned To
agent-facade

## Created
2026-02-11T13:21:36Z

## Last Update
2026-02-11T13:42:38Z

## Blocked By
- TASK-260211-2wg0cc
- TASK-260211-246avp

## Blocks
- (none)

## Checklist
(empty)

## Notes
Starting implementation. Creating integration_test.go with full pipeline tests.
Done: Created integration_test.go with 21 tests covering: full pipeline, field projection, preset expansion, list with projection, batch mixed results (success + error + success), Search via Schema methods, Search with options (case-insensitive, context, file glob), QueryJSON/SearchJSON validity, no-loader scenario, error propagation (parse error, unknown operation, unknown field, regex error), schema options verification, default extensions, handler error maps, not-found error maps.
