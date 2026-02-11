# Dashboard Performance

Optimize the main dashboard to load under 2 seconds on 3G connections.

## Acceptance Criteria

- Lazy-load non-critical widgets
- Implement virtual scrolling for the activity feed
- Add skeleton screens for loading states

## Notes

Current load time: ~4.5s on 3G (measured via Lighthouse).
TODO: Profile the API response payload sizes — some endpoints return way too much data.
Blocked by: backend pagination API (task-5).
