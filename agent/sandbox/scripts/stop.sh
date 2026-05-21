#!/bin/bash
#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

set -e

BASE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$BASE_DIR"

echo "🛑 Stopping all services..."
docker compose down

echo "🧹 Deleting sandbox containers..."
if [ -f .env ]; then
  source .env
  for i in $(seq 0 $((SANDBOX_EXECUTOR_MANAGER_POOL_SIZE - 1))); do
    echo "🧹 Deleting sandbox_python_$i..."
    docker rm -f "sandbox_python_$i" >/dev/null 2>&1 || true

    echo "🧹 Deleting sandbox_nodejs_$i..."
    docker rm -f "sandbox_nodejs_$i" >/dev/null 2>&1 || true
  done
else
  echo "⚠️ .env not found, skipping container cleanup"
fi

echo "✅ Stopping and cleanup complete"
