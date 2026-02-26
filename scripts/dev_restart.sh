#!/bin/bash

# 复用 stop 和 start 逻辑，或者直接 restart
cd "$(dirname "$0")/../docker" || exit 1

echo "🔄 Restarting RAGFlow DEV environment..."

docker compose --env-file .env.dev -p ragflow-dev -f docker-compose.yml -f docker-compose.dev.yml restart

echo "✅ RAGFlow DEV restarted."
