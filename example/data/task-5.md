# Pagination API

Add cursor-based pagination to all list endpoints.

## Acceptance Criteria

- Implement cursor-based pagination (not offset-based)
- Support configurable page sizes (default 20, max 100)
- Return total count in response headers

## Notes

TODO: Migrate existing offset-based consumers to cursor-based.
This unblocks several frontend performance tasks.
