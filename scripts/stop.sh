#!/bin/bash

# 停止脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=== UniMap 停止脚本 ==="

echo "1. 停止服务"
echo "停止中..."
docker-compose down

echo "2. 检查服务状态"
echo "检查中..."
docker-compose ps

echo "=== 停止完成 ==="
echo "服务已停止"
