#!/usr/bin/env bash
#
# Stages the desktop engine where the shell's bundle config expects it.
#
# The shell is only a window and a supervisor; every product feature lives in
# the Go binary. `bundle.resources` names it as src-tauri/binaries/ragflow-desktop
# because Tauri resolves that path from the config file, so the binary has to sit
# inside the tree before the bundle step. It is a build product: it is ignored by
# git and rewritten on every build.
#
#   RAGFLOW_DESKTOP_BINARY   copy this file instead of ../bin/ragflow-desktop

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Windows keeps the executable suffix: the shell looks the sidecar up by name
# (src-tauri/src/main.rs tries binaries/ragflow-desktop.exe first there), and the
# DLLs the engine links against have to sit beside it — LoadLibrary resolves them
# against the executable's directory.
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) exe=".exe" ;;
  *) exe="" ;;
esac
src="${RAGFLOW_DESKTOP_BINARY:-$here/../bin/ragflow-desktop$exe}"
dest="$here/src-tauri/binaries/ragflow-desktop$exe"

if [ ! -f "$src" ]; then
  cat >&2 <<EOF
stage-sidecar: the engine binary is missing: $src

Build it first, then retry:
    bash build.sh --cpp && bash build.sh --desktop

Or point RAGFLOW_DESKTOP_BINARY at an existing ragflow-desktop.
EOF
  exit 1
fi

mkdir -p "$(dirname "$dest")"

# Re-copying half a gigabyte on every build is pointless: the engine is rebuilt
# in place, so its mtime moves when it changes. The DLLs below are staged on
# every run — they change independently of the engine, and skipping them on an
# unchanged engine shipped a stale librag_tokenizer_c_api.dll.
size() { stat -c %s "$1" 2>/dev/null || stat -f %z "$1"; }
if [ -f "$dest" ] && [ ! "$src" -nt "$dest" ] && [ "$(size "$src")" = "$(size "$dest")" ]; then
  echo "stage-sidecar: engine up to date ($(du -h "$dest" | cut -f1))"
else
  cp -f "$src" "$dest"
  # The macOS/Linux bundle marks the sidecar executable; cp keeps the mode go
  # build wrote, but the bundle step is the one that has to be able to exec it.
  chmod 0755 "$dest" 2>/dev/null || true
  echo "stage-sidecar: $(du -h "$dest" | cut -f1) -> ${dest#"$here"/}"
fi

# The engine's native dependencies are DLLs on Windows (office_oxide, pdfium,
# onnxruntime); Copying them next to the sidecar is what makes the bundled app
# start. build.sh's stage_native_dlls has already put them beside the Go binary.
if [ -n "$exe" ]; then
  for dll in "$(dirname "$src")"/*.dll; do
    [ -f "$dll" ] || continue
    cp -f "$dll" "$(dirname "$dest")/$(basename "$dll")"
    echo "stage-sidecar: staged $(basename "$dll")"
  done
fi
