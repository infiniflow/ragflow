#!/usr/bin/env bash
#
# Stages the analyzer resource tree where the shell's bundle config expects it.
#
# The engine needs the `resource/` tree at startup: the analyzer dictionaries
# (rag/), OpenCC and WordNet. It is not source code — it is a data bundle the
# engine loads — so it ships inside the app and is resolved at run time by
# internal/common.ResourceDir.
#
# Tauri copies `bundle.resources` entries into the app's resource directory
# keeping their relative layout, so staging the tree as src-tauri/resource makes
# it land as $RESOURCES/resource, which is exactly what the lookup expects.
#
#   RAGFLOW_RESOURCE_DIR      stage this tree instead of probing for one
#   RAGFLOW_RESOURCE_INCLUDE  subdirectories to stage (default: rag wordnet opencc)
#
# A tree is only complete when it contains rag/huqie.trie; a half-copied
# directory would make the engine fail later with a confusing error, so the
# marker is checked here instead.
#
# Copying uses mkdir/cp only. install(1) comes from coreutils, which is present
# in the Git for Windows that build.sh already needs but is not guaranteed by
# every bash a Windows host may have, and there is no fallback for a missing
# install.

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
    "$repo_root/bin/resource" \
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
    \$RAGFLOW_RESOURCE_DIR, $repo_root/{resource,bin/resource,../resource,../../resource},
    ~/.ragflow/resource, /usr/share/infinity/resource

This tree is not one of the downloads: deps/download_deps.py fetches DeepDoc
weights and tokenizer assets, and the Go native libraries come from the release
bundles, but the analyzer dictionaries (rag/, wordnet/, opencc/) are a separate
data checkout. Clone it next to this repository (or into ./resource) and retry:

    git clone --depth 1 https://gitee.com/infiniflow/resource.git resource

Or point RAGFLOW_RESOURCE_DIR at an existing tree.
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
  mkdir -p "$conf_dest"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete "$conf_src/" "$conf_dest/"
  else
    rm -rf "$conf_dest"
    mkdir -p "$conf_dest"
    cp -R "$conf_src/." "$conf_dest/"
  fi
  chmod -R u+w "$conf_dest"
  # The two template catalogues seed the canvas_template table. They live under
  # source directories a packaged app never ships, so they ride along inside conf/
  # where findTemplateDirs resolves them from the executable.
  copy_templates "$repo_root/internal/agent/templates" "$conf_dest/canvas_templates"
  copy_templates "$repo_root/internal/ingestion/pipeline/template" "$conf_dest/pipeline_templates"
  chmod -R u+w "$conf_dest"
  echo "stage-resources: conf/ $(du -sh "$conf_dest" | cut -f1)"
}

copy_templates() {
  local src="$1" dest="$2"
  [ -d "$src" ] || return 0
  if command -v rsync >/dev/null 2>&1; then
    mkdir -p "$dest"
    rsync -a --delete "$src/" "$dest/"
  else
    rm -rf "$dest"
    mkdir -p "$dest"
    cp -R "$src/." "$dest/"
  fi
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
  mkdir -p "$model_dest"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete --exclude=.cache "$model_src/" "$model_dest/"
  else
    rm -rf "$model_dest"
    mkdir -p "$model_dest"
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
  # Exactly the files the counters open, at the paths they look for. The rest of
  # the downloaded tree (model cards, merges, configs, and the InfiniFlow/deepdoc
  # copy of the weights staged above) is 18 MB of metadata the engine never reads.
  rm -rf "$deps_dest"
  local rel
  for rel in \
    cl100k_base.tiktoken \
    huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model \
    huggingface.co/BAAI/bge-large-en-v1.5/vocab.txt \
    huggingface.co/Qwen/Qwen3-Embedding-0.6B/tokenizer.json \
    huggingface.co/intfloat/e5-mistral-7b-instruct/tokenizer.json; do
    if [ ! -f "$deps_src/$rel" ]; then
      echo "stage-resources: warning: deps/$rel is missing; a tokenizer counter will be unavailable" >&2
      continue
    fi
    mkdir -p "$deps_dest/$(dirname "$rel")"
    cp -f "$deps_src/$rel" "$deps_dest/$rel"
  done
  chmod -R u+w "$deps_dest"
  echo "stage-resources: deps/ $(du -sh "$deps_dest" | cut -f1)"
}

src="$(cd "$src" && pwd)"
stage_conf
stage_deepdoc
stage_deps

# Two analyzer implementations read this tree and share it: the Go tokenizer
# (C++ analyzer: rag/, wordnet/, opencc/) and the embedded Infinity engine's `rag`
# analyzer, which reads rag/. Infinity's optional `chinese`, `ik`, `japanese` and
# `korean` analyzers want jieba/, ik/ and mecab/ instead, and nothing in this
# build selects them — 145 MB of a 248 MB tree. spacy/ likewise only serves the
# cgo_thincner NER extractor, which the desktop tags (cgo,static,sonic,desktop) do
# not compile. Both stay out; RAGFLOW_RESOURCE_INCLUDE adds them back for a build
# that needs those analyzers.
resource_include="${RAGFLOW_RESOURCE_INCLUDE:-rag wordnet opencc}"

# The marker's mtime is enough to notice an updated bundle: the tree is replaced
# wholesale when it is refreshed, it is never edited file by file in place.
# `v3` covers the switch to a subset copy; the include list is part of the key so
# widening it re-stages.
fingerprint="v3 $src $resource_include $(stat -c %Y "$src/$marker" 2>/dev/null || stat -f %m "$src/$marker" 2>/dev/null)"
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$fingerprint" ] && [ -f "$dest/$marker" ] && [ ! -e "$dest/.git" ]; then
  echo "stage-resources: up to date ($(du -sh "$dest" | cut -f1) from $src)"
  exit 0
fi

# Built fresh rather than synced: the destination must not keep a subdirectory a
# narrower include list dropped. The tree is usually a git checkout, and .git is
# up to 126 MB of objects the app never opens — it also carries read-only files,
# which fs::copy preserves, and tauri-build copies bundle.resources into
# target/<profile> on every build-script run, so the second copy would then fail
# with EACCES opening its own earlier output. The chmod covers the rest.
rm -rf "$dest"
mkdir -p "$dest"
for part in $resource_include; do
  if [ ! -e "$src/$part" ]; then
    echo "stage-resources: warning: $src/$part is missing; the analyzer may not load" >&2
    continue
  fi
  if command -v rsync >/dev/null 2>&1; then
    rsync -a "$src/$part" "$dest/"
  else
    cp -R "$src/$part" "$dest/"
  fi
done
for top in LICENSE README.md; do
  [ -f "$src/$top" ] && cp "$src/$top" "$dest/"
done
find "$dest" -name .DS_Store -delete 2>/dev/null || true
find "$dest" -maxdepth 2 -type d -name .git -prune -exec rm -rf {} + 2>/dev/null || true
chmod -R u+w "$dest"

printf '%s' "$fingerprint" > "$stamp"
echo "stage-resources: $(du -sh "$dest" | cut -f1) from $src"
