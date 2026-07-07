#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import sys
import types
import warnings

import pytest

# xgboost imports pkg_resources and emits a deprecation warning that is promoted
# to error in our pytest configuration; ignore it for this unit test module.
warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401
        return
    except (ImportError, OSError) as exc:
        # cv2 can fail to import with OSError too (e.g. missing shared libs),
        # not just ImportError; fall back to the stub in both cases.
        logging.debug("cv2 unavailable; installing test stub: %s", exc)

    stub = types.ModuleType("cv2")

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from api.db.services import document_service as ds  # noqa: E402


@pytest.fixture(autouse=True)
def _reset_pending_cache(monkeypatch):
    """Disable the short-lived backlog cache so each test is deterministic."""
    monkeypatch.setattr(ds, "_PENDING_TASK_COUNT_CACHE", {})
    monkeypatch.setattr(ds, "_PENDING_TASK_COUNT_TTL_SECONDS", 0.0)
    yield


def _patch_lag(monkeypatch, lag):
    """Make REDIS_CONN.queue_info report a given consumer-group lag."""
    group_info = None if lag is None else {"lag": lag}
    monkeypatch.setattr(ds.REDIS_CONN, "queue_info", lambda *_a, **_k: group_info)


def _patch_pending(monkeypatch, pending):
    monkeypatch.setattr(ds, "get_pending_task_count", lambda *_a, **_k: pending)


@pytest.mark.p2
class TestGetQueueLength:
    def test_lag_capped_by_genuine_pending(self, monkeypatch):
        # Redis still reports 34 undelivered messages, but only 5 tasks are
        # genuinely waiting -> the user must not see "34 tasks ahead".
        _patch_lag(monkeypatch, 34)
        _patch_pending(monkeypatch, 5)
        assert ds.get_queue_length(0) == 5

    def test_self_heals_to_zero_after_stop(self, monkeypatch):
        # Everything was cancelled: no genuine backlog -> queue length is 0
        # even though stale messages are still sitting in the Redis stream.
        _patch_lag(monkeypatch, 34)
        _patch_pending(monkeypatch, 0)
        assert ds.get_queue_length(0) == 0

    def test_reports_lag_when_smaller_than_pending(self, monkeypatch):
        # Some waiting tasks were already delivered (in flight), so lag is the
        # tighter, truthful bound.
        _patch_lag(monkeypatch, 2)
        _patch_pending(monkeypatch, 9)
        assert ds.get_queue_length(0) == 2

    def test_falls_back_to_lag_when_db_unavailable(self, monkeypatch):
        # If the backlog cannot be computed we keep the previous behaviour.
        _patch_lag(monkeypatch, 7)
        _patch_pending(monkeypatch, None)
        assert ds.get_queue_length(0) == 7

    def test_missing_group_info_is_zero(self, monkeypatch):
        _patch_lag(monkeypatch, None)
        _patch_pending(monkeypatch, 5)
        assert ds.get_queue_length(0) == 0

    def test_null_lag_value_is_treated_as_zero(self, monkeypatch):
        monkeypatch.setattr(ds.REDIS_CONN, "queue_info", lambda *_a, **_k: {"lag": None})
        _patch_pending(monkeypatch, 5)
        assert ds.get_queue_length(0) == 0


@pytest.mark.p2
class TestGetPendingTaskCount:
    def test_returns_none_on_db_error(self, monkeypatch):
        def _boom(*_a, **_k):
            raise RuntimeError("db down")

        monkeypatch.setattr(ds.Task, "select", _boom)
        assert ds.get_pending_task_count() is None

    def test_uses_cache_within_ttl(self, monkeypatch):
        monkeypatch.setattr(ds, "_PENDING_TASK_COUNT_TTL_SECONDS", 60.0)
        # Cache is keyed by priority (None == "all priorities").
        monkeypatch.setattr(
            ds,
            "_PENDING_TASK_COUNT_CACHE",
            {None: {"value": 11, "expire_at": ds.monotonic() + 60.0}},
        )

        def _boom(*_a, **_k):
            raise AssertionError("DB must not be queried while cache is valid")

        monkeypatch.setattr(ds.Task, "select", _boom)
        assert ds.get_pending_task_count() == 11

    def test_cache_is_per_priority(self, monkeypatch):
        monkeypatch.setattr(ds, "_PENDING_TASK_COUNT_TTL_SECONDS", 60.0)
        # Priority 1 is cached; priority 0 is not -> only priority 0 hits the DB.
        monkeypatch.setattr(
            ds,
            "_PENDING_TASK_COUNT_CACHE",
            {1: {"value": 3, "expire_at": ds.monotonic() + 60.0}},
        )

        def _boom(*_a, **_k):
            raise AssertionError("DB must not be queried for a cached priority")

        monkeypatch.setattr(ds.Task, "select", _boom)
        assert ds.get_pending_task_count(1) == 3
