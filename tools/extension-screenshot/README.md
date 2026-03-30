# Extension Screenshot Bridge (MVP)

This is a Day 6 MVP browser extension scaffold for the screenshot bridge workflow.

## Load in Chrome/Edge

1. Open browser extension page.
2. Enable developer mode.
3. Load unpacked folder: tools/extension-screenshot.

## Current MVP Scope

- MV3 service worker skeleton
- Pairing helpers
- Local bridge API wrappers
- Basic capture flow utilities
- Bridge task pull integration with:
  - GET /api/screenshot/bridge/tasks/next
- Mock callback integration with:
  - POST /api/screenshot/bridge/mock/result

## Current Status (2026-03-27)

- End-to-end flow has been validated:
  - pair -> enqueue batch screenshot -> pull bridge task -> capture -> callback -> backend persists file
- Bridge token auth is enabled when `screenshot.extension.pairing_required=true`.
- Callback now supports `image_data` (data URL), and backend persists image files under `screenshots/<batch_id>/`.
- Day 7 is complete:
  - extension path now covers search-engine and target-site capture, not just batch URL flow
  - `fallback_to_cdp` is active for both single and batch capture paths

## Recommended Next Steps

1. Replace mock callback semantics with real extension callback contract (production-safe fields and signature).
2. Add callback payload signing and stricter token/session rotation policy.
3. Implement Day 8 query auto-capture migration to engine-aware availability checks.
4. Add CI smoke checks for bridge APIs and extension script linting.

## Notes

- This extension is currently tuned for development integration flow.
- For production rollout, complete Day 7-12 hardening tasks in Update_Plan.md.
