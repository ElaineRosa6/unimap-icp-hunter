param(
    [string]$ConfigPath = "configs/config.yaml",
    [string]$ServerBase = "http://127.0.0.1:8448",
    [switch]$SkipVerify
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $ConfigPath)) {
    throw "Config file not found: $ConfigPath"
}

$lines = Get-Content -Path $ConfigPath
$inScreenshot = $false
$inExtension = $false
$updatedEngine = $false
$updatedExtEnabled = $false
$updatedFallback = $false

for ($i = 0; $i -lt $lines.Count; $i++) {
    $line = $lines[$i]

    if ($line -match '^screenshot:\s*$') {
        $inScreenshot = $true
        $inExtension = $false
        continue
    }

    if ($inScreenshot -and $line -match '^[^\s]') {
        $inScreenshot = $false
        $inExtension = $false
    }

    if (-not $inScreenshot) {
        continue
    }

    if ($line -match '^\s{4}extension:\s*$') {
        $inExtension = $true
        continue
    }

    if ($inExtension -and $line -match '^\s{4}[a-zA-Z_]') {
        $inExtension = $false
    }

    if ($line -match '^\s{4}engine:\s*') {
        $lines[$i] = '    engine: cdp'
        $updatedEngine = $true
        continue
    }

    if ($inExtension -and $line -match '^\s{8}enabled:\s*') {
        $lines[$i] = '        enabled: false'
        $updatedExtEnabled = $true
        continue
    }

    if ($inExtension -and $line -match '^\s{8}fallback_to_cdp:\s*') {
        $lines[$i] = '        fallback_to_cdp: true'
        $updatedFallback = $true
        continue
    }
}

if (-not $updatedEngine) {
    throw "Failed to locate screenshot.engine in $ConfigPath"
}
if (-not $updatedExtEnabled) {
    throw "Failed to locate screenshot.extension.enabled in $ConfigPath"
}
if (-not $updatedFallback) {
    throw "Failed to locate screenshot.extension.fallback_to_cdp in $ConfigPath"
}

Set-Content -Path $ConfigPath -Value $lines -Encoding UTF8
Write-Host "Updated $ConfigPath -> engine=cdp, extension.enabled=false, fallback_to_cdp=true"
Write-Host "Next step: restart service (recommended: .\\scripts\\stop.ps1 ; .\\scripts\\start.ps1)"

if ($SkipVerify) {
    exit 0
}

try {
    $health = Invoke-RestMethod -Method Get -Uri "$ServerBase/api/screenshot/bridge/health" -TimeoutSec 5
    Write-Host "Bridge health after config update:"
    $health | ConvertTo-Json -Depth 6
} catch {
    Write-Warning "Health endpoint not reachable. Restart service first, then verify: $ServerBase/api/screenshot/bridge/health"
}
