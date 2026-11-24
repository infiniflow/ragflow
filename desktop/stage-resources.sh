#!/usr/bin/env bash
#
# Stages the analyzer resource tree where the shell's bundle config expects it.
#
# The engine needs the `resource/` tree at startup: the analyzer dictionaries
# (ik, jieba, mecab, rag), OpenCC, WordNet and the spaCy models. It is not source
# code — it is a data bundle the engine loads — so it ships inside the app and is
# resolved at run time by internal/common.ResourceDir.
#
# Tauri copies `bundle.resources` entries into the app's resource directory
# keeping their relative layout, so staging the tree as src-tauri/resource makes
# it land as $RESOURCES/resource, which is exactly what the lookup expects.
#
#   RAGFLOW_RESOURCE_DIR   stage this tree instead of probing for one
#
# A tree is only complete when it contains rag/huqie.trie; a half-copied
# directory would make the engine fail later with a confusing error, so the
# marker is checked here instead.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/.." && pwd)"
dest="$here/src-tauri/resource"
marker="rag/huqie.trie"
stamp="$here/src-tauri/.resource-stamp"

find_source() {
  local candidate
  for candidate in \
    "${RAGFLOW_RESOURCE_DIR:-}" \
    "$repo_root/resource" \
    "$repo_root/../resource" \
    "$repo_root/../../resource" \
    "$HOME/.ragflow/resource" \
    /usr/share/infinity/resource; do
    [ -n "$candidate" ] && [ -f "$candidate/$marker" ] || continue
    printf '%s\n' "$candidate"
    return 0
  done
  return 1
}

if ! src="$(find_source)"; then
  cat >&2 <<EOF
stage-resources: no analyzer resource tree found.

Looked for a directory containing $marker in:
    \$RAGFLOW_RESOURCE_DIR, $repo_root/{resource,../resource,../../resource},
    ~/.ragflow/resource, /usr/share/infinity/resource

Download it (uv run deps/download_deps.py) or point RAGFLOW_RESOURCE_DIR at the
tree, then retry.
EOF
  exit 1
fi

src="$(cd "$src" && pwd)"
# The marker's mtime is enough to notice an updated bundle: the tree is replaced
# wholesale when it is refreshed, it is never edited file by file in place.
# `v2` covers the switch to a .git-free copy (see the exclusions below).
fingerprint="v2 $src $(stat -c %Y "$src/$marker" 2>/dev/null || stat -f %m "$src/$marker" 2>/dev/null)"
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$fingerprint" ] && [ -f "$dest/$marker" ] && [ ! -e "$dest/.git" ]; then
  echo "stage-resources: up to date ($(du -sh "$dest" | cut -f1) from $src)"
  exit 0
fi

# The tree is usually a git checkout, and .git is up to 126 MB of objects the
# app never opens. Excluded, not just ignored: it also carries read-only files,
# which fs::copy preserves — tauri-build copies bundle.resources into
# target/<profile> on every build-script run, and the second copy then fails with
# EACCES opening its own earlier output. The chmod covers any read-only file that
# does belong to the bundle.
if command -v rsync >/dev/null 2>&1; then
  install -d "$dest"
  rsync -a --delete --exclude=.git --exclude=.DS_Store "$src/" "$dest/"
else
  rm -rf "$dest"
  install -d "$dest"
  ( cd "$src" && tar -cf - --exclude=.git --exclude=.DS_Store . ) | ( cd "$dest" && tar -xf - )
fi
# rsync's --delete spares excluded paths, so a tree staged by an older revision
# keeps its .git until it is removed here.
find "$dest" -maxdepth 2 -type d -name .git -prune -exec rm -rf {} + 2>/dev/null || true
find "$dest" -name .DS_Store -delete 2>/dev/null || true
chmod -R u+w "$dest"

printf '%s' "$fingerprint" > "$stamp"
echo "stage-resources: $(du -sh "$dest" | cut -f1) from $src"
