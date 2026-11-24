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

# The engine also reads its configuration directory at start: conf/models is
# fatal when missing, and conf/system_settings.json seeds the defaults. It is part
# of the repository and small (1.7 MB), so it is re-synced on every build instead
# of being fingerprinted.
stage_conf() {
  local conf_src="$repo_root/conf" conf_dest="$here/src-tauri/conf"
  if [ ! -d "$conf_src" ]; then
    echo "stage-resources: warning: $conf_src is missing; the packaged app cannot load model providers" >&2
    return 0
  fi
  install -d "$conf_dest"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete "$conf_src/" "$conf_dest/"
  else
    rm -rf "$conf_dest"
    install -d "$conf_dest"
    cp -R "$conf_src/." "$conf_dest/"
  fi
  chmod -R u+w "$conf_dest"
  echo "stage-resources: conf/ $(du -sh "$conf_dest" | cut -f1)"
}

# DeepDoc's ONNX weights are mandatory: the engine aborts at startup when the
# in-process backend cannot load them. The download cache inside the directory is
# not part of the bundle.
stage_deepdoc() {
  local model_src="$repo_root/internal/rag/res/deepdoc" model_dest="$here/src-tauri/deepdoc"
  if [ ! -d "$model_src" ]; then
    echo "stage-resources: warning: $model_src is missing; the packaged app cannot start (DeepDoc is required)" >&2
    return 0
  fi
  install -d "$model_dest"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete --exclude=.cache "$model_src/" "$model_dest/"
  else
    rm -rf "$model_dest"
    install -d "$model_dest"
    ( cd "$model_src" && tar -cf - --exclude=.cache . ) | ( cd "$model_dest" && tar -xf - )
  fi
  chmod -R u+w "$model_dest"
  echo "stage-resources: deepdoc/ $(du -sh "$model_dest" | cut -f1)"
}

# Runtime tokenizer assets: the cl100k BPE table and the pinned embedding
# tokenizer.json catalogues under huggingface.co. The engine aborts when the
# table is missing, and deps/ as a whole must not ship — it also holds the
# build-time native-library archives (hundreds of MB) and the download scripts.
# huggingface.co/InfiniFlow is left out too: those are the DeepDoc weights,
# already staged above under their own directory.
stage_deps() {
  local deps_src="$repo_root/deps" deps_dest="$here/src-tauri/deps"
  if [ ! -d "$deps_src" ]; then
    echo "stage-resources: warning: $deps_src is missing; tokenization will fail at startup" >&2
    return 0
  fi
  rm -rf "$deps_dest"
  install -d "$deps_dest/huggingface.co"
  if [ -f "$deps_src/cl100k_base.tiktoken" ]; then
    install -m 0644 "$deps_src/cl100k_base.tiktoken" "$deps_dest/"
  else
    echo "stage-resources: warning: deps/cl100k_base.tiktoken is missing; the engine cannot count tokens" >&2
  fi
  local repo
  for repo in BAAI Qwen intfloat; do
    [ -d "$deps_src/huggingface.co/$repo" ] || continue
    cp -R "$deps_src/huggingface.co/$repo" "$deps_dest/huggingface.co/"
  done
  chmod -R u+w "$deps_dest"
  echo "stage-resources: deps/ $(du -sh "$deps_dest" | cut -f1)"
}

src="$(cd "$src" && pwd)"
stage_conf
stage_deepdoc
stage_deps

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
