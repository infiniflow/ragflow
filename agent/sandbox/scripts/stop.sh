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
# Remove the sandbox containers that actually exist instead of walking a range derived from
# SANDBOX_EXECUTOR_MANAGER_POOL_SIZE. A pool created at one size and torn down after .env changed
# left every container above the new size running; a missing .env skipped cleanup altogether; and a
# container that never joined the executor-manager queue was never in the range to begin with.
# The pattern is anchored to the generated pool names, so an unrelated container that merely
# contains "sandbox_python_" in its name is not touched.
# `docker ps` failing and finding nothing look identical once both become an empty string, and
# the difference matters: the first leaves every sandbox container running while reporting
# success. Only grep's no-match exit is tolerated here.
if ! all_containers="$(docker ps -a --format '{{.Names}}')"; then
  echo "❌ Could not list Docker containers; sandbox containers were NOT removed" >&2
  exit 1
fi
sandbox_containers="$(printf '%s\n' "$all_containers" |
  grep -E '^sandbox_(python|nodejs)_[0-9]+$' || true)"
if [ -n "$sandbox_containers" ]; then
  while IFS= read -r container; do
    echo "🧹 Deleting $container..."
    docker rm -f "$container" >/dev/null 2>&1 || true
  done <<<"$sandbox_containers"
else
  echo "✅ No sandbox containers found"
fi

echo "✅ Stopping and cleanup complete"
