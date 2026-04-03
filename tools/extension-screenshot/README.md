# Extension Screenshot Bridge (MVP)

This is a Day 6 MVP browser extension scaffold for the screenshot bridge workflow.

## Load in Chrome/Edge

1. Open browser extension page.
2. Enable developer mode.
3. Load unpacked folder: tools/extension-screenshot.

## Configuration

### API Base URL
默认连接 `http://127.0.0.1:8448`。如需修改，在扩展控制台执行：

```javascript
chrome.storage.local.set({ apiBaseURL: "http://your-server:port" })
```

### Memory Settings
内存相关参数（在 `src/capture.js` 中修改）：
- `MAX_TAB_POOL_SIZE`: Tab池大小（默认3）
- `TAB_REUSE_TIMEOUT_MS`: Tab复用超时（默认30秒）

## Current MVP Scope

- MV3 service worker skeleton
- Pairing helpers
- Local bridge API wrappers
- Basic capture flow utilities
- Bridge task pull integration with:
  - GET /api/screenshot/bridge/tasks/next
- Mock callback integration with:
  - POST /api/screenshot/bridge/mock/result

## Current Status (2026-04-03)

- **Tab Pool Management**: 截图完成后Tab被释放或复用，避免内存泄漏
- **Configurable API URL**: API地址可通过storage配置
- End-to-end flow has been validated:
  - pair -> enqueue batch screenshot -> pull bridge task -> capture -> callback -> backend persists file
- Bridge token auth is enabled when `screenshot.extension.pairing_required=true`.
- Callback now supports `image_data` (data URL), and backend persists image files under `screenshots/<batch_id>/`.
- Day 7 is complete:
  - extension path now covers search-engine and target-site capture, not just batch URL flow
  - `fallback_to_cdp` is active for both single and batch capture paths
- Day 16 hardening baseline is complete:
  - extension callback now sends `X-Bridge-Timestamp` / `X-Bridge-Nonce` / `X-Bridge-Signature`
  - extension proactively rotates bridge token before expiry via `/api/screenshot/bridge/token/rotate`
  - backend can enforce callback signature and nonce replay checks via config
  - CI smoke workflow added for bridge-focused tests and extension script syntax checks

## Known Issues Fixed (2026-04-03)

1. **Permission Error**: 修复 `captureVisibleTab` 权限错误，添加 `<all_urls>` 权限
2. **Memory Leak**: 实现Tab池管理，批量截图不再耗尽内存
3. **Hardcoded URL**: API地址现可配置

## Recommended Next Steps

1. Replace mock callback semantics with real extension callback contract (production-safe fields and signature).
2. Keep callback signing enabled in production and continue token/session governance (issue/rotation/expiry audit).
3. Extend CI smoke from syntax checks to live bridge e2e in controlled environment.
4. Add release-evidence linkage for Day15 acceptance and rollback drill records.

## Notes

- This extension is currently tuned for development integration flow.
- For production rollout, complete Day 7-12 hardening tasks in Update_Plan.md.