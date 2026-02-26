#!/bin/bash

# 进入 docker 目录，确保相对路径正确
cd "$(dirname "$0")/../docker" || exit 1

echo "🚀 Starting RAGFlow DEV environment..."
echo "   - Project: ragflow-dev"
echo "   - Ports: 10080 (Web), 15455 (MySQL), 19200 (ES)"
echo "   - Config: .env.dev"

# 核心命令
# --env-file: 指定环境变量文件
# -p: 指定项目名（隔离关键）
# -f: 堆叠配置文件 (Base + Dev Patch)
docker compose --env-file .env.dev -p ragflow-dev -f docker-compose.yml -f docker-compose.dev.yml up -d

echo "✅ RAGFlow DEV is running at http://localhost:10080"




