# Feature Parity Plan (Web / GUI / CLI)

Date: 2026-03-23
Status: One-Shot Unified (API-first + local fallback)

## 1. Current Capability Matrix

Legend:
- Full: fully available in this entrypoint
- Partial: available but not equivalent in behavior/scope
- Missing: no direct support

| Capability | Web | GUI | CLI |
|---|---|---|---|
| Multi-engine asset query | Full | Full | Full |
| Query status tracking | Full | Partial | Missing |
| WebSocket stream updates | Full | Missing | Missing |
| URL import and reachability | Full | Full | Missing |
| Tamper check and baseline set | Full | Full | Missing |
| Baseline list/delete management | Full | Full | Missing |
| Tamper history browse | Full | Full | Missing |
| Tamper history delete by URL | Full | Full | Missing |
| Screenshot capture (single/batch) | Full | Full | Missing |
| Screenshot batch/file management | Full | Full | Missing |
| Cookie and CDP management | Full | Partial | Partial |
| Version display consistency | Full | Full | Full |

## 2. Alignment Targets

1. Web and GUI parity target:
- GUI major operations must be mappable to first-class Web APIs.
- Data shape and error envelope should be consistent.

2. CLI target:
- Keep existing direct-engine mode for compatibility.
- Add API-first mode for user-friendly access to Web capabilities.

## 3. Minimum Retrofit Plan

### P0 (high-impact, minimum scope)

1. Define capability contract and response conventions.
2. Fill missing Web APIs required by GUI operations.
3. Introduce CLI API mode with minimal subcommands.

### P1 (experience consistency)

1. GUI client flow calls Web APIs by default, with local fallback.
2. CLI supports async status follow for long-running operations.

### P2 (governance)

1. Cross-entrypoint contract tests.
2. Unified error-code catalog and docs consolidation.

## 4. Started in This Iteration

Implemented now:

1. Web: tamper history delete API
- Route: DELETE /api/tamper/history/delete?url={url}
- Behavior: remove all check records for the given URL.

2. Web: screenshot batch management APIs
- Route: GET /api/screenshot/batches
- Behavior: list screenshot batches.

3. Web: screenshot file listing API
- Route: GET /api/screenshot/batches/files?batch={batch}
- Behavior: list files in a screenshot batch with metadata and preview URL.

4. Web: screenshot delete APIs
- Route: DELETE /api/screenshot/batches/delete?batch={batch}
- Behavior: delete a screenshot batch directory.
- Route: DELETE /api/screenshot/file/delete?batch={batch}&file={file}
- Behavior: delete one screenshot file in the batch.

Security and safety notes:
- Added strict path token validation for batch/file names.
- Added directory traversal checks before file operations.

5. CLI: first API-first subcommands
- `query`: call `POST /api/query`
- `tamper-check`: call `POST /api/tamper/check`
- `screenshot-batch`: call `POST /api/screenshot/batch-urls`
- Keep legacy direct-engine flags for backward compatibility.

6. P0 test hardening
- Added CLI unit tests for API helper request paths and argument parsing helpers.
- Added Web handler tests for screenshot management APIs and tamper history delete API.
- Full test suite is green after changes.

7. GUI first parity path delivered
- GUI tamper-check now prefers Web API (`POST /api/tamper/check`).
- Automatic local fallback is kept to preserve standalone usability.
- Runtime status now indicates execution source (API/local).

8. GUI one-shot unified delivery
- Monitor tab:
	- Baseline set: API-first (`POST /api/tamper/baseline`) + local fallback.
	- Baseline list/delete: API-first (`GET /api/tamper/baseline/list`, `DELETE /api/tamper/baseline/delete`) + local fallback.
	- Batch screenshot: API-first (`POST /api/screenshot/batch-urls`) + local fallback.
- History tab:
	- History list: API-first (`GET /api/tamper/history`) + local fallback.
	- History delete: API-first (`DELETE /api/tamper/history/delete`) + local fallback.
	- Baseline delete in history view: API-first + local fallback.
- Screenshot tab:
	- Batch/file list: API-first (`GET /api/screenshot/batches`, `GET /api/screenshot/batches/files`) + local fallback.
	- Batch/file delete: API-first (`DELETE /api/screenshot/batches/delete`, `DELETE /api/screenshot/file/delete`) + local fallback.

9. Web API consistency fix
- `handleTamperBaselineDelete` now uses `DELETE` to match routed method.
- Baseline URL is read from query parameter (`?url=`), matching API-first callers.

## 5. Next Recommended Steps

1. Add GUI integration tests for API-first fallback switches (API fail -> local success).

2. Add Web tests for `DELETE /api/tamper/baseline/delete?url=...` method contract.

3. Evaluate phase-2 hard cut: remove local fallback after acceptance window.
