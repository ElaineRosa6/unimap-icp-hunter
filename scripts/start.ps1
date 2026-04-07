# 启动脚本 (PowerShell版本)

Set-ExecutionPolicy Bypass -Scope Process -Force

$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$PROJECT_DIR = Split-Path -Parent $SCRIPT_DIR

cd $PROJECT_DIR

Write-Host "=== UniMap 启动脚本 ===" -ForegroundColor Green

Write-Host "1. 启动服务" -ForegroundColor Cyan
Write-Host "启动中..." -ForegroundColor Yellow
docker-compose up -d

Write-Host "2. 检查服务状态" -ForegroundColor Cyan
Write-Host "检查中..." -ForegroundColor Yellow
docker-compose ps

Write-Host "3. 查看日志" -ForegroundColor Cyan
Write-Host "查看最近10条日志..." -ForegroundColor Yellow
docker-compose logs --tail=10

Write-Host "=== 启动完成 ===" -ForegroundColor Green
Write-Host "服务已启动，访问地址: http://localhost:8080" -ForegroundColor Green
Write-Host "健康检查地址: http://localhost:8080/health" -ForegroundColor Green
