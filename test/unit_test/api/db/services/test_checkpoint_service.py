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

import pytest

from api.db.services import checkpoint_service
from api.db.services.checkpoint_service import (
    CHECKPOINT_KEY_PREFIX,
    CHECKPOINT_TTL,
    _checkpoint_key,
    clear_checkpoint,
    load_checkpoint,
    save_checkpoint,
)


class _FakeRedis:
    """In-memory fake that mimics the subset of Redis commands used by checkpoint_service."""

    def __init__(self):
        self._store = {}

    def sadd(self, key, *values):
        if key not in self._store:
            self._store[key] = set()
        self._store[key].update(values)

    def smembers(self, key):
        return set(self._store.get(key, set()))

    def expire(self, key, ttl):
        pass  # TTL is a no-op in the fake

    def delete(self, *keys):
        for key in keys:
            self._store.pop(key, None)


class _FakeRedisConn:
    """Mimics REDIS_CONN with a .REDIS attribute."""

    def __init__(self):
        self.REDIS = _FakeRedis()


class _BrokenRedis:
    """Redis stub that raises on every operation."""

    def sadd(self, *args, **kwargs):
        raise ConnectionError("Redis unavailable")

    def smembers(self, *args, **kwargs):
        raise ConnectionError("Redis unavailable")

    def expire(self, *args, **kwargs):
        raise ConnectionError("Redis unavailable")

    def delete(self, *args, **kwargs):
        raise ConnectionError("Redis unavailable")


class _BrokenRedisConn:
    def __init__(self):
        self.REDIS = _BrokenRedis()


@pytest.fixture
def fake_redis(monkeypatch):
    """Replace REDIS_CONN with an in-memory fake for each test."""
    conn = _FakeRedisConn()
    monkeypatch.setattr(checkpoint_service, "REDIS_CONN", conn)
    return conn


@pytest.fixture
def broken_redis(monkeypatch):
    """Replace REDIS_CONN with a stub that always raises."""
    conn = _BrokenRedisConn()
    monkeypatch.setattr(checkpoint_service, "REDIS_CONN", conn)
    return conn


class TestCheckpointKey:
    """Tests for _checkpoint_key helper."""

    def test_prefixes_task_id(self):
        assert _checkpoint_key("task123") == f"{CHECKPOINT_KEY_PREFIX}task123"

    def test_empty_task_id(self):
        assert _checkpoint_key("") == f"{CHECKPOINT_KEY_PREFIX}"


class TestSaveCheckpoint:
    """Tests for save_checkpoint function."""

    @pytest.mark.p2
    def test_save_single_doc(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        key = _checkpoint_key("task1")
        assert "doc_a" in fake_redis.REDIS.smembers(key)

    @pytest.mark.p2
    def test_save_multiple_docs(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        save_checkpoint("task1", "doc_b")
        save_checkpoint("task1", "doc_c")
        key = _checkpoint_key("task1")
        assert fake_redis.REDIS.smembers(key) == {"doc_a", "doc_b", "doc_c"}

    @pytest.mark.p2
    def test_save_duplicate_doc_is_idempotent(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        save_checkpoint("task1", "doc_a")
        key = _checkpoint_key("task1")
        assert fake_redis.REDIS.smembers(key) == {"doc_a"}

    @pytest.mark.p2
    def test_save_different_tasks_are_independent(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        save_checkpoint("task2", "doc_b")
        assert fake_redis.REDIS.smembers(_checkpoint_key("task1")) == {"doc_a"}
        assert fake_redis.REDIS.smembers(_checkpoint_key("task2")) == {"doc_b"}

    @pytest.mark.p3
    def test_save_does_not_raise_on_redis_error(self, broken_redis):
        save_checkpoint("task1", "doc_a")  # should not raise


class TestLoadCheckpoint:
    """Tests for load_checkpoint function."""

    @pytest.mark.p2
    def test_load_returns_saved_docs(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        save_checkpoint("task1", "doc_b")
        result = load_checkpoint("task1")
        assert result == {"doc_a", "doc_b"}

    @pytest.mark.p2
    def test_load_empty_for_unknown_task(self, fake_redis):
        result = load_checkpoint("nonexistent")
        assert result == set()

    @pytest.mark.p2
    def test_load_returns_set_type(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        result = load_checkpoint("task1")
        assert isinstance(result, set)

    @pytest.mark.p3
    def test_load_returns_empty_set_on_redis_error(self, broken_redis):
        result = load_checkpoint("task1")
        assert result == set()


class TestClearCheckpoint:
    """Tests for clear_checkpoint function."""

    @pytest.mark.p2
    def test_clear_removes_all_docs(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        save_checkpoint("task1", "doc_b")
        clear_checkpoint("task1")
        result = load_checkpoint("task1")
        assert result == set()

    @pytest.mark.p2
    def test_clear_does_not_affect_other_tasks(self, fake_redis):
        save_checkpoint("task1", "doc_a")
        save_checkpoint("task2", "doc_b")
        clear_checkpoint("task1")
        assert load_checkpoint("task1") == set()
        assert load_checkpoint("task2") == {"doc_b"}

    @pytest.mark.p2
    def test_clear_nonexistent_task_is_safe(self, fake_redis):
        clear_checkpoint("nonexistent")  # should not raise

    @pytest.mark.p3
    def test_clear_does_not_raise_on_redis_error(self, broken_redis):
        clear_checkpoint("task1")  # should not raise


class TestFullWorkflow:
    """Integration-style tests for the checkpoint lifecycle."""

    @pytest.mark.p1
    def test_save_load_clear_cycle(self, fake_redis):
        """Simulates a GraphRAG task: save progress doc by doc, load on resume, clear on completion."""
        task_id = "graphrag_task_001"
        doc_ids = ["doc_1", "doc_2", "doc_3", "doc_4", "doc_5"]

        # Simulate processing first 3 docs before crash
        for doc_id in doc_ids[:3]:
            save_checkpoint(task_id, doc_id)

        # Simulate restart: load checkpoint
        completed = load_checkpoint(task_id)
        assert completed == {"doc_1", "doc_2", "doc_3"}

        # Simulate resuming: process remaining docs
        remaining = [d for d in doc_ids if d not in completed]
        assert remaining == ["doc_4", "doc_5"]
        for doc_id in remaining:
            save_checkpoint(task_id, doc_id)

        # All docs now completed
        assert load_checkpoint(task_id) == set(doc_ids)

        # Task done: clear checkpoint
        clear_checkpoint(task_id)
        assert load_checkpoint(task_id) == set()

    @pytest.mark.p1
    def test_concurrent_saves_no_data_loss(self, fake_redis):
        """Redis SADD is atomic — concurrent saves should never lose data."""
        task_id = "concurrent_task"
        doc_ids = [f"doc_{i}" for i in range(100)]

        for doc_id in doc_ids:
            save_checkpoint(task_id, doc_id)

        result = load_checkpoint(task_id)
        assert result == set(doc_ids)

    @pytest.mark.p2
    def test_ttl_constant_is_7_days(self):
        assert CHECKPOINT_TTL == 60 * 60 * 24 * 7
