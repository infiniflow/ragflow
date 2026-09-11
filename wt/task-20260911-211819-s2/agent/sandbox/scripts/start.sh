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

if [ -f .env ]; then
  source .env
  SANDBOX_EXECUTOR_MANAGER_PORT="${SANDBOX_EXECUTOR_MANAGER_PORT:-9385}"        # Default to 9385 if not set in .env
  SANDBOX_EXECUTOR_MANAGER_POOL_SIZE="${SANDBOX_EXECUTOR_MANAGER_POOL_SIZE:-5}" # Default to 5 if not set in .env
  SANDBOX_BASE_PYTHON_IMAGE=${SANDBOX_BASE_PYTHON_IMAGE-"sandbox-base-python:latest"}
  SANDBOX_BASE_NODEJS_IMAGE=${SANDBOX_BASE_NODEJS_IMAGE-"sandbox-base-nodejs:latest"}
else
  echo "⚠️ .env not found, using default ports and pool size"
  SANDBOX_EXECUTOR_MANAGER_PORT=9385
  SANDBOX_EXECUTOR_MANAGER_POOL_SIZE=5
  SANDBOX_BASE_PYTHON_IMAGE=sandbox-base-python:latest
  SANDBOX_BASE_NODEJS_IMAGE=sandbox-base-nodejs:latest
fi

echo "📦 STEP 1: Build sandbox-base image ..."
if [ -f .env ]; then
  source .env &&
    echo "🐍 Building base sandbox image for Python ($SANDBOX_BASE_PYTHON_IMAGE)..." &&
    docker build -t "$SANDBOX_BASE_PYTHON_IMAGE" ./sandbox_base_image/python &&
    echo "⬢ Building base sandbox image for Nodejs ($SANDBOX_BASE_NODEJS_IMAGE)..." &&
    docker build -t "$SANDBOX_BASE_NODEJS_IMAGE" ./sandbox_base_image/nodejs
else
  echo "⚠️ .env file not found, skipping build."
fi

echo "🧹 STEP 2: Clean up old sandbox containers (sandbox_nodejs_0~$((SANDBOX_EXECUTOR_MANAGER_POOL_SIZE - 1)) and sandbox_python_0~$((SANDBOX_EXECUTOR_MANAGER_POOL_SIZE - 1))) ..."
for i in $(seq 0 $((SANDBOX_EXECUTOR_MANAGER_POOL_SIZE - 1))); do
  echo "🧹 Deleting sandbox_python_$i..."
  docker rm -f "sandbox_python_$i" >/dev/null 2>&1 || true

  echo "🧹 Deleting sandbox_nodejs_$i..."
  docker rm -f "sandbox_nodejs_$i" >/dev/null 2>&1 || true
done

echo "🔧 STEP 3: Build executor services ..."
docker compose build

echo "🚀 STEP 4: Start services ..."
docker compose up -d

echo "⏳ STEP 5a: Check if ports are open (basic connectivity) ..."
bash ./scripts/wait-for-it.sh "localhost" "$SANDBOX_EXECUTOR_MANAGER_PORT" -t 30

echo "⏳ STEP 5b: Check if the interfaces are healthy (/healthz) ..."
bash ./scripts/wait-for-it-http.sh "http://localhost:$SANDBOX_EXECUTOR_MANAGER_PORT/healthz" 30

echo "✅ STEP 6: Run security tests ..."
python3 ./tests/sandbox_security_tests_full.py

echo "🎉 Service is ready: http://localhost:$SANDBOX_EXECUTOR_MANAGER_PORT/docs"
