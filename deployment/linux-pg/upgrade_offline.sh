#!/usr/bin/env bash
set -Eeuo pipefail

CHECK_ONLY=0
if [[ ${1:-} == "--check" ]]; then
  CHECK_ONLY=1
  shift
fi
if [[ $# -ne 0 ]]; then
  echo "Usage: $0 [--check]" >&2
  exit 2
fi

PACKAGE_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PACKAGE_MANIFEST=${PACKAGE_ROOT}/OFFLINE-PACKAGE.env
CHECKSUMS=${PACKAGE_ROOT}/SHA256SUMS
PAYLOAD_DIR=${PACKAGE_ROOT}/payload

[[ -r ${PACKAGE_MANIFEST} && -r ${CHECKSUMS} ]] || {
  echo "Offline package manifest or checksums are missing." >&2
  exit 1
}

manifest_value() {
  local key=$1
  awk -F= -v key="${key}" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "${PACKAGE_MANIFEST}"
}

RELEASE_VERSION=$(manifest_value RELEASE_VERSION)
PACKAGE_MODE=$(manifest_value PACKAGE_MODE)
TARGET_OS=$(manifest_value TARGET_OS)
TARGET_VERSION=$(manifest_value TARGET_VERSION)
TARGET_ARCH=$(manifest_value TARGET_ARCH)
DOCKER_DNF_REPO=$(manifest_value DOCKER_DNF_REPO)
SOURCE_ARCHIVE=$(manifest_value SOURCE_ARCHIVE)
FRONTEND_ARCHIVE=$(manifest_value FRONTEND_ARCHIVE)
DOCKER_IMAGES_ARCHIVE=$(manifest_value DOCKER_IMAGES_ARCHIVE)
GVISOR_BUNDLE=$(manifest_value GVISOR_BUNDLE)

if [[ -z ${RELEASE_VERSION} || ${PACKAGE_MODE} != "offline" || ${TARGET_OS} != "rocky" ||
      ${TARGET_VERSION} != "9" || ${TARGET_ARCH} != "amd64" || -z ${DOCKER_DNF_REPO} ]]; then
  echo "Unsupported or incomplete offline package manifest." >&2
  exit 1
fi
for payload_name in "${SOURCE_ARCHIVE}" "${FRONTEND_ARCHIVE}" "${DOCKER_IMAGES_ARCHIVE}" docker-images.txt; do
  if [[ -z ${payload_name} || ${payload_name} == */* || ! -r ${PAYLOAD_DIR}/${payload_name} ]]; then
    echo "Required offline payload is missing or invalid: ${payload_name}" >&2
    exit 1
  fi
done
[[ ${GVISOR_BUNDLE} == "gvisor" && -d ${PAYLOAD_DIR}/${GVISOR_BUNDLE} ]] || { echo "Pinned gVisor bundle is missing." >&2; exit 1; }

[[ $(uname -m) == "x86_64" ]] || { echo "The offline package supports x86_64 only." >&2; exit 1; }
. /etc/os-release
if [[ ${ID:-} != "rocky" || (${VERSION_ID:-} != 9 && ${VERSION_ID:-} != 9.*) ]]; then
  echo "The offline package requires Rocky Linux 9.x; found ${ID:-unknown} ${VERSION_ID:-unknown}." >&2
  exit 1
fi
(cd "${PACKAGE_ROOT}" && sha256sum -c SHA256SUMS)

SOURCE_DIR=${SOURCE_DIR:-/srv/ragflow-linux-pg-${RELEASE_VERSION}-upgrade}
INSTALL_DIR=${INSTALL_DIR:-/opt/ragflow-pg}
PROJECT_NAME=${PROJECT_NAME:-ragflow-pg}
RAGFLOW_PORT=${RAGFLOW_PORT:-}
BACKUP_ROOT=${BACKUP_ROOT:-/var/backups/ragflow-pg}
BACKUP_MINIO=${BACKUP_MINIO:-1}
SECRETS_DIR=${SECRETS_DIR:-/etc/ragflow-pg}
ALLOW_DOWNGRADE=${ALLOW_DOWNGRADE:-0}
ALLOW_UNHEALTHY=${ALLOW_UNHEALTHY:-0}

sudo -v
if sudo test -e "${SOURCE_DIR}" && [[ -n $(sudo find "${SOURCE_DIR}" -mindepth 1 -maxdepth 1 -print -quit) ]]; then
  echo "SOURCE_DIR is not empty: ${SOURCE_DIR}. Select a fresh SOURCE_DIR for this package; existing files were not changed." >&2
  exit 1
fi
sudo install -d -m 0755 "${SOURCE_DIR}"
sudo tar -xzf "${PAYLOAD_DIR}/${SOURCE_ARCHIVE}" -C "${SOURCE_DIR}"
sudo install -d -m 0755 "${SOURCE_DIR}/web"
sudo tar -xzf "${PAYLOAD_DIR}/${FRONTEND_ARCHIVE}" -C "${SOURCE_DIR}/web"
sudo find "${SOURCE_DIR}" -type f -name '*.sh' -exec chmod 0755 {} +
sudo test -s "${SOURCE_DIR}/web/dist/index.html"

run_preflight() {
  sudo env \
    INSTALL_DIR="${INSTALL_DIR}" PROJECT_NAME="${PROJECT_NAME}" RAGFLOW_PORT="${RAGFLOW_PORT}" \
    SECRETS_DIR="${SECRETS_DIR}" BACKUP_ROOT="${BACKUP_ROOT}" BACKUP_MINIO="${BACKUP_MINIO}" \
    ALLOW_DOWNGRADE="${ALLOW_DOWNGRADE}" ALLOW_UNHEALTHY="${ALLOW_UNHEALTHY}" \
    bash "${SOURCE_DIR}/deployment/linux-pg/upgrade.sh" --check
}

run_preflight
if [[ ${CHECK_ONLY} == "1" ]]; then
  exit 0
fi

INSTALLED_VERSION=$(sudo awk -F= '$1 == "RELEASE_VERSION" { print $2; exit }' \
  "${SECRETS_DIR}/deployed-source.env" 2>/dev/null || true)
if [[ ${INSTALLED_VERSION#v} == "${RELEASE_VERSION#v}" ]]; then
  echo "Offline package ${RELEASE_VERSION} is already installed; Docker images were not reloaded."
  exit 0
fi

ENABLED_DNF_REPOS=$(sudo dnf -q repolist --enabled | awk 'NR > 1 { print $1 }')
grep -Fxq "${DOCKER_DNF_REPO}" <<<"${ENABLED_DNF_REPOS}" || {
  echo "Required Docker DNF repository is not enabled: ${DOCKER_DNF_REPO}" >&2
  exit 1
}
sudo dnf install -y --enablerepo="${DOCKER_DNF_REPO}" \
  ca-certificates curl jq openssl rsync tar util-linux \
  docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
sudo env GVISOR_BUNDLE_DIR="${PAYLOAD_DIR}/${GVISOR_BUNDLE}" bash "${SOURCE_DIR}/deployment/linux-pg/install_gvisor.sh"
sudo docker load -i "${PAYLOAD_DIR}/${DOCKER_IMAGES_ARCHIVE}"
while IFS= read -r image_name; do
  [[ -z ${image_name} ]] && continue
  sudo docker image inspect "${image_name}" >/dev/null
done < "${PAYLOAD_DIR}/docker-images.txt"

sudo env \
  OFFLINE_INSTALL=1 \
  INSTALL_DIR="${INSTALL_DIR}" PROJECT_NAME="${PROJECT_NAME}" RAGFLOW_PORT="${RAGFLOW_PORT}" \
  SECRETS_DIR="${SECRETS_DIR}" BACKUP_ROOT="${BACKUP_ROOT}" BACKUP_MINIO="${BACKUP_MINIO}" \
  ALLOW_DOWNGRADE="${ALLOW_DOWNGRADE}" ALLOW_UNHEALTHY="${ALLOW_UNHEALTHY}" \
  bash "${SOURCE_DIR}/deployment/linux-pg/upgrade.sh"
