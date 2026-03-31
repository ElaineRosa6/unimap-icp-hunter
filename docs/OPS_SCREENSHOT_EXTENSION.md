# Screenshot Extension Ops Runbook

## Scope

This runbook focuses on screenshot extension bridge operations.

- Engine modes: cdp, extension
- Bridge endpoints: /api/screenshot/bridge/health, /api/screenshot/bridge/status
- Rollback scripts:
  - scripts/rollback_extension_to_cdp.ps1
  - scripts/rollback_extension_to_cdp.sh

## Daily Checks

1. Service health:
   - Check Web service is reachable.
   - Check bridge health/status endpoints.
2. Bridge diagnostics:
   - ready, bridge_connected
   - queue_len, in_flight_tasks, pending_tasks
   - last_error, last_error_at
3. Auth/session state:
   - pairing_required
   - paired_clients
   - callback_signature_required
4. Runtime quality:
   - Observe bridge timeout/retry/fallback trends.
   - Verify screenshot success ratio remains stable.

## Proxy Pool Checks (Day13)

1. Config sanity:
   - network.proxy_pool.enabled
   - network.proxy_pool.strategy=round_robin
   - network.proxy_pool.proxies non-empty when enabled
2. Runtime behavior:
   - Monitor reachability response contains proxy field.
   - Screenshot/tamper requests can complete when one proxy is down.
3. Cooldown/fallback:
   - Repeated proxy failures trigger cooldown.
   - allow_direct_fallback=true allows direct requests when all proxies cool down.
4. Regression gate:
   - Run go test ./... after proxy_pool config changes.

## Failure Triage Order

1. Determine mode:
   - screenshot.engine in config
2. Check bridge status:
   - Is bridge connected and ready?
3. Check auth:
   - Is pairing enabled? Is token expired?
   - Is callback signature check enabled and passing?
   - Is token rotation endpoint available and healthy?
4. Check extension runtime:
   - Can extension pull tasks/next?
   - Can extension post mock/result?
5. Check fallback behavior:
   - fallback_to_cdp enabled?

## Key API Checks

PowerShell examples:

```powershell
Invoke-RestMethod -Method Get -Uri "http://127.0.0.1:8448/api/screenshot/bridge/health"
Invoke-RestMethod -Method Get -Uri "http://127.0.0.1:8448/api/screenshot/bridge/status"
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8448/api/screenshot/bridge/token/rotate" -Headers @{ Authorization = "Bearer <token>" } -ContentType "application/json" -Body '{"revoke_old":true}'
```

Shell examples:

```bash
curl -fsS http://127.0.0.1:8448/api/screenshot/bridge/health
curl -fsS http://127.0.0.1:8448/api/screenshot/bridge/status
```

Signed callback header checklist (when `callback_signature_required=true`):

1. Request includes `X-Bridge-Timestamp`, `X-Bridge-Nonce`, `X-Bridge-Signature`.
2. Server time skew is within configured window (`callback_signature_skew_seconds`).
3. Same nonce is not reused within nonce TTL (`callback_nonce_ttl_seconds`).

Reachability sample (check proxy field):

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/url/reachability \
   -H "Content-Type: application/json" \
   -d '{"urls":["https://example.com"],"concurrency":1}'
```

Day14 port-scan sample (CDN excluded by default):

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/url/port-scan \
   -H "Content-Type: application/json" \
   -d '{"urls":["https://example.com"],"ports":[80,443,8080],"concurrency":1}'
```

## Day15 Pre-Implementation Checklist

1. Confirm distributed mode remains disabled by default.
2. Confirm single-node fallback path remains available for all critical flows.
3. Prepare node auth token issuance and rotation policy.
4. Define heartbeat timeout and offline eviction thresholds.
5. Define D15-A/B/C release windows and rollback owners.

## Day15 D15-A API Checks

Register node:

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/nodes/register \
   -H "Authorization: Bearer <node-token>" \
   -H "Content-Type: application/json" \
   -d '{"node_id":"node-a","hostname":"worker-a","region":"cn-east","max_concurrency":3,"capabilities":["port_scan","screenshot"]}'
```

Heartbeat:

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/nodes/heartbeat \
   -H "Authorization: Bearer <node-token>" \
   -H "Content-Type: application/json" \
   -d '{"node_id":"node-a","current_load":1,"max_concurrency":3,"avg_latency_ms":12.5,"success_rate_5m":99.1}'
```

Status:

```bash
curl -fsS http://127.0.0.1:8448/api/nodes/status
```

## Day15 D15-B API Checks

Enqueue task:

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/nodes/task/enqueue \
   -H "Content-Type: application/json" \
   -d '{"task_id":"task-1","task_type":"port_scan","priority":10,"required_caps":["port_scan"],"payload":{"url":"https://example.com"}}'
```

Claim task:

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/nodes/task/claim \
   -H "Authorization: Bearer <node-token>" \
   -H "Content-Type: application/json" \
   -d '{"node_id":"node-a","caps":["port_scan"]}'
```

Submit task result:

```bash
curl -fsS -X POST http://127.0.0.1:8448/api/nodes/task/result \
   -H "Authorization: Bearer <node-token>" \
   -H "Content-Type: application/json" \
   -d '{"task_id":"task-1","node_id":"node-a","status":"completed","duration_ms":32,"output":{"ok":true}}'
```

Node auth verification:

1. Configure `distributed.node_auth_tokens` with `node_id -> token` mapping.
2. Call register/heartbeat/claim/result without token and confirm `401 node_auth_failed`.
3. Retry with `Authorization: Bearer <token>` and confirm request succeeds.

Task queue status:

```bash
curl -fsS http://127.0.0.1:8448/api/nodes/task/status
```

## Day15 D15-C API Checks

Network profile:

```bash
curl -fsS http://127.0.0.1:8448/api/nodes/network/profile
```

Distributed toggle verification:

1. Set `distributed.enabled=false` and restart service.
2. Verify `/api/nodes/*` endpoints return `distributed_disabled`.
3. Set `distributed.enabled=true` and restart service.
4. Verify register/heartbeat/task/profile endpoints become available again.
5. If `distributed.admin_token` is set, verify `status/profile/enqueue/task-status` require `Authorization: Bearer <admin-token>`.

## Day15 One-Click E2E Drill

Windows PowerShell:

```powershell
./scripts/day15_distributed_e2e.ps1 -BaseUrl "http://127.0.0.1:8448" -NodeId "node-a" -NodeToken "token-a" -AdminToken "admin-token" -TaskId "task-e2e-1"
```

Linux/macOS:

```bash
./scripts/day15_distributed_e2e.sh http://127.0.0.1:8448 node-a token-a task-e2e-1 admin-token
```

Bridge strict e2e (signature + rotate) on Windows:

```powershell
./scripts/bridge_e2e.ps1 -BaseUrl "http://127.0.0.1:8448" -StrictSignature -RotateToken
```

Expected result:

1. register/heartbeat/claim/result all return `success=true`.
2. `/api/nodes/task/status` contains the submitted task and final status is `completed`.
3. `/api/nodes/network/profile` includes the node profile record.

## Day13-15 Alignment Snapshot

1. Day13 proxy pool: implemented and in use.
2. Day14 url port-scan + CDN exclusion: implemented and available.
3. Day15 distributed nodes: implemented (D15-A/B/C), with distributed gate and node token auth.

## Rollback Triggers

Trigger extension -> cdp rollback when one or more conditions persist:

1. bridge ready=false or bridge disconnected for more than 5 minutes.
2. bridge timeout/retry errors continuously rising.
3. pairing/auth cannot be recovered after refresh.
4. extension callback path unavailable or unstable.

## Hardening Config Baseline

Use these values for production baseline:

1. `screenshot.extension.pairing_required=true`
2. `screenshot.extension.callback_signature_required=true`
3. `screenshot.extension.callback_signature_skew_seconds=300`
4. `screenshot.extension.callback_nonce_ttl_seconds=600`

## Rollback Procedure

1. Execute rollback script:
   - Windows: scripts/rollback_extension_to_cdp.ps1
   - Linux/macOS: scripts/rollback_extension_to_cdp.sh
2. Restart service:
   - Windows: scripts/stop.ps1 and scripts/start.ps1
   - Linux/macOS: scripts/stop.sh and scripts/start.sh
3. Verify mode and APIs:
   - screenshot.engine=cdp
   - /api/screenshot/bridge/health is reachable
   - screenshot target/batch APIs are functional

## Post-Rollback Validation

1. Run one target screenshot request.
2. Run one batch-urls request.
3. Confirm result file path is persisted.
4. Confirm query flow remains available.
