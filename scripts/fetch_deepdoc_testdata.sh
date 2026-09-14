#!/usr/bin/env bash
# Fetch external DeepDoc testdata into the repo on demand.
#
# Usage: scripts/fetch_deepdoc_testdata.sh <pkg>
#   <pkg> = the package directory that owns the testdata, e.g. "native"
#          (mirrors internal/deepdoc/<pkg>/testdata).
#
# The data lives in an external asset repository
# (RAGFLOW_TESTDATA_REPO, default infiniflow/ragflow-testdata — the canonical
# org-owned asset repository. The data was migrated out of the xugangqiang fork
# (see deepdoc_native_testdata_handoff.md S8) and is pinned by a tag recorded in
# internal/deepdoc/<pkg>/testdata.ref. We sparse-clone only the relevant
# subtree into a content-addressed cache and symlink it into the package so
# existing tests (which read relative testdata/... paths) need no changes.
#
# Env:
#   RAGFLOW_TESTDATA_REPO  repo "owner/name" (default infiniflow/ragflow-testdata)
#   RAGFLOW_TESTDATA_REF   override the anchor tag/ref (else read testdata.ref)
#   RAGFLOW_TESTDATA_DIR   pre-seeded asset repo root (first level: deepdoc/).
#                          When it carries deepdoc/<pkg>/testdata, that copy is
#                          used as-is: no ref pinning, no network fetch. This is
#                          what a CI runner that mounts the asset repo should
#                          set; a missing subtree falls back to cloning.
#   XDG_CACHE_HOME         cache base (default ~/.cache)
#
# Behavior:
#   - With RAGFLOW_TESTDATA_DIR set, the pre-seeded copy wins over everything
#     below. The ref pin is bypassed — whoever seeds the directory owns its
#     freshness.
#   - If testdata is already present INLINE (a real dir, pre-migration), this
#     script leaves it untouched and exits (nothing to fetch).
#   - If a correct symlink already exists, it exits.
#   - Only when testdata is ABSENT do we clone the pinned subtree and symlink
#     it in. We never delete an inline (tracked) testdata directory.
#   - When a GEN_* env var is set (testdata regeneration, e.g. GEN_CONTOURS=1),
#     the subtree is COPIED to a writable local dir instead of symlinked, so
#     regeneration tests can write back (handoff S4.5).

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "usage: $0 <pkg>" >&2
  exit 2
fi
PKG="$1"

# Resolve repo root from this script's location (scripts/ -> repo root).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TARGET="$ROOT/internal/deepdoc/$PKG/testdata"

# Serialize all invocations for a given <pkg>. `go test ./internal/deepdoc/native/...`
# runs the `native` and `croptest` test binaries concurrently, and croptest imports
# native — so native's fetch_testdata init() runs in BOTH processes at once, racing on
# the same TARGET. Two concurrent `ln -s` calls on a symlink-to-directory make the second
# one descend INTO it and create <TARGET>/testdata ("Permission denied"), the exact CI
# failure this guards against. A per-package flock makes the rm+ln critical section
# atomic across processes; the first writer wins, the rest wait and then see the result.
LOCKDIR="${XDG_CACHE_HOME:-$HOME/.cache}/ragflow-testdata-locks"
mkdir -p "$LOCKDIR"
exec 9>"$LOCKDIR/$PKG.lock"
flock 9

# Determine whether we need a writable copy (regeneration) or a symlink.
NEED_WRITE=0
for v in "${!GEN_@}"; do
  if [ -n "${!v:-}" ]; then NEED_WRITE=1; break; fi
done

# Pre-seeded testdata: CI runners mount the asset repo at a fixed path instead
# of cloning it. Use that copy directly when it carries this package's subtree.
# This runs before the ref is resolved: a pre-seeded copy carries no ref, so
# packages without a testdata.ref can be served this way too.
if [ -n "${RAGFLOW_TESTDATA_DIR:-}" ]; then
  PRESET="$RAGFLOW_TESTDATA_DIR/deepdoc/$PKG/testdata"
  if [ -d "$PRESET" ] && [ -n "$(ls -A "$PRESET" 2>/dev/null)" ]; then
    if [ -L "$TARGET" ] && [ "$(readlink -f "$TARGET")" = "$(readlink -f "$PRESET")" ] && [ -n "$(ls -A "$PRESET" 2>/dev/null)" ]; then
      echo "fetch_deepdoc_testdata: $PKG already linked to pre-seeded $PRESET"
      exit 0
    fi
    if [ -d "$TARGET" ] && [ ! -L "$TARGET" ] && [ -n "$(ls -A "$TARGET" 2>/dev/null)" ]; then
      echo "fetch_deepdoc_testdata: $PKG testdata already present inline at $TARGET"
      exit 0
    fi
    # Remove any stale TARGET before linking. TARGET may be a wrong symlink, an
    # empty directory, or a leftover real directory from a prior GEN_* copy or a
    # dereferenced symlink on a persistent runner. `ln -sfn` below also unlinks
    # TARGET first, but removing it here keeps intent explicit. A partial
    # removal failure is non-fatal: ln -sfn still replaces TARGET, and the copy
    # fallback below covers the rare case where even that is blocked.
    #
    # Use `ln -sfn`, never `ln -s`: when TARGET already exists as a directory,
    # plain `ln -s` descends INTO it and creates <TARGET>/testdata (the exact
    # "Permission denied" failure this job hit), because the existing dir is not
    # writable. `-f` unlinks TARGET first; `-n`/`--no-dereference` stops ln from
    # treating TARGET as a container.
    rm -rf -- "$TARGET" 2>/dev/null || true
    if [ "$NEED_WRITE" -eq 1 ]; then
      echo "fetch_deepdoc_testdata: copying writable pre-seeded testdata for regeneration ($PKG)"
      rm -rf -- "$TARGET" 2>/dev/null || true
      cp -r "$PRESET" "$TARGET"
    elif ln -sfn "$PRESET" "$TARGET" 2>/dev/null; then
      echo "fetch_deepdoc_testdata: linked $TARGET -> $PRESET (pre-seeded)"
    else
      # Symlink creation failed (e.g. the parent directory is not writable on a
      # shared runner). The pre-seeded fixtures are authoritative and readable,
      # so copy them in place instead of reddening CI over a filesystem quirk.
      echo "fetch_deepdoc_testdata: symlink failed; copying pre-seeded testdata for $PKG" >&2
      mkdir -p "$TARGET"
      cp -r "$PRESET"/. "$TARGET"/.
    fi
    echo "fetch_deepdoc_testdata: done ($PKG, pre-seeded)"
    exit 0
  fi
  echo "fetch_deepdoc_testdata: RAGFLOW_TESTDATA_DIR set but deepdoc/$PKG/testdata is missing; falling back to clone" >&2
fi

REF_FILE="$ROOT/internal/deepdoc/$PKG/testdata.ref"
REF="${RAGFLOW_TESTDATA_REF:-}"
if [ -z "$REF" ] && [ -f "$REF_FILE" ]; then
  REF="$(head -n1 "$REF_FILE" | tr -d '[:space:]')"
fi
if [ -z "$REF" ]; then
  echo "fetch_deepdoc_testdata: no ref set (RAGFLOW_TESTDATA_REF or $REF_FILE)" >&2
  exit 1
fi

REPO="${RAGFLOW_TESTDATA_REPO:-infiniflow/ragflow-testdata}"
CACHE_BASE="${XDG_CACHE_HOME:-$HOME/.cache}/ragflow-testdata"
CACHE="$CACHE_BASE/$REF"
SRC="$CACHE/deepdoc/$PKG/testdata"

# Already a correct symlink -> done.
if [ -L "$TARGET" ] && [ "$(readlink -f "$TARGET")" = "$(readlink -f "$SRC")" ] && [ -n "$(ls -A "$SRC" 2>/dev/null)" ]; then
  echo "fetch_deepdoc_testdata: $PKG already linked ($REF)"
  exit 0
fi

# Inline testdata already present (real dir, pre-migration) -> nothing to do.
if [ -d "$TARGET" ] && [ ! -L "$TARGET" ] && [ -n "$(ls -A "$TARGET" 2>/dev/null)" ]; then
  echo "fetch_deepdoc_testdata: $PKG testdata already present inline at $TARGET"
  exit 0
fi

# Absent (or stale symlink) -> fetch.
# rm -rf for the same reason as the pre-seeded branch: a leftover empty
# directory must be removed, not silently turned into a nested symlink.
rm -rf -- "$TARGET"

if [ ! -e "$SRC" ] || [ -z "$(ls -A "$SRC" 2>/dev/null)" ]; then
  echo "fetch_deepdoc_testdata: cloning $REPO @ $REF (subtree deepdoc/$PKG/testdata)"
  mkdir -p "$CACHE_BASE"
  # Network clones are best-effort and occasionally fail with a transient TLS
  # reset (seen on the self-hosted runner). Retry a few times before giving up
  # so a CI blip does not redden the run.
  attempt=0
  max_attempts=3
  fetched=0
  until [ "$attempt" -ge "$max_attempts" ]; do
    attempt=$((attempt + 1))
    rm -rf "$CACHE"
    if git clone --depth 1 --filter=blob:none --branch "$REF" --sparse \
         "https://github.com/$REPO.git" "$CACHE" >&2 && \
       git -C "$CACHE" sparse-checkout set "deepdoc/$PKG/testdata" >&2; then
      fetched=1
      break
    fi
    echo "fetch_deepdoc_testdata: clone attempt $attempt/$max_attempts failed, retrying in 3s" >&2
    sleep 3
  done
  if [ "$fetched" -ne 1 ]; then
    echo "fetch_deepdoc_testdata: clone failed after $max_attempts attempts" >&2
    exit 1
  fi
fi

if [ ! -e "$SRC" ] || [ -z "$(ls -A "$SRC" 2>/dev/null)" ]; then
  echo "fetch_deepdoc_testdata: cloned but subtree deepdoc/$PKG/testdata is empty" >&2
  exit 1
fi

if [ "$NEED_WRITE" -eq 1 ]; then
  echo "fetch_deepdoc_testdata: copying writable testdata for regeneration ($PKG @ $REF)"
  rm -rf "$TARGET"
  cp -r "$SRC" "$TARGET"
else
  echo "fetch_deepdoc_testdata: linking $TARGET -> $SRC"
  ln -sfn "$SRC" "$TARGET"
fi
echo "fetch_deepdoc_testdata: done ($PKG @ $REF)"
