#!/usr/bin/env bash
# Invoke after the native build. All Go packages run against disposable storage.
set -euo pipefail
cd "$(dirname "$0")/.."

resource_dir=$(mktemp -d "${RUNNER_TEMP:-/tmp}/ragflow-test-resources.XXXXXX")
minio_id=""
cleanup() {
  if [[ -n "$minio_id" ]]; then
    sudo docker rm -f -v "$minio_id"
  fi
  rm -rf -- "$resource_dir"
}
trap cleanup EXIT
resource_commit=0937399b60f1949267388548e33ea0d5c0cc25f7
git -C "$resource_dir" init --quiet
git -C "$resource_dir" remote add origin https://github.com/infiniflow/resource.git
git -C "$resource_dir" fetch --quiet --depth 1 origin "$resource_commit"
git -C "$resource_dir" checkout --quiet --detach FETCH_HEAD
test "$(git -C "$resource_dir" rev-parse HEAD)" = "$resource_commit"
test -s "$resource_dir/rag/huqie.txt"
export RAGFLOW_DICT_PATH="$resource_dir"
export RAGFLOW_TEST_MINIO_USER=regression
export RAGFLOW_TEST_MINIO_PASSWORD=regression-only-minio
minio_id=$(sudo docker run -d --label ragflow.regression=go \
  -p "${RAGFLOW_TEST_BIND_ADDRESS:-127.0.0.1}::9000" \
  -e MINIO_ROOT_USER="$RAGFLOW_TEST_MINIO_USER" \
  -e MINIO_ROOT_PASSWORD="$RAGFLOW_TEST_MINIO_PASSWORD" \
  pgsty/minio:RELEASE.2026-03-25T00-00-00Z server /data)
export RAGFLOW_TEST_MINIO_ENDPOINT
minio_port=$(sudo docker inspect --format '{{(index (index .NetworkSettings.Ports "9000/tcp") 0).HostPort}}' "$minio_id")
RAGFLOW_TEST_MINIO_ENDPOINT="${RAGFLOW_TEST_DOCKER_HOST:-127.0.0.1}:$minio_port"
ready=0
for attempt in $(seq 1 60); do
  if curl --noproxy '*' -fsS "http://$RAGFLOW_TEST_MINIO_ENDPOINT/minio/health/ready" >/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
test "$ready" -eq 1
./build.sh --test ./...
