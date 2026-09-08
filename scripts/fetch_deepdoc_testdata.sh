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
#   XDG_CACHE_HOME         cache base (default ~/.cache)
#
# Behavior:
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
TARGET="$ROOT/internal/deepdoc/$PKG/testdata"

# Serialize concurrent invocations. `go test ./internal/deepdoc/native/...`
# builds the `native` and `croptest` test binaries and runs them in parallel,
# and both link package native, whose fetch_testdata-tagged init() runs this
# script for the same <pkg> at the same time. The instances share $CACHE and
# $TARGET, so an unlocked `rm -rf $CACHE` in one destroys the other's in-flight
# clone/sparse-checkout — or a cache the other just finished linking — and the
# tests then fail on missing fixtures. Hold an exclusive lock across the whole
# decide/clone/link section; a waiter acquires it only after the winner is done
# and then takes the read-only "already linked" fast path without mutating.
mkdir -p "$CACHE_BASE"
LOCK="$CACHE_BASE/fetch.lock"
lock_acquired=0
lock_deadline=$(( $(date +%s) + 600 ))
while :; do
  if mkdir "$LOCK" 2>/dev/null; then
    lock_acquired=1
    break
  fi
  # Break a stale lock left by a killed run (cancelled job, crash): no healthy
  # clone outlives 15 minutes here.
  if [ -n "$(find "$LOCK" -maxdepth 0 -mmin +15 2>/dev/null)" ]; then
    echo "fetch_deepdoc_testdata: removing stale lock $LOCK" >&2
    rm -rf "$LOCK" 2>/dev/null || true
    continue
  fi
  if [ "$(date +%s)" -ge "$lock_deadline" ]; then
    # Deadlocking the test binary forever is worse than the race this lock
    # fixes; give up and run unlocked (loudly).
    echo "fetch_deepdoc_testdata: timed out waiting for $LOCK; proceeding unlocked" >&2
    break
  fi
  sleep 1
done
release_lock() {
  if [ "$lock_acquired" -eq 1 ]; then
    rm -rf "$LOCK" 2>/dev/null || true
  fi
}
trap release_lock EXIT

# Determine whether we need a writable copy (regeneration) or a symlink.
NEED_WRITE=0
for v in "${!GEN_@}"; do
  if [ -n "${!v:-}" ]; then NEED_WRITE=1; break; fi
done

# src_complete: the cache is usable only when every file the pinned tree
# records for the subtree is present on disk. A partial checkout (interrupted
# lazy blob fetch, clone clobbered by a concurrent run, leftover from a
# cancelled job) is non-empty but incomplete, and linking it fails the tests
# on missing fixtures — so treat it as absent and re-clone.
src_complete() {
  [ -d "$SRC" ] || return 1
  git -C "$CACHE" rev-parse --verify HEAD >/dev/null 2>&1 || return 1
  local listed missing
  listed="$(git -C "$CACHE" ls-tree -r --name-only HEAD -- "deepdoc/$PKG/testdata" 2>/dev/null || true)"
  [ -n "$listed" ] || return 1
  missing="$(printf '%s\n' "$listed" | while IFS= read -r f; do
    [ -f "$CACHE/$f" ] || echo "$f"
  done)"
  if [ -n "$missing" ]; then
    echo "fetch_deepdoc_testdata: cache at $CACHE is missing pinned files:" >&2
    printf '%s\n' "$missing" | head -5 | sed 's/^/  /' >&2
    return 1
  fi
}

# Already a correct symlink over a complete cache -> done.
if [ -L "$TARGET" ] && [ "$(readlink -f "$TARGET")" = "$(readlink -f "$SRC")" ] && src_complete; then
  echo "fetch_deepdoc_testdata: $PKG already linked ($REF)"
  exit 0
fi

# Inline testdata already present (real dir, pre-migration) -> nothing to do.
if [ -d "$TARGET" ] && [ ! -L "$TARGET" ] && [ -n "$(ls -A "$TARGET" 2>/dev/null)" ]; then
  echo "fetch_deepdoc_testdata: $PKG testdata already present inline at $TARGET"
  exit 0
fi

# Absent (or stale symlink / incomplete cache) -> fetch.
rm -f "$TARGET" 2>/dev/null || true

if ! src_complete; then
  echo "fetch_deepdoc_testdata: cloning $REPO @ $REF (subtree deepdoc/$PKG/testdata)"
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
       git -C "$CACHE" sparse-checkout set "deepdoc/$PKG/testdata" >&2 && \
       src_complete; then
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

if [ "$NEED_WRITE" -eq 1 ]; then
  echo "fetch_deepdoc_testdata: copying writable testdata for regeneration ($PKG @ $REF)"
  rm -rf "$TARGET"
  cp -r "$SRC" "$TARGET"
else
  echo "fetch_deepdoc_testdata: linking $TARGET -> $SRC"
  ln -s "$SRC" "$TARGET"
fi
echo "fetch_deepdoc_testdata: done ($PKG @ $REF)"
