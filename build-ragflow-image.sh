#!/usr/bin/env bash
#
# Собирает Docker-образ RAGFlow из исходников на Linux-сервере.
#
# Клонирует репозиторий (ваш GitLab) в локальную папку, переключается на нужную
# ветку и собирает образ через официальный Dockerfile репозитория. Скрипт
# никогда не пересоздаёт контейнер приложения: запускайте его сами с собранным
# тегом.
#
# Локальные конфиги сервера (docker/.env, docker/service_conf.yaml.template)
# и правки под Yandex SSO (api/apps/auth/*, api/apps/restful_apis/user_api.py)
# сохраняются между обновлениями, так что свои значения можно держать там.
#
set -euo pipefail

REPO_URL="${RAGFLOW_REPO_URL:-https://gitlab.citysense.ru/ragflow-ecosystem/ragflow}"
BRANCH="${RAGFLOW_BRANCH:-main}"
COMMIT="${RAGFLOW_COMMIT:-HEAD}"
TAG="${RAGFLOW_IMAGE_TAG:-citysense/ragflow:$(date +%Y%m%d-%H%M%S)}"
DATA_DIR="${RAGFLOW_DATA_DIR:-$HOME/ragflow}"
NO_CACHE=0

usage() {
  cat <<EOF
Использование: $0 [параметры]

Параметры:
  --repo <url>       Git-репозиторий для клонирования (по умолчанию: \$RAGFLOW_REPO_URL или GitLab-репозиторий)
  --branch <имя>     Ветка для сборки (по умолчанию: \$RAGFLOW_BRANCH или main)
  --commit <sha>     Коммит для сборки (по умолчанию: \$RAGFLOW_COMMIT или HEAD)
  --tag <образ:тег>  Тег итогового образа (по умолчанию: \$RAGFLOW_IMAGE_TAG или citysense/ragflow:<дата-время>)
  --data-dir <путь>  Где на сервере лежит репозиторий (по умолчанию: \$RAGFLOW_DATA_DIR или ~/ragflow)
  --no-cache         Полная пересборка без кэша BuildKit
  -h, --help         Показать эту справку
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)     REPO_URL="$2"; shift 2 ;;
    --branch)   BRANCH="$2"; shift 2 ;;
    --commit)   COMMIT="$2"; shift 2 ;;
    --tag)      TAG="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --no-cache) NO_CACHE=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    *) echo "Неизвестный параметр: $1" >&2; usage; exit 1 ;;
  esac
done

log() { echo -e "\n=== $* ==="; }

# --- 1. Демон Docker -------------------------------------------------
log "Проверка демона Docker"
docker info >/dev/null 2>&1 || { echo "ОШИБКА: демон Docker не запущен." >&2; exit 1; }

# --- 2. Клонирование / обновление репозитория -------------------------
log "Репозиторий: $DATA_DIR (ветка: $BRANCH, коммит: $COMMIT)"
if [ ! -d "$DATA_DIR/.git" ]; then
  echo "Клонирование $REPO_URL ..."
  git clone "$REPO_URL" "$DATA_DIR"
fi
cd "$DATA_DIR"

# Сохраняем локальные конфиги сервера и правки под Yandex SSO между обновлениями
BACKUP_DIR="$(mktemp -d)"
trap 'rm -rf "$BACKUP_DIR"' EXIT
for f in docker/.env docker/service_conf.yaml.template \
         api/apps/auth/yandex.py api/apps/auth/__init__.py \
         api/apps/restful_apis/user_api.py; do
  if [ -f "$f" ]; then
    mkdir -p "$BACKUP_DIR/$(dirname "$f")"
    cp "$f" "$BACKUP_DIR/$f"
  fi
done

git fetch origin "$BRANCH"
git checkout "$BRANCH" 2>/dev/null || git checkout -b "$BRANCH" --track origin/"$BRANCH"
if [ "$COMMIT" = "HEAD" ]; then
  git reset --hard origin/"$BRANCH"
else
  git rev-parse --verify "$COMMIT^{commit}" >/dev/null
  git reset --hard "$COMMIT"
fi

# Восстанавливаем локальные конфиги и правки под Yandex SSO
for f in docker/.env docker/service_conf.yaml.template \
         api/apps/auth/yandex.py api/apps/auth/__init__.py \
         api/apps/restful_apis/user_api.py; do
  if [ -f "$BACKUP_DIR/$f" ]; then
    mkdir -p "$(dirname "$f")"
    cp "$BACKUP_DIR/$f" "$f"
    echo "Восстановлен $f (локальные значения сохранены)"
  fi
done

# --- 3. Сборка образа -------------------------------------------------
log "Сборка $TAG"
BUILD_ARGS=(build --progress=plain -f Dockerfile -t "$TAG")
if [ "$NO_CACHE" = "1" ]; then
  BUILD_ARGS+=(--no-cache)
fi
docker "${BUILD_ARGS[@]}" .

log "Готово"
echo "Чтобы запустить приложение, укажите RAGFLOW_IMAGE=$TAG в docker/.env и выполните:"
echo "  docker compose -f docker/docker-compose.yml up -d ragflow-cpu"
docker images "$TAG" --format "  {{.Repository}}:{{.Tag}}   ID={{.ID}}   Size={{.Size}}"
