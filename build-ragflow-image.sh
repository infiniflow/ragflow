#!/usr/bin/env bash
#
# Build the RAGFlow Docker image from source on a Linux server.
#
# Pulls the repository (your GitLab) to a local dir, checks out the target
# branch, builds the image with the repo's official Dockerfile, and optionally
# (re)creates the app container via docker compose.
#
# Server-local config (docker/.env, docker/service_conf.yaml.template) is
# preserved across updates, so you can keep your own values there.
#
set -euo pipefail

REPO_URL="${RAGFLOW_REPO_URL:-https://gitlab.citysense.ru/ragflow-ecosystem/ragflow}"
BRANCH="${RAGFLOW_BRANCH:-main}"
TAG="${RAGFLOW_IMAGE_TAG:-infiniflow/ragflow:nightly}"
DATA_DIR="${RAGFLOW_DATA_DIR:-$HOME/ragflow}"
DEPLOY=0
NO_CACHE=0

usage() {
  cat <<EOF
Usage: $0 [options]

Options:
  --repo <url>       Git repository to pull (default: \$RAGFLOW_REPO_URL or the GitLab repo)
  --branch <name>    Branch to build (default: \$RAGFLOW_BRANCH or main)
  --tag <image:tag>  Result image tag (default: \$RAGFLOW_IMAGE_TAG or infiniflow/ragflow:nightly)
  --data-dir <path>  Where the repo lives on this server (default: \$RAGFLOW_DATA_DIR or ~/ragflow)
  --deploy           After building, (re)create the ragflow-cpu container via docker compose
  --no-cache         Full rebuild without BuildKit cache
  -h, --help         Show this help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)     REPO_URL="$2"; shift 2 ;;
    --branch)   BRANCH="$2"; shift 2 ;;
    --tag)      TAG="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --deploy)   DEPLOY=1; shift ;;
    --no-cache) NO_CACHE=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

log() { echo -e "\n=== $* ==="; }

# --- 1. Docker daemon -------------------------------------------------
log "Checking Docker daemon"
docker info >/dev/null 2>&1 || { echo "ERROR: Docker daemon is not running." >&2; exit 1; }

# --- 2. Clone / update the repository ---------------------------------
log "Repository: $DATA_DIR (branch: $BRANCH)"
if [ ! -d "$DATA_DIR/.git" ]; then
  echo "Cloning $REPO_URL ..."
  git clone "$REPO_URL" "$DATA_DIR"
fi
cd "$DATA_DIR"

# Preserve server-local config files across updates
BACKUP_DIR="$(mktemp -d)"
trap 'rm -rf "$BACKUP_DIR"' EXIT
for f in docker/.env docker/service_conf.yaml.template; do
  if [ -f "$f" ]; then
    mkdir -p "$BACKUP_DIR/$(dirname "$f")"
    cp "$f" "$BACKUP_DIR/$f"
  fi
done

git fetch origin "$BRANCH"
git checkout "$BRANCH" 2>/dev/null || git checkout -b "$BRANCH" --track origin/"$BRANCH"
git reset --hard origin/"$BRANCH"

# Restore server-local config
for f in docker/.env docker/service_conf.yaml.template; do
  if [ -f "$BACKUP_DIR/$f" ]; then
    mkdir -p "$(dirname "$f")"
    cp "$BACKUP_DIR/$f" "$f"
    echo "Restored $f (server-local values kept)"
  fi
done

# --- 3. Build the image -----------------------------------------------
log "Building $TAG"
BUILD_ARGS=(build --progress=plain -f Dockerfile -t "$TAG")
if [ "$NO_CACHE" = "1" ]; then
  BUILD_ARGS+=(--no-cache)
fi
docker "${BUILD_ARGS[@]}" .

# --- 4. Optional deploy ----------------------------------------------
if [ "$DEPLOY" = "1" ]; then
  log "Starting docker compose (base services + app)"
  cd docker
  # up -d (без имени сервиса): поднимает ВСЕ сервисы — es01, mysql, minio,
  # redis из include docker-compose-base.yml + ragflow-cpu/ragflow-gpu.
  # Compose сам пересоздаст ragflow-cpu, если образ изменился.
  docker compose -f docker-compose.yml up -d
  echo "Check: docker compose -f docker-compose.yml ps"
  echo "Follow logs: docker logs -f docker-ragflow-cpu-1"
fi

log "Done"
docker images "$TAG" --format "  {{.Repository}}:{{.Tag}}   ID={{.ID}}   Size={{.Size}}"
