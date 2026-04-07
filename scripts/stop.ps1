# 停止脚本 (PowerShell版本)

Set-ExecutionPolicy Bypass -Scope Process -Force

$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$PROJECT_DIR = Split-Path -Parent $SCRIPT_DIR

cd $PROJECT_DIR

Write-Host "=== UniMap 停止脚本 ===" -ForegroundColor Green

Write-Host "1. 停止服务" -ForegroundColor Cyan
Write-Host "停止中..." -ForegroundColor Yellow
docker-compose down

Write-Host "2. 检查服务状态" -ForegroundColor Cyan
Write-Host "检查中..." -ForegroundColor Yellow
docker-compose ps

Write-Host "=== 停止完成 ===" -ForegroundColor Green
Write-Host "服务已停止" -ForegroundColor Green
