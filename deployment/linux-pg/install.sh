#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
SOURCE_ROOT=$(cd -- "${SCRIPT_DIR}/../.." && pwd)
INSTALL_DIR=${INSTALL_DIR:-/opt/ragflow-pg}
PROJECT_NAME=${PROJECT_NAME:-ragflow-pg}
RAGFLOW_PORT=${RAGFLOW_PORT:-80}
ADMIN_EMAIL=${ADMIN_EMAIL:-admin@ragflow.local}
ADMIN_NICKNAME=${ADMIN_NICKNAME:-RAGFlow Admin}
SECRETS_DIR=${SECRETS_DIR:-/etc/ragflow-pg}
ADMIN_PASSWORD=${ADMIN_PASSWORD:-}
OFFLINE_INSTALL=${OFFLINE_INSTALL:-0}
REGISTRY_INSTALL=${REGISTRY_INSTALL:-0}
PREBUILT_FRONTEND=${PREBUILT_FRONTEND:-${OFFLINE_INSTALL}}
IMAGE_ENV_FILE=${IMAGE_ENV_FILE:-}
REGISTRY_HOST=${REGISTRY_HOST:-}
REGISTRY_USERNAME=${REGISTRY_USERNAME:-}
REGISTRY_PASSWORD_FILE=${REGISTRY_PASSWORD_FILE:-}
DOCKER_DNF_REPO=${DOCKER_DNF_REPO:-cifra-docker}
GVISOR_BUNDLE_DIR=${GVISOR_BUNDLE_DIR:-}
MIN_DOCKER_VERSION=24.0.0
MIN_COMPOSE_VERSION=2.26.1

[[ ${OFFLINE_INSTALL} == "0" || ${OFFLINE_INSTALL} == "1" ]] || { echo "OFFLINE_INSTALL must be 0 or 1." >&2; exit 1; }
[[ ${REGISTRY_INSTALL} == "0" || ${REGISTRY_INSTALL} == "1" ]] || { echo "REGISTRY_INSTALL must be 0 or 1." >&2; exit 1; }
[[ ${PREBUILT_FRONTEND} == "0" || ${PREBUILT_FRONTEND} == "1" ]] || { echo "PREBUILT_FRONTEND must be 0 or 1." >&2; exit 1; }
[[ ${OFFLINE_INSTALL} != "1" || ${REGISTRY_INSTALL} != "1" ]] || {
  echo "OFFLINE_INSTALL and REGISTRY_INSTALL cannot both be enabled." >&2
  exit 1
}

if [[ $(uname -m) != "x86_64" ]]; then
  echo "Only x86_64 Linux is supported by this release package." >&2
  exit 1
fi

if [[ ! -r /etc/os-release ]]; then
  echo "/etc/os-release is required." >&2
  exit 1
fi
. /etc/os-release
case ${ID:-} in
  ubuntu|debian)
    PACKAGE_FAMILY=deb
    ;;
  rocky)
    if [[ ${VERSION_ID:-} != 9 && ${VERSION_ID:-} != 9.* ]]; then
      echo "Rocky Linux 9.x is required; found ${VERSION_ID:-unknown}." >&2
      exit 1
    fi
    PACKAGE_FAMILY=rpm
    ;;
  *)
    echo "Supported distributions: Ubuntu, Debian, and Rocky Linux 9.x; found ${ID:-unknown}." >&2
    exit 1
    ;;
esac

SOURCE_MANIFEST=${SOURCE_ROOT}/DEPLOYMENT-SOURCE.env
if [[ ! -r ${SOURCE_MANIFEST} ]]; then
  echo "DEPLOYMENT-SOURCE.env is missing. Extract the approved release archive before installation." >&2
  exit 1
fi
manifest_value() {
  local key=$1
  awk -F= -v key="${key}" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "${SOURCE_MANIFEST}"
}
RELEASE_VERSION=$(manifest_value RELEASE_VERSION)
PACKAGE_FORMAT=$(manifest_value PACKAGE_FORMAT)
if [[ -z ${RELEASE_VERSION} || ${PACKAGE_FORMAT} != "tar.gz" ]]; then
  echo "DEPLOYMENT-SOURCE.env does not describe a supported release archive." >&2
  exit 1
fi
if [[ ${SOURCE_ROOT} == ${INSTALL_DIR} ]]; then
  echo "INSTALL_DIR must differ from the extracted source directory." >&2
  exit 1
fi
sudo -v
if sudo test -e "${INSTALL_DIR}"; then
  if ! sudo test -d "${INSTALL_DIR}"; then
    echo "INSTALL_DIR exists and is not a directory: ${INSTALL_DIR}" >&2
    exit 1
  fi
  if [[ -n $(sudo find "${INSTALL_DIR}" -mindepth 1 -maxdepth 1 -print -quit) ]]; then
    echo "INSTALL_DIR is not empty. This script is for first install only: ${INSTALL_DIR}" >&2
    exit 1
  fi
fi
if [[ ${OFFLINE_INSTALL} == "1" ]]; then
  if [[ ${ID} != "rocky" || (${VERSION_ID:-} != 9 && ${VERSION_ID:-} != 9.*) ]]; then
    echo "The offline package supports Rocky Linux 9.x only; found ${ID} ${VERSION_ID:-unknown}." >&2
    exit 1
  fi
  for command_name in curl docker jq openssl rsync tar; do
    command -v "${command_name}" >/dev/null || {
      echo "Required offline prerequisite is missing: ${command_name}" >&2
      exit 1
    }
  done
  sudo docker compose version >/dev/null
else
  case ${PACKAGE_FAMILY} in
    deb)
      sudo apt-get update
      sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y \
        ca-certificates curl jq openssl rsync tar docker.io docker-compose-v2
      ;;
    rpm)
      ENABLED_DNF_REPOS=$(sudo dnf -q repolist --enabled | awk 'NR > 1 { print $1 }')
      if ! grep -Fxq "${DOCKER_DNF_REPO}" <<<"${ENABLED_DNF_REPOS}"; then
        echo "Required Docker DNF repository is not enabled: ${DOCKER_DNF_REPO}" >&2
        exit 1
      fi
      sudo dnf install -y --enablerepo="${DOCKER_DNF_REPO}" \
        ca-certificates curl jq openssl rsync tar \
        docker-ce docker-ce-cli containerd.io \
        docker-buildx-plugin docker-compose-plugin
      ;;
  esac
fi
sudo systemctl enable --now docker
DOCKER_RUNTIMES=$(sudo docker info --format '{{json .Runtimes}}')
if ! grep -q '"runsc"' <<<"${DOCKER_RUNTIMES}"; then
  [[ -n ${GVISOR_BUNDLE_DIR} ]] || { echo 'Docker runsc runtime is not registered and no gVisor bundle was provided.' >&2; exit 1; }
  sudo env GVISOR_BUNDLE_DIR="${GVISOR_BUNDLE_DIR}" bash "${SCRIPT_DIR}/install_gvisor.sh"
fi

if [[ ${REGISTRY_INSTALL} == "1" && -n ${REGISTRY_USERNAME} ]]; then
  if [[ -z ${REGISTRY_HOST} || -z ${REGISTRY_PASSWORD_FILE} ]]; then
    echo "REGISTRY_HOST and REGISTRY_PASSWORD_FILE are required when REGISTRY_USERNAME is set." >&2
    exit 1
  fi
  if [[ ! -r ${REGISTRY_PASSWORD_FILE} ]]; then
    echo "Registry password file is not readable: ${REGISTRY_PASSWORD_FILE}" >&2
    exit 1
  fi
  sudo docker login "${REGISTRY_HOST}" --username "${REGISTRY_USERNAME}" --password-stdin < "${REGISTRY_PASSWORD_FILE}"
fi

version_at_least() {
  local actual=$1 minimum=$2
  [[ $(printf '%s\n%s\n' "${minimum}" "${actual}" | sort -V | head -n 1) == "${minimum}" ]]
}

DOCKER_VERSION=$(sudo docker version --format '{{.Server.Version}}')
COMPOSE_VERSION=$(sudo docker compose version --short)
COMPOSE_VERSION=${COMPOSE_VERSION#v}
if ! version_at_least "${DOCKER_VERSION}" "${MIN_DOCKER_VERSION}"; then
  echo "Docker ${DOCKER_VERSION} is too old; ${MIN_DOCKER_VERSION} or newer is required." >&2
  exit 1
fi
if ! version_at_least "${COMPOSE_VERSION}" "${MIN_COMPOSE_VERSION}"; then
  echo "Docker Compose ${COMPOSE_VERSION} is too old; ${MIN_COMPOSE_VERSION} or newer is required." >&2
  exit 1
fi

printf 'vm.max_map_count=262144\nnet.ipv4.ip_forward=1\n' | \
  sudo tee /etc/sysctl.d/99-ragflow-pg.conf >/dev/null
sudo sysctl --system >/dev/null
test "$(sysctl -n vm.max_map_count)" -ge 262144
test "$(sysctl -n net.ipv4.ip_forward)" -eq 1

sudo install -d -m 0755 "${INSTALL_DIR}"
sudo rsync -a --delete \
  --exclude .git --exclude .venv --exclude .codex_tmp --exclude .playwright-cli \
  --exclude node_modules --exclude output --exclude docker/ragflow-logs \
  "${SOURCE_ROOT}/" "${INSTALL_DIR}/"

if [[ ${PREBUILT_FRONTEND} == "1" ]]; then
  test -s "${INSTALL_DIR}/web/dist/index.html"
else
  FRONTEND_DEPS_VOLUME="${PROJECT_NAME}_frontend_build_deps_$$"
  cleanup_frontend_deps() {
    sudo docker volume rm -f "${FRONTEND_DEPS_VOLUME}" >/dev/null 2>&1 || true
  }
  trap cleanup_frontend_deps ERR INT TERM
  sudo docker volume create "${FRONTEND_DEPS_VOLUME}" >/dev/null
  sudo docker run --rm \
    -e NODE_OPTIONS=--max-old-space-size=8192 \
    -e npm_config_cache=/tmp/npm-cache \
    -v "${INSTALL_DIR}:/workspace" \
    -v "${FRONTEND_DEPS_VOLUME}:/workspace/web/node_modules" \
    -w /workspace/web \
    node:20-bookworm-slim \
    sh -lc 'npm ci && npm run build'
  cleanup_frontend_deps
  trap - ERR INT TERM
fi

sudo chown -R root:root "${INSTALL_DIR}"
sudo find "${INSTALL_DIR}" -type d -exec chmod 0755 {} +
sudo find "${INSTALL_DIR}" -type f -name '*.sh' -exec chmod 0755 {} +
test -s "${INSTALL_DIR}/web/dist/index.html"

ENV_FILE=${INSTALL_DIR}/docker/.env
sudo install -m 0600 "${INSTALL_DIR}/deployment/linux-pg/env.template" "${ENV_FILE}"

random_secret() { openssl rand -hex 32; }
set_env() {
  local key=$1 value=$2 escaped
  escaped=$(printf '%s' "${value}" | sed 's/[&|]/\\&/g')
  sudo sed -i "s|^${key}=.*|${key}=${escaped}|" "${ENV_FILE}"
}
set_env POSTGRES_PASSWORD "$(random_secret)"
set_env ELASTIC_PASSWORD "$(random_secret)"
set_env OPENSEARCH_PASSWORD "$(random_secret)"
set_env MINIO_PASSWORD "$(random_secret)"
set_env REDIS_PASSWORD "$(random_secret)"
set_env RAGFLOW_CREDENTIALS_KEY "$(random_secret)"
set_env GRAFANA_ADMIN_PASSWORD "$(random_secret)"
set_env SVR_WEB_HTTP_PORT "${RAGFLOW_PORT}"

if [[ -n ${IMAGE_ENV_FILE} ]]; then
  if [[ ! -r ${IMAGE_ENV_FILE} ]]; then
    echo "Image environment file is not readable: ${IMAGE_ENV_FILE}" >&2
    exit 1
  fi
  while IFS='=' read -r image_key image_value; do
    [[ -z ${image_key} || ${image_key} == \#* ]] && continue
    case ${image_key} in
      POSTGRES_IMAGE|RAGFLOW_IMAGE|VALKEY_IMAGE|ELASTICSEARCH_IMAGE|PLANTUML_IMAGE|MINIO_IMAGE|T_ONE_ASR_IMAGE|OTEL_COLLECTOR_IMAGE|TEMPO_IMAGE|LOKI_IMAGE|PROMETHEUS_IMAGE|GRAFANA_IMAGE|SANDBOX_EXECUTOR_MANAGER_IMAGE|SANDBOX_BASE_NODEJS_IMAGE|SANDBOX_BASE_PYTHON_IMAGE)
        if [[ -z ${image_value} || ${image_value} =~ [[:space:]] ]]; then
          echo "Invalid image reference for ${image_key}." >&2
          exit 1
        fi
        set_env "${image_key}" "${image_value}"
        ;;
      *)
        echo "Unsupported key in image environment file: ${image_key}" >&2
        exit 1
        ;;
    esac
  done < "${IMAGE_ENV_FILE}"
fi

sudo install -d -m 0700 "${SECRETS_DIR}"
if [[ -z ${ADMIN_PASSWORD} ]]; then
  ADMIN_PASSWORD=$(random_secret)
fi
sudo install -m 0600 /dev/null "${SECRETS_DIR}/admin.env"
sudo tee "${SECRETS_DIR}/admin.env" >/dev/null <<EOF
ADMIN_EMAIL=${ADMIN_EMAIL}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
EOF
sudo install -m 0644 /dev/null "${SECRETS_DIR}/deployed-source.env"
sudo tee "${SECRETS_DIR}/deployed-source.env" < "${SOURCE_MANIFEST}" >/dev/null
printf 'INSTALLED_AT_UTC=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" | \
  sudo tee -a "${SECRETS_DIR}/deployed-source.env" >/dev/null

cd "${INSTALL_DIR}/docker"
COMPOSE=(sudo docker compose --env-file .env -p "${PROJECT_NAME}" \
  -f docker-compose.yml \
  -f docker-compose.local.yml \
  -f docker-compose.linux.local.yml \
  -f docker-compose.observability.yml \
  -f ../deployment/linux-pg/docker-compose.release.yml)

"${COMPOSE[@]}" config --quiet
services=$("${COMPOSE[@]}" config --services)
if grep -qx mysql <<<"${services}"; then
  echo "Refusing to deploy: MySQL is active in the merged Compose config." >&2
  exit 1
fi
grep -qx postgres <<<"${services}"
for required_service in t-one-asr sandbox-executor-manager otel-collector tempo loki prometheus grafana; do
  grep -qx "${required_service}" <<<"${services}" || { echo "Required service is absent: ${required_service}" >&2; exit 1; }
done
DOCKER_RUNTIMES=$(sudo docker info --format '{{json .Runtimes}}')
grep -q '"runsc"' <<<"${DOCKER_RUNTIMES}" || { echo 'Docker runsc runtime is not registered.' >&2; exit 1; }

if [[ ${OFFLINE_INSTALL} == "1" ]]; then
  "${COMPOSE[@]}" up -d --no-build --pull never
elif [[ ${REGISTRY_INSTALL} == "1" ]]; then
  "${COMPOSE[@]}" pull
  "${COMPOSE[@]}" up -d --no-build --pull never
else
  "${COMPOSE[@]}" up -d --build
fi

deadline=$((SECONDS + 600))
until curl -fsS --max-time 10 "http://127.0.0.1:${RAGFLOW_PORT}/api/v1/system/healthz" | jq -e '.status == "ok"' >/dev/null; do
  if (( SECONDS >= deadline )); then
    "${COMPOSE[@]}" ps
    sudo docker logs --tail 200 "${PROJECT_NAME}-ragflow-cpu-1" || true
    echo "RAGFlow did not become healthy within 600 seconds." >&2
    exit 1
  fi
  sleep 5
done
curl -fsS --max-time 10 "http://127.0.0.1:${RAGFLOW_PORT}/" >/dev/null
curl -fsS --max-time 10 http://127.0.0.1:3001/api/health | jq -e '.database == "ok"' >/dev/null
curl -fsS --max-time 10 http://127.0.0.1:9090/-/ready >/dev/null
ASR_CONTAINER=$("${COMPOSE[@]}" ps -q t-one-asr)
SANDBOX_CONTAINER=$("${COMPOSE[@]}" ps -q sandbox-executor-manager)
test -n "${ASR_CONTAINER}" -a -n "${SANDBOX_CONTAINER}"
sudo docker exec "${ASR_CONTAINER}" python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:9011/health/ready', timeout=10)"
sudo docker exec "${SANDBOX_CONTAINER}" curl -fsS http://127.0.0.1:9385/healthz >/dev/null
SANDBOX_LOGS=$(sudo docker logs "${SANDBOX_CONTAINER}" 2>&1)
grep -Eq 'Container pool initialization complete: [1-9][0-9]*/[1-9][0-9]* available' <<<"${SANDBOX_LOGS}"
sudo grep -Eq '^RAGFLOW_CREDENTIALS_KEY=[0-9a-f]{64}$' "${ENV_FILE}"
sudo grep -Eq '^GRAFANA_ADMIN_PASSWORD=[0-9a-f]{64}$' "${ENV_FILE}"

RAGFLOW_CONTAINER=$("${COMPOSE[@]}" ps -q ragflow-cpu)
test -n "${RAGFLOW_CONTAINER}"

sudo docker exec -i \
  -e BOOTSTRAP_ADMIN_EMAIL="${ADMIN_EMAIL}" \
  -e BOOTSTRAP_ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
  -e BOOTSTRAP_ADMIN_NICKNAME="${ADMIN_NICKNAME}" \
  "${RAGFLOW_CONTAINER}" python - < "${INSTALL_DIR}/deployment/linux-pg/seed_admin.py"
sudo docker exec -i "${RAGFLOW_CONTAINER}" python - < "${INSTALL_DIR}/deployment/linux-pg/seed_asr.py"

sudo docker exec "${RAGFLOW_CONTAINER}" printenv DB_TYPE | grep -qx postgres
sudo docker exec -i \
  -e BOOTSTRAP_ADMIN_EMAIL="${ADMIN_EMAIL}" \
  "${RAGFLOW_CONTAINER}" python - <<'PY'
from common import settings
settings.init_settings()
from api.db.services import UserService
import os
email = os.environ["BOOTSTRAP_ADMIN_EMAIL"]
users = list(UserService.query(email=email))
assert len(users) == 1 and users[0].is_superuser
PY

"${COMPOSE[@]}" ps
echo
echo "RAGFlow installation completed."
echo "Release: ${RELEASE_VERSION}"
echo "Published address: 0.0.0.0:${RAGFLOW_PORT}"
echo "Local health URL: http://127.0.0.1:${RAGFLOW_PORT}/"
echo "Admin email: ${ADMIN_EMAIL}"
echo "Admin password is stored in ${SECRETS_DIR}/admin.env (mode 0600)."
echo "Database: PostgreSQL; pgvector is not required because DOC_ENGINE=elasticsearch."
cat "${SECRETS_DIR}/deployed-source.env"
