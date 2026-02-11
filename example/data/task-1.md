# Auth Service Refactor

Refactor the authentication service to use JWT tokens instead of session cookies.

## Acceptance Criteria

- Replace session-based auth with JWT
- Add token refresh endpoint
- Update middleware to validate JWT
- Maintain backward compatibility during migration

## Notes

TODO: Benchmark token validation latency under load.
TODO: Decide on token expiry duration (currently thinking 15min access / 7d refresh).
