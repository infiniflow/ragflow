#!/bin/bash
# RAGFlow Docker 启动脚本
# 使用自定义模型下载目录

# 设置模型下载目录
export RAGFLOW_MODEL_DIR="E:/github_myrepo/ragflow/ragflow/docker_download"

# 创建目录
mkdir -p "$RAGFLOW_MODEL_DIR"

# 进入 docker 目录
cd "$(dirname "$0")/docker"

# 停止现有容器
docker compose -f docker-compose-base.yml down
docker compose -f docker-compose.yml --profile cpu down

# 启动基础服务
docker compose -f docker-compose-base.yml up -d

# 启动 RAGFlow 服务（挂载自定义模型目录）
docker compose -f docker-compose.yml --profile cpu up -d

# 查看日志
echo "等待服务启动..."
sleep 10
docker logs -f docker-ragflow-cpu-1
