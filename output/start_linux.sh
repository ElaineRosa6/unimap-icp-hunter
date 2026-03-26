#!/bin/bash

echo "==============================================="
echo "UniMAP Multi-Engine Search Service"
echo "==============================================="
echo

# 设置环境变量（可选）
# export QUAKE_API_KEY=your_quake_api_key
# export ZOOMEYE_API_KEY=your_zoomeye_api_key
# export HUNTER_API_KEY=your_hunter_api_key
# export FOFA_API_KEY=your_fofa_api_key
# export FOFA_EMAIL=your_fofa_email

echo "Starting UniMAP service..."
echo

# 赋予执行权限
chmod +x ./unimap-linux-amd64

# 启动服务
./unimap-linux-amd64