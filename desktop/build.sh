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

# appimagetool needs the AppImage type-2 runtime and fetches it from GitHub every
# time it packages. When that fetch cannot complete — an offline build host, or a
# network that drops the release asset — the appimage plugin exits non-zero and
# the whole bundle surfaces as "failed to run linuxdeploy", which names neither
# the missing file nor the network. Keep the runtime in the build cache and hand
# it to the plugin through LDAI_RUNTIME_FILE, so packaging never needs the
# download: it is taken once from the release, or lifted off the linuxdeploy
# AppImage the Tauri CLI already caches.
appimage_payload_offset() {
  local src="$1" off block_size compression
  # The magic alone decides nothing: 'hsqs' also occurs inside the runtime's own
  # machine code. Every hit is confirmed against the squashfs superblock fields
  # that follow it (block size, compression id).
  for off in $(grep -abo 'hsqs' "$src" | cut -d: -f1); do
    block_size="$(od -An -tu4 -j "$((off + 12))" -N 4 "$src" 2>/dev/null | tr -d ' ')"
    case "$block_size" in
      4096|8192|16384|32768|65536|131072|262144|524288|1048576) ;;
      *) continue ;;
    esac
    compression="$(od -An -tu2 -j "$((off + 20))" -N 2 "$src" 2>/dev/null | tr -d ' ')"
    case "$compression" in
      1|2|3|4|5|6) printf '%s\n' "$off"; return 0 ;;
    esac
  done
  return 1
}

appimage_runtime() {
  local arch cache src off
  case "$(uname -m)" in
    x86_64|amd64) arch=x86_64 ;;
    aarch64|arm64) arch=aarch64 ;;
    *) echo "unsupported architecture for AppImage: $(uname -m)" >&2; return 1 ;;
  esac
  cache="${XDG_CACHE_HOME:-$HOME/.cache}/ragflow-desktop/runtime-$arch"

  if [ ! -f "$cache" ]; then
    echo "=== appimage runtime ===" >&2
    mkdir -p "$(dirname "$cache")"
    if curl -fsSL --retry 2 --connect-timeout 10 --max-time 60 -o "$cache.part" \
        "https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-$arch"; then
      mv "$cache.part" "$cache"
      echo "  fetched runtime-$arch" >&2
    else
      rm -f "$cache.part"
      src="$(ls -1 "${XDG_CACHE_HOME:-$HOME/.cache}"/tauri/linuxdeploy-*-"$arch".AppImage 2>/dev/null | head -1)"
      if [ -z "$src" ]; then
        echo "  no network for the runtime and no linuxdeploy AppImage to take it from." >&2
        echo "  Download runtime-$arch from https://github.com/AppImage/type2-runtime/releases" >&2
        echo "  and save it as $cache." >&2
        return 1
      fi
      if ! off="$(appimage_payload_offset "$src")"; then
        echo "  could not locate the payload in $src" >&2
        return 1
      fi
      head -c "$off" "$src" > "$cache"
      echo "  took the runtime from $src (no network)" >&2
    fi
    if [ "$(head -c 4 "$cache" | od -An -tx1 | tr -d ' \n')" != "7f454c46" ]; then
      echo "  $cache is not an ELF runtime; removing it." >&2
      rm -f "$cache"
      return 1
    fi
  fi
  printf '%s\n' "$cache"
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

if [ "$(uname -s)" = "Linux" ] && [ -z "${LDAI_RUNTIME_FILE:-}" ]; then
  if runtime="$(appimage_runtime)"; then
    export LDAI_RUNTIME_FILE="$runtime"
  else
    echo "warning: packaging the AppImage will fetch its runtime from GitHub instead" >&2
  fi
fi

cargo tauri build "${extra[@]+"${extra[@]}"}"

bundle="$here/src-tauri/target/release/bundle"
for artifact in "$bundle"/macos/*.app "$bundle"/dmg/*.dmg "$bundle"/nsis/*.exe \
  "$bundle"/deb/*.deb "$bundle"/appimage/*.AppImage; do
  [ -e "$artifact" ] && echo "  $(du -sh "$artifact" | cut -f1)	$artifact"
done
exit 0
