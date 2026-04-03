param(
    [string]$BaseUrl = "http://127.0.0.1:8448",
    [string]$NodeId = "node-e2e-a",
    [string]$NodeToken = "token-e2e-a",
    [string]$AdminToken = "",
    [string]$TaskId = "task-e2e-1"
)

$ErrorActionPreference = "Stop"

function Assert-Success($resp, $step) {
    if ($null -eq $resp) {
        throw "$step failed: empty response"
    }
    if ($resp.PSObject.Properties.Name -contains "success") {
        if (-not [bool]$resp.success) {
            throw "$step failed: success=false"
        }
    }
}

$headers = @{ Authorization = "Bearer $NodeToken" }
$admin = $AdminToken
if ([string]::IsNullOrWhiteSpace($admin)) {
    throw "AdminToken is required when distributed.admin_token is enabled. Re-run with -AdminToken <admin-token>."
}
$adminHeaders = @{ Authorization = "Bearer $admin" }

Write-Host "[1/7] Register node..."
$registerBody = @{
    node_id = $NodeId
    hostname = "worker-e2e"
    region = "local"
    max_concurrency = 2
    capabilities = @("port_scan", "screenshot")
    egress_ip = "127.0.0.1"
} | ConvertTo-Json -Depth 6
$registerResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/nodes/register" -Headers $headers -ContentType "application/json" -Body $registerBody
Assert-Success $registerResp "register"

Write-Host "[2/7] Heartbeat..."
$heartbeatBody = @{
    node_id = $NodeId
    current_load = 0
    max_concurrency = 2
    avg_latency_ms = 9.6
    success_rate_5m = 99.9
    egress_ip = "127.0.0.1"
} | ConvertTo-Json -Depth 6
$heartbeatResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/nodes/heartbeat" -Headers $headers -ContentType "application/json" -Body $heartbeatBody
Assert-Success $heartbeatResp "heartbeat"

Write-Host "[3/7] Enqueue task..."
$enqueueBody = @{
    task_id = $TaskId
    task_type = "port_scan"
    priority = 10
    required_caps = @("port_scan")
    payload = @{ url = "https://example.com" }
} | ConvertTo-Json -Depth 6
$enqueueResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/nodes/task/enqueue" -Headers $adminHeaders -ContentType "application/json" -Body $enqueueBody
Assert-Success $enqueueResp "enqueue"

Write-Host "[4/7] Claim task..."
$claimBody = @{ node_id = $NodeId; caps = @("port_scan") } | ConvertTo-Json -Depth 6
$claimResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/nodes/task/claim" -Headers $headers -ContentType "application/json" -Body $claimBody
Assert-Success $claimResp "claim"
if ($null -eq $claimResp.task) {
    throw "claim failed: no task returned"
}

Write-Host "[5/7] Submit task result..."
$resultBody = @{
    task_id = $TaskId
    node_id = $NodeId
    status = "completed"
    duration_ms = 18
    output = @{ ok = $true; source = "day15_distributed_e2e.ps1" }
} | ConvertTo-Json -Depth 6
$resultResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/nodes/task/result" -Headers $headers -ContentType "application/json" -Body $resultBody
Assert-Success $resultResp "result"

Write-Host "[6/7] Check task snapshot..."
$taskStatusResp = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/nodes/task/status" -Headers $adminHeaders
Assert-Success $taskStatusResp "task status"

Write-Host "[7/7] Check network profile..."
$profileResp = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/nodes/network/profile" -Headers $adminHeaders
Assert-Success $profileResp "network profile"

Write-Host "`n=== REGISTER ==="
$registerResp | ConvertTo-Json -Depth 8
Write-Host "`n=== CLAIM ==="
$claimResp | ConvertTo-Json -Depth 8
Write-Host "`n=== RESULT ==="
$resultResp | ConvertTo-Json -Depth 8
Write-Host "`n=== TASK STATUS ==="
$taskStatusResp | ConvertTo-Json -Depth 8
Write-Host "`n=== NETWORK PROFILE ==="
$profileResp | ConvertTo-Json -Depth 8
