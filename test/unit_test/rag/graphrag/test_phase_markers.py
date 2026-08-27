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
"""Tests for GraphRAG phase-completion markers."""

import importlib
import sys
from unittest.mock import MagicMock

import pytest


@pytest.fixture
def fake_redis(monkeypatch):
    """Replace REDIS_CONN inside phase_markers with an in-memory fake."""
    store: dict[str, tuple[str, int]] = {}

    fake = MagicMock()
    fake.exist = lambda k: k in store
    fake.get = lambda k: store[k][0] if k in store else None

    def _set(k, v, exp=3600):
        store[k] = (v, exp)
        return True

    def _delete(k):
        store.pop(k, None)
        return True

    fake.set = _set
    fake.delete = _delete

    # Re-import the module so the patched REDIS_CONN is used.
    sys.modules.pop("rag.graphrag.phase_markers", None)
    sys.modules["rag.utils.redis_conn"] = MagicMock(REDIS_CONN=fake)
    module = importlib.import_module("rag.graphrag.phase_markers")
    return module, store, fake


@pytest.mark.p1
def test_set_and_has_phase_marker_round_trip(fake_redis):
    module, store, _ = fake_redis
    assert module.has_phase_marker("kb-1", module.PHASE_RESOLUTION) is False
    assert module.set_phase_marker("kb-1", module.PHASE_RESOLUTION) is True
    assert module.has_phase_marker("kb-1", module.PHASE_RESOLUTION) is True
    # Marker is namespaced by kb_id and phase
    assert "graphrag:phase:kb-1:resolution_done" in store
    assert module.has_phase_marker("kb-2", module.PHASE_RESOLUTION) is False
    assert module.has_phase_marker("kb-1", module.PHASE_COMMUNITY) is False


@pytest.mark.p1
def test_clear_phase_markers_drops_all_named(fake_redis):
    module, store, _ = fake_redis
    module.set_phase_marker("kb-1", module.PHASE_RESOLUTION)
    module.set_phase_marker("kb-1", module.PHASE_COMMUNITY)
    module.set_phase_marker("kb-2", module.PHASE_RESOLUTION)

    module.clear_phase_markers("kb-1")

    assert module.has_phase_marker("kb-1", module.PHASE_RESOLUTION) is False
    assert module.has_phase_marker("kb-1", module.PHASE_COMMUNITY) is False
    # Other KBs untouched.
    assert module.has_phase_marker("kb-2", module.PHASE_RESOLUTION) is True


@pytest.mark.p1
def test_phase_marker_helpers_are_silent_on_invalid_input(fake_redis):
    module, _store, _ = fake_redis
    assert module.has_phase_marker("", module.PHASE_RESOLUTION) is False
    assert module.set_phase_marker("", module.PHASE_RESOLUTION) is False
    # Empty kb_id is a silent no-op, never raises.
    module.clear_phase_markers("")


@pytest.mark.p2
def test_redis_failure_does_not_break_pipeline(fake_redis):
    module, _store, fake = fake_redis

    def _boom(*_args, **_kwargs):
        raise RuntimeError("redis down")

    fake.exist = _boom
    fake.set = _boom
    fake.delete = _boom

    # Marker absence must be assumed on Redis failure -- the pipeline must
    # always be allowed to run rather than incorrectly skipping a phase.
    assert module.has_phase_marker("kb-1", module.PHASE_RESOLUTION) is False
    assert module.set_phase_marker("kb-1", module.PHASE_RESOLUTION) is False
    module.clear_phase_markers("kb-1")  # must not raise
