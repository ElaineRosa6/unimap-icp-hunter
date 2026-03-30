param(
    [string]$BaseUrl = "http://127.0.0.1:8448",
    [string]$BatchId = "bridge_e2e_full"
)

$ErrorActionPreference = "Stop"

Write-Host "[1/6] Pairing for bridge token..."
$pairReq = @{ client_id = "powershell-e2e"; pair_code = "dev-pair" } | ConvertTo-Json
$pairResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/screenshot/bridge/pair" -ContentType "application/json" -Body $pairReq
$token = $pairResp.token
if ([string]::IsNullOrWhiteSpace($token)) {
    throw "Pairing failed: empty token"
}
$authHeader = @{ Authorization = "Bearer $token" }

Write-Host "[2/6] Start async batch screenshot request..."
$batchBody = @{ urls = @("https://example.com"); batch_id = $BatchId; concurrency = 1 } | ConvertTo-Json -Depth 5
$batchJob = Start-Job -ScriptBlock {
    param($url, $body)
    Invoke-RestMethod -Method Post -Uri "$url/api/screenshot/batch-urls" -ContentType "application/json" -Body $body
} -ArgumentList $BaseUrl, $batchBody

Start-Sleep -Seconds 1

Write-Host "[3/6] Pull next bridge task..."
$taskResp = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/screenshot/bridge/tasks/next" -Headers $authHeader
if ($null -eq $taskResp.task) {
    throw "No task available from bridge queue"
}
$task = $taskResp.task

Write-Host "[4/6] Push mock result with image_data..."
# 1x1 PNG data URL (valid)
$imageData = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR4nGNgYAAAAAMAASsJTYQAAAAASUVORK5CYII="
$mockReq = @{
    request_id = $task.request_id
    success = $true
    image_path = ""
    image_data = $imageData
    batch_id = $task.batch_id
    url = $task.url
    error_code = ""
    error = ""
} | ConvertTo-Json -Depth 6
$mockResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/screenshot/bridge/mock/result" -Headers $authHeader -ContentType "application/json" -Body $mockReq

Write-Host "[5/6] Wait for batch API completion..."
$batchResp = Receive-Job -Job $batchJob -Wait -AutoRemoveJob

Write-Host "[6/6] Query bridge status..."
$statusResp = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/screenshot/bridge/status" -Headers $authHeader

Write-Host "\n=== TASK ==="
$taskResp | ConvertTo-Json -Depth 8
Write-Host "\n=== MOCK ==="
$mockResp | ConvertTo-Json -Depth 8
Write-Host "\n=== BATCH API ==="
$batchResp | ConvertTo-Json -Depth 8
Write-Host "\n=== STATUS ==="
$statusResp | ConvertTo-Json -Depth 8
