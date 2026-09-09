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


def _phase_key(kb_id: str, phase: str) -> str:
    return f"graphrag:phase:{kb_id}:{phase}"


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
