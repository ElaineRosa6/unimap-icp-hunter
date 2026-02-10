#!/bin/bash

# 部署脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=== UniMap 部署脚本 ==="

echo "1. 检查Docker和Docker Compose是否安装"
if ! command -v docker &> /dev/null; then
    echo "错误: Docker未安装，请先安装Docker"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "错误: Docker Compose未安装，请先安装Docker Compose"
    exit 1
fi

echo "2. 构建镜像"
echo "构建中..."
docker-compose build

echo "3. 启动服务"
echo "启动中..."
docker-compose up -d

echo "4. 检查服务状态"
echo "检查中..."
docker-compose ps

echo "5. 查看日志"
echo "查看最近10条日志..."
docker-compose logs --tail=10

echo "=== 部署完成 ==="
echo "服务已启动，访问地址: http://localhost:8080"
echo "健康检查地址: http://localhost:8080/health"
