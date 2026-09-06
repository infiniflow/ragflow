#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
SOURCE_ROOT=$(cd -- "${SCRIPT_DIR}/../.." && pwd)
INSTALL_DIR=${INSTALL_DIR:-/opt/ragflow-pg}
PROJECT_NAME=${PROJECT_NAME:-ragflow-pg}
RAGFLOW_PORT=${RAGFLOW_PORT:-}
SECRETS_DIR=${SECRETS_DIR:-/etc/ragflow-pg}
BACKUP_ROOT=${BACKUP_ROOT:-/var/backups/ragflow-pg}
OFFLINE_INSTALL=${OFFLINE_INSTALL:-0}
REGISTRY_INSTALL=${REGISTRY_INSTALL:-0}
IMAGE_ENV_FILE=${IMAGE_ENV_FILE:-}
BACKUP_MINIO=${BACKUP_MINIO:-1}
ALLOW_DOWNGRADE=${ALLOW_DOWNGRADE:-0}
ALLOW_UNHEALTHY=${ALLOW_UNHEALTHY:-0}
CHECK_ONLY=0
MIN_DOCKER_VERSION=24.0.0
MIN_COMPOSE_VERSION=2.26.1

[[ ${INSTALL_DIR} == /* && ${INSTALL_DIR} != "/" ]] || { echo "INSTALL_DIR must be an absolute non-root path." >&2; exit 1; }
[[ ${BACKUP_ROOT} == /* && ${BACKUP_ROOT} != "/" ]] || { echo "BACKUP_ROOT must be an absolute non-root path." >&2; exit 1; }
[[ ${PROJECT_NAME} =~ ^[a-z0-9][a-z0-9_-]*$ ]] || { echo "Invalid Compose project name: ${PROJECT_NAME}" >&2; exit 1; }
[[ ${BACKUP_MINIO} == "0" || ${BACKUP_MINIO} == "1" ]] || { echo "BACKUP_MINIO must be 0 or 1." >&2; exit 1; }
[[ ${ALLOW_DOWNGRADE} == "0" || ${ALLOW_DOWNGRADE} == "1" ]] || { echo "ALLOW_DOWNGRADE must be 0 or 1." >&2; exit 1; }
[[ ${ALLOW_UNHEALTHY} == "0" || ${ALLOW_UNHEALTHY} == "1" ]] || { echo "ALLOW_UNHEALTHY must be 0 or 1." >&2; exit 1; }
[[ ${OFFLINE_INSTALL} == "0" || ${OFFLINE_INSTALL} == "1" ]] || { echo "OFFLINE_INSTALL must be 0 or 1." >&2; exit 1; }
[[ ${REGISTRY_INSTALL} == "0" || ${REGISTRY_INSTALL} == "1" ]] || { echo "REGISTRY_INSTALL must be 0 or 1." >&2; exit 1; }
[[ ${OFFLINE_INSTALL} != "1" || ${REGISTRY_INSTALL} != "1" ]] || {
  echo "OFFLINE_INSTALL and REGISTRY_INSTALL cannot both be enabled." >&2
  exit 1
}

if [[ ${1:-} == "--check" ]]; then
  CHECK_ONLY=1
  shift
fi
if [[ $# -ne 0 ]]; then
  echo "Usage: $0 [--check]" >&2
  exit 2
fi

die() {
  echo "$*" >&2
  exit 1
}

manifest_value() {
  local manifest=$1 key=$2
  awk -F= -v key="${key}" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "${manifest}"
}

# Unsigned source identity metadata: not authenticity, running-image or schema proof.
candidate_source_id() {
  local path=$1 expected=${2#v} value
  [[ -f ${path} && -r ${path} && ! -L ${path} ]] || die "Unreadable or linked candidate manifest: ${path}"
  value=$(jq -er --arg expected "${expected}" '
    select(type == "object")
    | select((.source_id | type) == "string")
    | select((.source_id | length) == 64)
    | select(.source_id | test("^[0-9a-f]{64}$"))
    | select((.identity.version | type) == "string")
    | select(.identity.version | test("^[0-9]+\\.[0-9]+\\.[0-9]+$"))
    | select($expected == "unknown" or .identity.version == $expected)
    | .source_id
  ' "${path}") || die "Invalid candidate source identity/version: ${path}"
  [[ ${value} =~ ^[0-9a-f]{64}$ ]] || die "Invalid candidate source identity/version: ${path}"
  printf '%s' "${value}"
}

env_value() {
  local env_file=$1 key=$2 default_value=${3:-} value
  value=$(awk -F= -v key="${key}" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "${env_file}")
  printf '%s' "${value:-${default_value}}"
}

version_at_least() {
  local actual=$1 minimum=$2
  [[ $(printf '%s\n%s\n' "${minimum}" "${actual}" | sort -V | head -n 1) == "${minimum}" ]]
}

set_env() {
  local env_file=$1 key=$2 value=$3 escaped
  escaped=$(printf '%s' "${value}" | sed 's/[&|]/\\&/g')
  if grep -q "^${key}=" "${env_file}"; then
    sed -i "s|^${key}=.*|${key}=${escaped}|" "${env_file}"
  else
    printf '%s=%s\n' "${key}" "${value}" >> "${env_file}"
  fi
}

compose() {
  local root=$1
  shift
  (
    cd "${root}/docker"
    sudo docker compose --env-file .env -p "${PROJECT_NAME}" \
      -f docker-compose.yml \
      -f docker-compose.local.yml \
      -f docker-compose.linux.local.yml \
      -f docker-compose.observability.yml \
      -f ../deployment/linux-pg/docker-compose.release.yml \
      "$@"
  )
}

validate_compose() {
  local root=$1 require_full_stack=${2:-1} services
  compose "${root}" config --quiet
  services=$(compose "${root}" config --services)
  grep -qx postgres <<<"${services}" || die "PostgreSQL is absent from the merged Compose config."
  if grep -qx mysql <<<"${services}"; then
    die "Refusing to upgrade: MySQL is active in the merged Compose config."
  fi
  if [[ ${require_full_stack} == "1" ]]; then
    for required_service in t-one-asr sandbox-executor-manager otel-collector tempo loki prometheus grafana; do
      grep -qx "${required_service}" <<<"${services}" || die "Required service is absent: ${required_service}"
    done
  fi
}

verify_runsc_runtime() {
  local docker_runtimes
  docker_runtimes=$(sudo docker info --format '{{json .Runtimes}}')
  grep -q '"runsc"' <<<"${docker_runtimes}" \
    || die "Docker runsc runtime is not registered. Install the pinned gVisor bundle first."
}

verify_offline_images() {
  local root=$1 image_name
  while IFS= read -r image_name; do
    [[ -z ${image_name} ]] && continue
    sudo docker image inspect "${image_name}" >/dev/null || die "Offline image is not loaded: ${image_name}"
  done < <(compose "${root}" config --images | sort -u)
}

verify_installed_audit_table() {
  local root=$1 postgres_container
  postgres_container=$(compose "${root}" ps -q postgres)
  [[ -n ${postgres_container} ]] || return 1
  sudo docker exec "${postgres_container}" sh -lc \
    'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select to_regclass('\''public.system_audit_event'\'') is not null"' \
    | grep -qx t
}

wait_for_health() {
  local deadline health_file=$1
  deadline=$((SECONDS + 600))
  until curl -fsS --max-time 10 "http://127.0.0.1:${RAGFLOW_PORT}/api/v1/system/healthz" \
    | sudo tee "${health_file}" \
    | jq -e '.status == "ok" and .db == "ok" and .redis == "ok" and .doc_engine == "ok" and .storage == "ok"' >/dev/null; do
    if (( SECONDS >= deadline )); then
      return 1
    fi
    sleep 5
  done
  curl -fsS --max-time 10 "http://127.0.0.1:${RAGFLOW_PORT}/" >/dev/null
}

for command_name in awk cmp curl flock jq rsync sed sha256sum sort tar tee; do
  command -v "${command_name}" >/dev/null || die "Required upgrade command is missing: ${command_name}"
done
[[ $(uname -m) == "x86_64" ]] || die "Only x86_64 Linux is supported."
[[ -r /etc/os-release ]] || die "/etc/os-release is required."
. /etc/os-release
if [[ ${ID:-} == "rocky" && ${VERSION_ID:-} != 9 && ${VERSION_ID:-} != 9.* ]]; then
  die "Rocky Linux 9.x is required; found ${VERSION_ID:-unknown}."
fi
case ${ID:-} in
  rocky|ubuntu|debian) ;;
  *) die "Supported distributions: Rocky Linux 9.x, Ubuntu, and Debian; found ${ID:-unknown}." ;;
esac

SOURCE_MANIFEST=${SOURCE_ROOT}/DEPLOYMENT-SOURCE.env
[[ -r ${SOURCE_MANIFEST} ]] || die "DEPLOYMENT-SOURCE.env is missing from the extracted release."
RELEASE_VERSION=$(manifest_value "${SOURCE_MANIFEST}" RELEASE_VERSION)
PACKAGE_FORMAT=$(manifest_value "${SOURCE_MANIFEST}" PACKAGE_FORMAT)
[[ -n ${RELEASE_VERSION} && ${PACKAGE_FORMAT} == "tar.gz" ]] || die "The source manifest is incomplete or unsupported."
[[ ${RELEASE_VERSION} =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "Invalid release version: ${RELEASE_VERSION}"
[[ ${SOURCE_ROOT} != "${INSTALL_DIR}" ]] || die "INSTALL_DIR must differ from the extracted release directory."

sudo -v
[[ ${EUID} -eq 0 ]] || die "Run the upgrade as root, for example: sudo env ... bash upgrade.sh"
[[ -d ${INSTALL_DIR} ]] || die "Existing installation not found: ${INSTALL_DIR}"
[[ -r ${INSTALL_DIR}/docker/.env ]] || die "Existing protected Docker environment is missing: ${INSTALL_DIR}/docker/.env"
[[ -r ${INSTALL_DIR}/deployment/linux-pg/docker-compose.release.yml ]] || die "Existing Linux release overlay is missing."
RECOVERY_REQUIRED_FILE=${SECRETS_DIR}/upgrade-recovery-required.env

exec 9>"/run/lock/${PROJECT_NAME}.upgrade.lock"
flock -n 9 || die "Another ${PROJECT_NAME} upgrade is already running."
[[ ! -e ${RECOVERY_REQUIRED_FILE} ]] || die "Unresolved upgrade recovery: ${RECOVERY_REQUIRED_FILE}. Verify or restore data and release compatibility before clearing this marker; automatic retry is blocked."

DOCKER_VERSION=$(sudo docker version --format '{{.Server.Version}}')
COMPOSE_VERSION=$(sudo docker compose version --short)
COMPOSE_VERSION=${COMPOSE_VERSION#v}
version_at_least "${DOCKER_VERSION}" "${MIN_DOCKER_VERSION}" || die "Docker ${DOCKER_VERSION} is too old."
version_at_least "${COMPOSE_VERSION}" "${MIN_COMPOSE_VERSION}" || die "Docker Compose ${COMPOSE_VERSION} is too old."

CURRENT_ENV=${INSTALL_DIR}/docker/.env
RAGFLOW_PORT=${RAGFLOW_PORT:-$(env_value "${CURRENT_ENV}" SVR_WEB_HTTP_PORT 80)}
[[ ${RAGFLOW_PORT} =~ ^[0-9]+$ && ${RAGFLOW_PORT} -ge 1 && ${RAGFLOW_PORT} -le 65535 ]] || die "Invalid RAGFLOW_PORT: ${RAGFLOW_PORT}"
[[ $(env_value "${CURRENT_ENV}" DB_TYPE) == "postgres" ]] || die "Existing installation is not configured for PostgreSQL."
[[ $(env_value "${CURRENT_ENV}" DOC_ENGINE) == "elasticsearch" ]] || die "The approved upgrade procedure requires DOC_ENGINE=elasticsearch."
validate_compose "${INSTALL_DIR}" 0
if ! curl -fsS --max-time 10 "http://127.0.0.1:${RAGFLOW_PORT}/api/v1/system/healthz" \
  | jq -e '.status == "ok"' >/dev/null; then
  [[ ${ALLOW_UNHEALTHY} == "1" ]] || die "Current RAGFlow health is not OK; diagnose it or use ALLOW_UNHEALTHY=1 in an approved recovery change."
fi

FROM_VERSION=unknown
if [[ -r ${SECRETS_DIR}/deployed-source.env ]]; then
  FROM_VERSION=$(manifest_value "${SECRETS_DIR}/deployed-source.env" RELEASE_VERSION)
  FROM_VERSION=${FROM_VERSION:-unknown}
fi
INCOMING_CANDIDATE=${SOURCE_ROOT}/DEPLOYMENT-CANDIDATE.json
INSTALLED_CANDIDATE=${INSTALL_DIR}/DEPLOYMENT-CANDIDATE.json
INCOMING_SOURCE_ID=
INSTALLED_SOURCE_ID=
if [[ -e ${INCOMING_CANDIDATE} || -L ${INCOMING_CANDIDATE} ]]; then
  INCOMING_SOURCE_ID=$(candidate_source_id "${INCOMING_CANDIDATE}" "${RELEASE_VERSION}")
fi
if [[ -e ${INSTALLED_CANDIDATE} || -L ${INSTALLED_CANDIDATE} ]]; then
  INSTALLED_SOURCE_ID=$(candidate_source_id "${INSTALLED_CANDIDATE}" "${FROM_VERSION}")
  [[ -n ${INCOMING_SOURCE_ID} ]] || die "Refusing candidate identity downgrade to an unverified legacy package."
fi
if [[ ${FROM_VERSION} =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  from_plain=${FROM_VERSION#v}
  to_plain=${RELEASE_VERSION#v}
  if [[ ${from_plain} == "${to_plain}" ]]; then
    if [[ -n ${INCOMING_SOURCE_ID} || -n ${INSTALLED_SOURCE_ID} ]]; then
      [[ -n ${INCOMING_SOURCE_ID} && ${INCOMING_SOURCE_ID} == "${INSTALLED_SOURCE_ID}" ]] \
        || die "Same version has missing or different source identity; no-op is prohibited."
    else
      echo "WARNING: legacy same-version no-op has unverified source identity." >&2
    fi
    curl -fsS --max-time 10 "http://127.0.0.1:${RAGFLOW_PORT}/api/v1/system/healthz" \
      | jq -e '.status == "ok" and .db == "ok" and .redis == "ok" and .doc_engine == "ok" and .storage == "ok"' >/dev/null \
      || die "Release ${RELEASE_VERSION} is recorded as installed, but full health verification failed."
    verify_installed_audit_table "${INSTALL_DIR}" \
      || die "Release ${RELEASE_VERSION} is recorded as installed, but system_audit_event is missing."
    echo "RAGFlow ${RELEASE_VERSION} is already installed and healthy; no upgrade is required."
    exit 0
  fi
  if [[ ${ALLOW_DOWNGRADE} != "1" &&
        $(printf '%s\n%s\n' "${from_plain}" "${to_plain}" | sort -V | tail -n 1) != "${to_plain}" ]]; then
    die "Target ${RELEASE_VERSION} is older than installed ${FROM_VERSION}; set ALLOW_DOWNGRADE=1 only for an approved rollback."
  fi
fi

STAGE_DIR="${INSTALL_DIR}.stage-${RELEASE_VERSION//[^0-9A-Za-z._-]/-}-$$"
[[ ! -e ${STAGE_DIR} ]] || die "Upgrade staging directory already exists: ${STAGE_DIR}"
sudo install -d -m 0755 "${STAGE_DIR}"
cleanup_stage() {
  local expected_prefix
  expected_prefix="$(basename -- "${INSTALL_DIR}").stage-"
  if [[ -n ${STAGE_DIR:-} && -d ${STAGE_DIR} &&
        $(dirname -- "${STAGE_DIR}") == "$(dirname -- "${INSTALL_DIR}")" &&
        $(basename -- "${STAGE_DIR}") == "${expected_prefix}"* ]]; then
    sudo rm -rf -- "${STAGE_DIR}"
  fi
}
trap cleanup_stage EXIT

sudo rsync -a --delete \
  --exclude .git --exclude .venv --exclude .codex_tmp --exclude .playwright-cli \
  --exclude node_modules --exclude output --exclude docker/ragflow-logs \
  "${SOURCE_ROOT}/" "${STAGE_DIR}/"
sudo install -m 0600 "${CURRENT_ENV}" "${STAGE_DIR}/docker/.env"
if [[ -r ${INSTALL_DIR}/docker/.env.local ]]; then
  sudo install -m 0600 "${INSTALL_DIR}/docker/.env.local" "${STAGE_DIR}/docker/.env.local"
fi
sudo test -s "${STAGE_DIR}/web/dist/index.html" || die "Prebuilt frontend is missing from the release."

if [[ -n ${IMAGE_ENV_FILE} ]]; then
  [[ -r ${IMAGE_ENV_FILE} ]] || die "Image environment file is not readable: ${IMAGE_ENV_FILE}"
  while IFS='=' read -r image_key image_value; do
    [[ -z ${image_key} || ${image_key} == \#* ]] && continue
    case ${image_key} in
      POSTGRES_IMAGE|RAGFLOW_IMAGE|VALKEY_IMAGE|ELASTICSEARCH_IMAGE|PLANTUML_IMAGE|MINIO_IMAGE|T_ONE_ASR_IMAGE|OTEL_COLLECTOR_IMAGE|TEMPO_IMAGE|LOKI_IMAGE|PROMETHEUS_IMAGE|GRAFANA_IMAGE|SANDBOX_EXECUTOR_MANAGER_IMAGE|SANDBOX_BASE_NODEJS_IMAGE|SANDBOX_BASE_PYTHON_IMAGE)
        [[ -n ${image_value} && ! ${image_value} =~ [[:space:]] ]] || die "Invalid image reference for ${image_key}."
        sudo sed -i "/^${image_key}=/d" "${STAGE_DIR}/docker/.env"
        printf '%s=%s\n' "${image_key}" "${image_value}" | sudo tee -a "${STAGE_DIR}/docker/.env" >/dev/null
        ;;
      *) die "Unsupported key in image environment file: ${image_key}" ;;
    esac
  done < "${IMAGE_ENV_FILE}"
fi
sudo chown -R root:root "${STAGE_DIR}"
sudo find "${STAGE_DIR}" -type f -name '*.sh' -exec chmod 0755 {} +
if ! grep -q '^GRAFANA_ADMIN_PASSWORD=' "${STAGE_DIR}/docker/.env"; then
  printf 'GRAFANA_ADMIN_PASSWORD=%s\n' "$(openssl rand -hex 32)" | sudo tee -a "${STAGE_DIR}/docker/.env" >/dev/null
fi
if grep -q '^SANDBOX_ENABLED=' "${STAGE_DIR}/docker/.env"; then
  sudo sed -i 's/^SANDBOX_ENABLED=.*/SANDBOX_ENABLED=1/' "${STAGE_DIR}/docker/.env"
else
  printf 'SANDBOX_ENABLED=1\n' | sudo tee -a "${STAGE_DIR}/docker/.env" >/dev/null
fi
validate_compose "${STAGE_DIR}" 1
verify_runsc_runtime
if [[ ${OFFLINE_INSTALL} == "1" ]]; then
  verify_offline_images "${STAGE_DIR}"
fi

# Resolve backup dependencies while the currently deployed application is still running.
CURRENT_RAGFLOW_CONTAINER=$(compose "${INSTALL_DIR}" ps -q ragflow-cpu)
[[ -n ${CURRENT_RAGFLOW_CONTAINER} ]] || die "RAGFlow container is not running before upgrade."
RAGFLOW_BACKUP_IMAGE=$(sudo docker inspect "${CURRENT_RAGFLOW_CONTAINER}" --format '{{.Image}}')
[[ -n ${RAGFLOW_BACKUP_IMAGE} ]] || die "Cannot resolve the current RAGFlow image for MinIO backup."
sudo docker image inspect "${RAGFLOW_BACKUP_IMAGE}" >/dev/null
POSTGRES_CONTAINER=$(compose "${INSTALL_DIR}" ps -q postgres)
MINIO_CONTAINER=$(compose "${INSTALL_DIR}" ps -q minio)
ES_CONTAINER=$(compose "${INSTALL_DIR}" ps -q es01)
[[ -n ${POSTGRES_CONTAINER} ]] || die "PostgreSQL container is not running."
[[ -n ${MINIO_CONTAINER} ]] || die "MinIO container is not running."
[[ -n ${ES_CONTAINER} ]] || die "Elasticsearch container is not running."
POSTGRES_DBNAME=$(env_value "${CURRENT_ENV}" POSTGRES_DBNAME rag_flow)
POSTGRES_USER=$(env_value "${CURRENT_ENV}" POSTGRES_USER rag_flow)

if [[ ${CHECK_ONLY} == "1" ]]; then
  echo "Upgrade preflight passed: ${FROM_VERSION} -> ${RELEASE_VERSION}"
  echo "Existing installation: ${INSTALL_DIR}"
  echo "Backup root: ${BACKUP_ROOT}"
  exit 0
elif [[ ${REGISTRY_INSTALL} == "1" ]]; then
  compose "${STAGE_DIR}" pull
elif [[ ${OFFLINE_INSTALL} != "1" ]]; then
  compose "${STAGE_DIR}" pull --ignore-buildable --policy missing
  compose "${STAGE_DIR}" build
fi

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP_DIR=${BACKUP_ROOT}/${STAMP}-${FROM_VERSION//[^0-9A-Za-z._-]/-}-to-${RELEASE_VERSION//[^0-9A-Za-z._-]/-}
PREVIOUS_DIR=${INSTALL_DIR}.previous-${STAMP}
FAILED_DIR=${INSTALL_DIR}.failed-${STAMP}
sudo install -d -m 0700 "${BACKUP_DIR}/config"

APP_STOPPED=0
SWITCHED=0
DATA_MAY_HAVE_CHANGED=0
upgrade_error() {
  local status=$?
  trap - ERR
  set +e
  if [[ ${DATA_MAY_HAVE_CHANGED} == "1" ]]; then
    echo "Upgrade failed after the data-change boundary. Automatic code rollback is prohibited." >&2
    compose "${INSTALL_DIR}" stop ragflow-cpu || echo "Could not stop the failed application; isolate it before recovery." >&2
    echo "Recovery required: ${RECOVERY_REQUIRED_FILE}. Previous release and backups are preserved." >&2
  elif [[ ${SWITCHED} == "1" && -d ${PREVIOUS_DIR} ]]; then
    compose "${INSTALL_DIR}" stop ragflow-cpu >/dev/null 2>&1 || true
    if [[ ! -e ${INSTALL_DIR} ]] || sudo mv -- "${INSTALL_DIR}" "${FAILED_DIR}"; then
      if sudo mv -- "${PREVIOUS_DIR}" "${INSTALL_DIR}"; then
        compose "${INSTALL_DIR}" start ragflow-cpu || true
      fi
    fi
  elif [[ ${APP_STOPPED} == "1" ]]; then
    compose "${INSTALL_DIR}" start ragflow-cpu || true
  fi
  if [[ -n ${BACKUP_DIR:-} && -d ${BACKUP_DIR} ]]; then
    printf 'FAILED_AT_UTC=%s\nFAILED_RELEASE=%s\nDATA_MAY_HAVE_CHANGED=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${RELEASE_VERSION}" "${DATA_MAY_HAVE_CHANGED}" \
      | sudo tee "${BACKUP_DIR}/UPGRADE-FAILED.env" >/dev/null
  fi
  echo "Data restoration is not automatic or verified by this script. Follow the recovery runbook before restarting an older release." >&2
  exit "${status}"
}
trap upgrade_error ERR

upgrade_fail() {
  echo "$*" >&2
  return 1
}

# A failing stop may already have stopped a container; recover the old app too.
APP_STOPPED=1
compose "${INSTALL_DIR}" stop ragflow-cpu

sudo install -m 0600 "${CURRENT_ENV}" "${BACKUP_DIR}/config/docker.env"
[[ ! -r ${INSTALL_DIR}/docker/.env.local ]] || sudo install -m 0600 "${INSTALL_DIR}/docker/.env.local" "${BACKUP_DIR}/config/docker.env.local"
[[ ! -r ${SECRETS_DIR}/admin.env ]] || sudo install -m 0600 "${SECRETS_DIR}/admin.env" "${BACKUP_DIR}/config/admin.env"
[[ ! -r ${SECRETS_DIR}/deployed-source.env ]] || sudo install -m 0644 "${SECRETS_DIR}/deployed-source.env" "${BACKUP_DIR}/config/deployed-source.env"

sudo docker exec "${POSTGRES_CONTAINER}" pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DBNAME}" -Fc \
  | sudo tee "${BACKUP_DIR}/postgres.dump" >/dev/null
sudo docker exec -i "${POSTGRES_CONTAINER}" pg_restore --list < "${BACKUP_DIR}/postgres.dump" >/dev/null
sudo docker exec "${POSTGRES_CONTAINER}" pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DBNAME}" --schema-only --no-owner \
  | sudo tee "${BACKUP_DIR}/postgres-schema-before.sql" >/dev/null

if [[ ${BACKUP_MINIO} == "1" ]]; then
  sudo docker run --rm --network none \
    --volumes-from "${MINIO_CONTAINER}" \
    -v "${BACKUP_DIR}:/backup" \
    --entrypoint /bin/tar \
    "${RAGFLOW_BACKUP_IMAGE}" -czf /backup/minio-data.tar.gz -C /data .
  sudo tar -tzf "${BACKUP_DIR}/minio-data.tar.gz" >/dev/null
else
  printf 'MINIO_BACKUP_SKIPPED=1\n' | sudo tee "${BACKUP_DIR}/MINIO-BACKUP-SKIPPED.env" >/dev/null
fi

sudo docker exec "${ES_CONTAINER}" sh -c \
  'curl -fsS --max-time 15 -u "elastic:${ELASTIC_PASSWORD}" "http://127.0.0.1:9200/_cat/indices?format=json&h=index,health,status,docs.count"' \
  | sudo tee "${BACKUP_DIR}/elasticsearch-indices-before.json" >/dev/null

sudo tee "${BACKUP_DIR}/UPGRADE.env" >/dev/null <<EOF
FROM_VERSION=${FROM_VERSION}
TO_VERSION=${RELEASE_VERSION}
INSTALL_DIR=${INSTALL_DIR}
PREVIOUS_DIR=${PREVIOUS_DIR}
PROJECT_NAME=${PROJECT_NAME}
BACKUP_MINIO=${BACKUP_MINIO}
CREATED_AT_UTC=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ELASTICSEARCH_RESTORE_MODE=reindex_from_postgres_and_minio
EOF
(cd "${BACKUP_DIR}" && sudo find . -type f ! -name SHA256SUMS -print0 | sort -z | sudo xargs -0 sha256sum | sudo tee SHA256SUMS >/dev/null)
(cd "${BACKUP_DIR}" && sudo sha256sum -c SHA256SUMS)

SWITCHED=1
sudo mv -- "${INSTALL_DIR}" "${PREVIOUS_DIR}"
sudo mv -- "${STAGE_DIR}" "${INSTALL_DIR}"
STAGE_DIR=

# The entrypoint may migrate data as soon as any new application process starts.
# Persist the stop-point before starting it, including for interrupted upgrades.
sudo install -d -m 0700 "${SECRETS_DIR}"
printf 'TARGET_RELEASE=%s\nBACKUP_DIR=%s\nPREVIOUS_DIR=%s\n' "${RELEASE_VERSION}" "${BACKUP_DIR}" "${PREVIOUS_DIR}" \
  | sudo tee "${RECOVERY_REQUIRED_FILE}" >/dev/null
sudo chmod 0600 "${RECOVERY_REQUIRED_FILE}"
DATA_MAY_HAVE_CHANGED=1
compose "${INSTALL_DIR}" up -d --no-build --pull never
wait_for_health "${BACKUP_DIR}/health-after.json"
curl -fsS --max-time 10 http://127.0.0.1:3001/api/health | jq -e '.database == "ok"' >/dev/null
curl -fsS --max-time 10 http://127.0.0.1:9090/-/ready >/dev/null
ASR_CONTAINER=$(compose "${INSTALL_DIR}" ps -q t-one-asr)
SANDBOX_CONTAINER=$(compose "${INSTALL_DIR}" ps -q sandbox-executor-manager)
[[ -n ${ASR_CONTAINER} && -n ${SANDBOX_CONTAINER} ]] || upgrade_fail "ASR or sandbox manager is not running after upgrade."
sudo docker exec "${ASR_CONTAINER}" python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:9011/health/ready', timeout=10)"
sudo docker exec "${SANDBOX_CONTAINER}" curl -fsS http://127.0.0.1:9385/healthz >/dev/null
SANDBOX_LOGS=$(sudo docker logs "${SANDBOX_CONTAINER}" 2>&1)
grep -Eq 'Container pool initialization complete: [1-9][0-9]*/[1-9][0-9]* available' <<<"${SANDBOX_LOGS}"

RAGFLOW_CONTAINER=$(compose "${INSTALL_DIR}" ps -q ragflow-cpu)
POSTGRES_CONTAINER=$(compose "${INSTALL_DIR}" ps -q postgres)
[[ -n ${RAGFLOW_CONTAINER} ]] || upgrade_fail "RAGFlow container is not running after upgrade."
[[ -n ${POSTGRES_CONTAINER} ]] || upgrade_fail "PostgreSQL container is not running after upgrade."
sudo docker exec "${RAGFLOW_CONTAINER}" printenv DB_TYPE | grep -qx postgres
sudo docker exec -i "${RAGFLOW_CONTAINER}" python - < "${INSTALL_DIR}/deployment/linux-pg/seed_asr.py"
sudo docker exec "${POSTGRES_CONTAINER}" psql -U "${POSTGRES_USER}" -d "${POSTGRES_DBNAME}" -Atc \
  "select to_regclass('public.system_audit_event') is not null" | grep -qx t
sudo docker exec "${POSTGRES_CONTAINER}" pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DBNAME}" --schema-only --no-owner \
  | sudo tee "${BACKUP_DIR}/postgres-schema-after.sql" >/dev/null

if [[ -n ${INCOMING_SOURCE_ID} ]]; then
  cmp -s -- "${INCOMING_CANDIDATE}" "${INSTALLED_CANDIDATE}" \
    || upgrade_fail "Installed candidate manifest bytes differ from the incoming package."
  DELIVERED_SOURCE_ID=$(candidate_source_id "${INSTALLED_CANDIDATE}" "${RELEASE_VERSION}")
  [[ ${DELIVERED_SOURCE_ID} == "${INCOMING_SOURCE_ID}" ]] \
    || upgrade_fail "Installed candidate identity changed during upgrade."
fi

sudo install -d -m 0700 "${SECRETS_DIR}"
sudo install -m 0644 "${SOURCE_MANIFEST}" "${SECRETS_DIR}/deployed-source.env"
printf 'UPGRADED_AT_UTC=%s\nPREVIOUS_VERSION=%s\nBACKUP_DIR=%s\nPREVIOUS_DIR=%s\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${FROM_VERSION}" "${BACKUP_DIR}" "${PREVIOUS_DIR}" \
  | sudo tee -a "${SECRETS_DIR}/deployed-source.env" >/dev/null
printf 'COMPLETED_AT_UTC=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" | sudo tee "${BACKUP_DIR}/UPGRADE-COMPLETED.env" >/dev/null
(cd "${BACKUP_DIR}" && sudo find . -type f ! -name SHA256SUMS -print0 | sort -z | sudo xargs -0 sha256sum | sudo tee SHA256SUMS >/dev/null)
(cd "${BACKUP_DIR}" && sudo sha256sum -c SHA256SUMS)

trap - ERR
sudo rm -- "${RECOVERY_REQUIRED_FILE}"
SWITCHED=0
APP_STOPPED=0
compose "${INSTALL_DIR}" ps
echo
echo "RAGFlow upgrade completed: ${FROM_VERSION} -> ${RELEASE_VERSION}"
echo "Backup: ${BACKUP_DIR}"
echo "Previous application release: ${PREVIOUS_DIR}"
echo "Elasticsearch rollback mode: rebuild indexes from PostgreSQL and MinIO."
echo "Do not remove the backup or previous release until the acceptance window is closed."
