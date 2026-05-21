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

host=$1
port=$2
shift 2

timeout=15
quiet=0

while [[ $# -gt 0 ]]; do
  case "$1" in
  -t | --timeout)
    timeout="$2"
    shift 2
    ;;
  -q | --quiet)
    quiet=1
    shift
    ;;
  *)
    break
    ;;
  esac
done

for i in $(seq "$timeout"); do
  if nc -z "$host" "$port" >/dev/null 2>&1; then
    [[ "$quiet" -ne 1 ]] && echo "✔ $host:$port is available after $i seconds"
    exit 0
  fi
  sleep 1
done

echo "✖ Timeout after $timeout seconds waiting for $host:$port"
exit 1
