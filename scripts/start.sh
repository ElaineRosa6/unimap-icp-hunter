#!/bin/bash

# 启动脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=== UniMap 启动脚本 ==="

echo "1. 启动服务"
echo "启动中..."
docker-compose up -d

echo "2. 检查服务状态"
echo "检查中..."
docker-compose ps

echo "3. 查看日志"
echo "查看最近10条日志..."
docker-compose logs --tail=10

echo "=== 启动完成 ==="
echo "服务已启动，访问地址: http://localhost:8080"
echo "健康检查地址: http://localhost:8080/health"
