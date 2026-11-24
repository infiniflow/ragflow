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
src="${RAGFLOW_DESKTOP_BINARY:-$here/../bin/ragflow-desktop}"
dest="$here/src-tauri/binaries/ragflow-desktop"

if [ ! -f "$src" ]; then
  cat >&2 <<EOF
stage-sidecar: the engine binary is missing: $src

Build it first, then retry:
    bash build.sh --cpp && bash build.sh --desktop

Or point RAGFLOW_DESKTOP_BINARY at an existing ragflow-desktop.
EOF
  exit 1
fi

install -d "$(dirname "$dest")"

# Re-copying half a gigabyte on every build is pointless: the engine is rebuilt
# in place, so its mtime moves when it changes.
size() { stat -c %s "$1" 2>/dev/null || stat -f %z "$1"; }
if [ -f "$dest" ] && [ ! "$src" -nt "$dest" ] && [ "$(size "$src")" = "$(size "$dest")" ]; then
  echo "stage-sidecar: up to date ($(du -h "$dest" | cut -f1))"
  exit 0
fi

install -m 0755 "$src" "$dest"
echo "stage-sidecar: $(du -h "$dest" | cut -f1) -> ${dest#"$here"/}"
