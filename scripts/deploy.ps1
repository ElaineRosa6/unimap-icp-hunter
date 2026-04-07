# 部署脚本 (PowerShell版本)

Set-ExecutionPolicy Bypass -Scope Process -Force

$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$PROJECT_DIR = Split-Path -Parent $SCRIPT_DIR

cd $PROJECT_DIR

Write-Host "=== UniMap 部署脚本 ===" -ForegroundColor Green

Write-Host "1. 检查Docker和Docker Compose是否安装" -ForegroundColor Cyan
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "错误: Docker未安装，请先安装Docker" -ForegroundColor Red
    exit 1
}

if (-not (Get-Command docker-compose -ErrorAction SilentlyContinue)) {
    Write-Host "错误: Docker Compose未安装，请先安装Docker Compose" -ForegroundColor Red
    exit 1
}

Write-Host "2. 构建镜像" -ForegroundColor Cyan
Write-Host "构建中..." -ForegroundColor Yellow
docker-compose build

Write-Host "3. 启动服务" -ForegroundColor Cyan
Write-Host "启动中..." -ForegroundColor Yellow
docker-compose up -d

Write-Host "4. 检查服务状态" -ForegroundColor Cyan
Write-Host "检查中..." -ForegroundColor Yellow
docker-compose ps

Write-Host "5. 查看日志" -ForegroundColor Cyan
Write-Host "查看最近10条日志..." -ForegroundColor Yellow
docker-compose logs --tail=10

Write-Host "=== 部署完成 ===" -ForegroundColor Green
Write-Host "服务已启动，访问地址: http://localhost:8080" -ForegroundColor Green
Write-Host "健康检查地址: http://localhost:8080/health" -ForegroundColor Green
