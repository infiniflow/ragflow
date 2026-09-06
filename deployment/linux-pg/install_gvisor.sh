#!/usr/bin/env bash
set -Eeuo pipefail

GVISOR_BUNDLE_DIR=${GVISOR_BUNDLE_DIR:?GVISOR_BUNDLE_DIR is required}
for path in runsc containerd-shim-runsc-v1 gvisor-bin/checkpointgofer gvisor-bin/gvisor_sentry; do
  [[ -r ${GVISOR_BUNDLE_DIR}/${path} ]] || { echo "gVisor bundle is missing ${path}" >&2; exit 1; }
done

runtime_path=$(sudo docker info --format '{{json .Runtimes}}' | jq -r '.runsc.path // empty')
sudo install -d -m 0755 /usr/local/bin/gvisor-bin
sudo install -m 0755 "${GVISOR_BUNDLE_DIR}/runsc" /usr/local/bin/runsc
sudo install -m 0755 "${GVISOR_BUNDLE_DIR}/containerd-shim-runsc-v1" /usr/local/bin/containerd-shim-runsc-v1
for sidecar in "${GVISOR_BUNDLE_DIR}"/gvisor-bin/*; do
  sudo install -m 0755 "${sidecar}" "/usr/local/bin/gvisor-bin/$(basename "${sidecar}")"
done

sudo /usr/local/bin/runsc install \
  -config_file=/etc/docker/daemon.json \
  -download-sidecars=NEVER \
  -require-sidecars=ALWAYS
# Existing runtime paths need no daemon reload. A changed registration requires
# a restart: SIGHUP reloads can invalidate Docker's host-gateway mappings.
if [[ ${runtime_path} != "/usr/local/bin/runsc" ]]; then
  sudo systemctl restart docker
fi
deadline=$((SECONDS + 60))
while true; do
  docker_runtimes=$(sudo docker info --format '{{json .Runtimes}}' 2>/dev/null || true)
  if grep -q '"runsc"' <<<"${docker_runtimes}"; then
    break
  fi
  (( SECONDS < deadline )) || { echo 'Docker did not register runsc within 60 seconds.' >&2; exit 1; }
  sleep 2
done
sudo /usr/local/bin/runsc --version
