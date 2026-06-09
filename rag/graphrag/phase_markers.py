#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""GraphRAG phase-completion markers.

Markers let a re-run of GraphRAG skip phases that already completed in a
prior (possibly cancelled or crashed) task on the same KB.

Markers are stored in Redis under ``graphrag:phase:{kb_id}:{phase}`` with a
7-day TTL.  They are intentionally KB-scoped (not task-scoped) so they
survive task cancellation and the creation of a new task on resume.

Invalidation rules (callers responsibility):
* ``clear_phase_markers`` is invoked by ``run_graphrag_for_kb`` whenever new
  document content is merged into the global graph -- the merged graph has
  changed, so prior resolution and community results are stale.
* ``clear_phase_markers`` is invoked by the unbind-task endpoint when the
  caller asks to wipe the graph.
"""

from __future__ import annotations

import logging

from rag.utils.redis_conn import REDIS_CONN


PHASE_RESOLUTION = "resolution_done"
PHASE_COMMUNITY = "community_done"

ALL_PHASES = (PHASE_RESOLUTION, PHASE_COMMUNITY)

# 7 days is well above any expected single GraphRAG run on typical hardware
# and keeps stale markers self-pruning if invalidation paths are missed.
_DEFAULT_TTL_SECONDS = 7 * 24 * 3600

# Dirty-document set TTL: 30 days, much longer than a single run, so a crash
# mid-delta never silently forgets which docs still need processing.
_DIRTY_TTL_SECONDS = 30 * 24 * 3600


def _phase_key(kb_id: str, phase: str) -> str:
    return f"graphrag:phase:{kb_id}:{phase}"


def _dirty_key(kb_id: str) -> str:
    return f"graphrag:dirty:{kb_id}"


def has_phase_marker(kb_id: str, phase: str) -> bool:
    """Return True iff the marker for (kb_id, phase) exists."""
    if not kb_id or not phase:
        return False
    try:
        return bool(REDIS_CONN.exist(_phase_key(kb_id, phase)))
    except Exception:
        # Markers are an optimization; a Redis miss must NEVER block a run.
        logging.exception("has_phase_marker(%s, %s) failed", kb_id, phase)
        return False


def set_phase_marker(kb_id: str, phase: str, ttl: int = _DEFAULT_TTL_SECONDS) -> bool:
    """Persist a marker indicating the named phase has completed for kb_id."""
    if not kb_id or not phase:
        return False
    try:
        return bool(REDIS_CONN.set(_phase_key(kb_id, phase), "1", ttl))
    except Exception:
        logging.exception("set_phase_marker(%s, %s) failed", kb_id, phase)
        return False


def clear_phase_markers(kb_id: str, phases: tuple[str, ...] = ALL_PHASES) -> None:
    """Drop the named phase markers for kb_id (no-op on miss)."""
    if not kb_id:
        return
    for phase in phases:
        try:
            REDIS_CONN.delete(_phase_key(kb_id, phase))
        except Exception:
            logging.exception("clear_phase_markers(%s, %s) failed", kb_id, phase)


# ---------------------------------------------------------------------------
# Dirty-document tracking
#
# A document is "dirty" when it has been added or re-parsed since the last
# successful GraphRAG run on the KB.  The set is stored as a JSON list in
# Redis under graphrag:dirty:{kb_id}.  Callers must:
#   1. mark_document_dirty() whenever a document content changes.
#   2. clear_dirty_documents() after a successful incremental run to remove
#      exactly the docs that were processed.
# ---------------------------------------------------------------------------

def mark_document_dirty(kb_id: str, doc_id: str) -> bool:
    """Add doc_id to the dirty set for kb_id.  Safe to call multiple times.

    Uses a native Redis Set (SADD) so concurrent callers never overwrite each
    other — no read-modify-write race.
    """
    if not kb_id or not doc_id:
        return False
    key = _dirty_key(kb_id)
    try:
        ok = REDIS_CONN.sadd(key, doc_id)
        # Refresh TTL on every write so the set outlives any single run.
        try:
            REDIS_CONN.REDIS.expire(key, _DIRTY_TTL_SECONDS)
        except Exception:
            logging.warning("mark_document_dirty: failed to refresh TTL for %s", key, exc_info=True)
        return bool(ok)
    except Exception:
        logging.exception("mark_document_dirty(%s, %s) failed", kb_id, doc_id)
        return False


def get_dirty_documents(kb_id: str) -> list[str]:
    """Return the list of dirty doc_ids for kb_id (empty list on miss/error)."""
    if not kb_id:
        return []
    try:
        members = REDIS_CONN.smembers(_dirty_key(kb_id))
        if not members:
            return []
        return [m.decode() if isinstance(m, bytes) else m for m in members]
    except Exception:
        logging.exception("get_dirty_documents(%s) failed", kb_id)
        return []


def clear_dirty_documents(kb_id: str, doc_ids: list[str] | None = None) -> bool:
    """Remove doc_ids from the dirty set.  If doc_ids is None, wipe the whole set.

    Uses SREM for targeted removal (atomic, no race) and DEL for a full wipe.
    """
    if not kb_id:
        return False
    key = _dirty_key(kb_id)
    try:
        if doc_ids is None:
            REDIS_CONN.delete(key)
            return True
        if doc_ids:
            REDIS_CONN.REDIS.srem(key, *doc_ids)
        # Refresh TTL if the set still has members so future incremental runs
        # don't lose track of remaining dirty docs after a partial clear.
        try:
            if REDIS_CONN.REDIS.exists(key):
                REDIS_CONN.REDIS.expire(key, _DIRTY_TTL_SECONDS)
        except Exception:
            logging.warning("clear_dirty_documents: failed to refresh TTL for %s", key, exc_info=True)
        return True
    except Exception:
        logging.exception("clear_dirty_documents(%s) failed", kb_id)
        return False
