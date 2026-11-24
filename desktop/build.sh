#!/usr/bin/env bash
#
# Builds the desktop app in one step: the engine, the bundle's staged inputs and
# the platform package.
#
#   bash desktop/build.sh                  # engine + .app + .dmg
#   bash desktop/build.sh --bundles app    # anything unrecognised goes to
#                                          # `cargo tauri build`
#   bash desktop/build.sh --no-engine      # reuse ../bin/ragflow-desktop as is
#   bash desktop/build.sh --dev            # tauri dev instead of build
#   bash desktop/build.sh --icons          # only regenerate icons from
#                                          # app-icon.png
#
# The engine is rebuilt by default: packaging a stale binary is a silent defect.
# Go rebuilds are incremental, and the C++ static library is only compiled when
# it is missing.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/.." && pwd)"

usage() {
  sed -n '3,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

engine=on
mode=build
extra=()
for arg in "$@"; do
  case "$arg" in
    --no-engine) engine=off ;;
    --dev) mode=dev ;;
    --icons) mode=icons ;;
    --help|-h) usage; exit 0 ;;
    *) extra+=("$arg") ;;
  esac
done

if ! cargo tauri --version >/dev/null 2>&1; then
  cat >&2 <<'EOF'
tauri-cli is missing. Install it once:
    cargo install tauri-cli --version "^2" --locked
EOF
  exit 1
fi

# bundle.icon names the generated set under src-tauri/icons, not app-icon.png:
# replacing the source image alone changes nothing until this runs. `tauri icon`
# also emits mobile and Windows-Store sets this app never uses; they are dropped
# so the icons directory stays the five files the bundle declares.
regen_icons() {
  local force="${1:-}"
  if [ -z "$force" ] && [ ! "$here/app-icon.png" -nt "$here/src-tauri/icons/icon.icns" ]; then
    echo "icons: up to date"
    return 0
  fi
  echo "=== icons ==="
  cargo tauri icon -o "$here/src-tauri/icons" "$here/app-icon.png"
  rm -rf "$here/src-tauri/icons/android" "$here/src-tauri/icons/ios"
  rm -f "$here"/src-tauri/icons/Square*.png "$here"/src-tauri/icons/StoreLogo.png \
    "$here"/src-tauri/icons/64x64.png
}

cd "$here"
if [ "$mode" = icons ]; then
  regen_icons force
  exit 0
fi

if [ "$engine" = on ]; then
  # librag_tokenizer_c_api.a is what the Go link consumes; without it
  # `build.sh --desktop` fails at link time rather than building it.
  cpp_lib="$repo_root/internal/binding/cpp/cmake-build-release/librag_tokenizer_c_api.a"
  if [ ! -f "$cpp_lib" ]; then
    echo "=== C++ static library missing; building it first (one-off) ==="
    bash "$repo_root/build.sh" --cpp
  fi
  echo "=== engine ==="
  bash "$repo_root/build.sh" --desktop
fi

echo "=== staging bundle inputs ==="
regen_icons
bash "$here/stage-sidecar.sh"
bash "$here/stage-resources.sh"

if [ "$mode" = dev ]; then
  # dev's resource_dir() is target/debug, which holds no bundle: point the engine
  # at the same tree the packaged app would carry.
  export RAGFLOW_SIDECAR="${RAGFLOW_SIDECAR:-$repo_root/bin/ragflow-desktop}"
  export RAGFLOW_DICT_PATH="${RAGFLOW_DICT_PATH:-$(cd "$repo_root/bin/resource" && pwd)}"
  exec cargo tauri dev "${extra[@]+"${extra[@]}"}"
fi

cargo tauri build "${extra[@]+"${extra[@]}"}"

bundle="$here/src-tauri/target/release/bundle"
for artifact in "$bundle"/macos/*.app "$bundle"/dmg/*.dmg "$bundle"/nsis/*.exe "$bundle"/deb/*.deb; do
  [ -e "$artifact" ] && echo "  $(du -sh "$artifact" | cut -f1)	$artifact"
done
exit 0
